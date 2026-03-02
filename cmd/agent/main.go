package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gregorytcarroll/k8s-sage/internal/agent"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		nodeName = "unknown"
	}

	kubeletURL := os.Getenv("KUBELET_URL")
	if kubeletURL == "" {
		kubeletURL = "https://localhost:10250"
	}

	serverURL := os.Getenv("SERVER_URL")
	if serverURL == "" {
		serverURL = "http://k8s-sage-server:8080"
	}

	pushInterval := 5 * time.Minute
	if pi := os.Getenv("PUSH_INTERVAL"); pi != "" {
		parsed, err := time.ParseDuration(pi)
		if err != nil {
			slog.Error("invalid PUSH_INTERVAL", "value", pi, "error", err)
			os.Exit(1)
		}
		pushInterval = parsed
	}

	store := agent.NewStore()
	collector := agent.NewCollector(kubeletURL, store, 30*time.Second)
	reporter := agent.NewReporter(nodeName, store)
	pusher := agent.NewPusher(serverURL, reporter, pushInterval)

	mux := http.NewServeMux()
	mux.HandleFunc("/report", reporter.HandleReport)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:         ":9101",
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go collector.Run(ctx)
	go pusher.Run(ctx)

	go func() {
		slog.Info("agent listening", "addr", server.Addr, "node", nodeName)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh

	slog.Info("shutting down")
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}
}
