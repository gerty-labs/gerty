package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gregorytcarroll/k8s-sage/internal/rules"
	"github.com/gregorytcarroll/k8s-sage/internal/server"
	"github.com/gregorytcarroll/k8s-sage/internal/slack"
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

	// Optional Slack notifier
	if webhookURL := os.Getenv("SLACK_WEBHOOK_URL"); webhookURL != "" {
		interval := 1 * time.Hour
		if raw := os.Getenv("SLACK_DIGEST_INTERVAL"); raw != "" {
			if parsed, err := time.ParseDuration(raw); err == nil {
				interval = parsed
			}
		}
		channel := os.Getenv("SLACK_CHANNEL")
		if channel == "" {
			channel = "#k8s-sage"
		}
		minSeverity := slack.SeverityOptimisation
		if raw := os.Getenv("SLACK_MIN_SEVERITY"); raw != "" {
			minSeverity = slack.Severity(raw)
		}

		notifier := slack.NewNotifier(slack.Config{
			WebhookURL:     webhookURL,
			Channel:        channel,
			DigestInterval: interval,
			MinSeverity:    minSeverity,
		}, aggregator, engine)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go notifier.Run(ctx)
		slog.Info("Slack notifier enabled", "channel", channel, "interval", interval)
	}

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
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}
}
