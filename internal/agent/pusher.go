package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Pusher periodically POSTs the agent's NodeReport to the central server.
type Pusher struct {
	serverURL string
	reporter  *Reporter
	interval  time.Duration
	client    *http.Client
}

const minPushInterval = 100 * time.Millisecond

// NewPusher creates a Pusher that sends reports to serverURL every interval.
func NewPusher(serverURL string, reporter *Reporter, interval time.Duration) *Pusher {
	if interval < minPushInterval {
		interval = minPushInterval
	}
	return &Pusher{
		serverURL: serverURL,
		reporter:  reporter,
		interval:  interval,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Run starts the push loop, blocking until ctx is cancelled.
func (p *Pusher) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.pushOnce(ctx); err != nil {
				slog.Warn("push to server failed", "error", err, "serverURL", p.serverURL)
			}
		}
	}
}

// pushOnce builds the current report and POSTs it to the server's ingest endpoint.
func (p *Pusher) pushOnce(ctx context.Context) error {
	report := p.reporter.BuildReport()

	body, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshaling report: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.serverURL+"/api/v1/ingest", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("posting report: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusTooManyRequests {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	slog.Info("pushed report to server",
		"status", resp.StatusCode,
		"pods", len(report.Pods),
		"node", report.NodeName)

	return nil
}
