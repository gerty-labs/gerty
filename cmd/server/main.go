package main

import (
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gregorytcarroll/k8s-sage/internal/rules"
	"github.com/gregorytcarroll/k8s-sage/internal/server"
	"github.com/gregorytcarroll/k8s-sage/internal/slm"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	aggregator := server.NewAggregator()
	engine := rules.NewEngine()

	// Optional SLM client for L2 recommendations
	var slmClient *slm.Client
	slmURL := os.Getenv("SLM_URL")
	if slmURL != "" {
		slmClient = slm.NewClient(slmURL, 10*time.Second)
		slog.Info("SLM enabled", "url", slmURL)
	} else {
		slog.Info("SLM disabled (set SLM_URL to enable)")
	}

	analyzer := server.NewAnalyzer(engine, slmClient)

	done := make(chan struct{})
	aggregator.StartPruner(done)

	api := server.NewAPI(aggregator, engine, analyzer)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	srv := &http.Server{
		Addr:         addr,
		Handler:      server.LoggingMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("sage-server listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh

	slog.Info("shutting down sage-server")
	close(done)
	srv.Close()
}
