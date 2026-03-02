package slack

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
	"github.com/gregorytcarroll/k8s-sage/internal/rules"
)

const (
	// defaultDigestInterval is how often to send a digest notification.
	defaultDigestInterval = 1 * time.Hour

	// dedupWindow is how long to suppress duplicate recommendation notifications.
	dedupWindow = 7 * 24 * time.Hour
)

// ReportSource provides cluster report data. Implemented by server.Aggregator.
type ReportSource interface {
	ClusterReport() models.ClusterReport
}

// Config holds Slack notifier configuration.
type Config struct {
	WebhookURL     string
	Channel        string
	DigestInterval time.Duration
	MinSeverity    Severity
}

// Notifier periodically checks for recommendations and sends Slack notifications.
type Notifier struct {
	webhook  *WebhookClient
	source   ReportSource
	engine   *rules.Engine
	config   Config
	seen     map[string]time.Time // key → last notified time
	mu       sync.Mutex
	now      func() time.Time // injectable clock for testing
}

// NewNotifier creates a Notifier that sends recommendation digests to Slack.
func NewNotifier(config Config, source ReportSource, engine *rules.Engine) *Notifier {
	interval := config.DigestInterval
	if interval == 0 {
		interval = defaultDigestInterval
	}
	config.DigestInterval = interval

	return &Notifier{
		webhook: NewWebhookClient(config.WebhookURL),
		source:  source,
		engine:  engine,
		config:  config,
		seen:    make(map[string]time.Time),
		now:     time.Now,
	}
}

// Run starts the notification loop, blocking until ctx is cancelled.
func (n *Notifier) Run(ctx context.Context) {
	ticker := time.NewTicker(n.config.DigestInterval)
	defer ticker.Stop()

	slog.Info("slack notifier started",
		"interval", n.config.DigestInterval,
		"channel", n.config.Channel,
		"minSeverity", n.config.MinSeverity)

	for {
		select {
		case <-ctx.Done():
			slog.Info("slack notifier stopped")
			return
		case <-ticker.C:
			if err := n.sendDigest(ctx); err != nil {
				slog.Warn("slack digest failed", "error", err)
			}
		}
	}
}

// sendDigest builds a cluster report, filters recommendations, and sends a digest.
func (n *Notifier) sendDigest(ctx context.Context) error {
	report := n.source.ClusterReport()
	recs := n.engine.AnalyzeCluster(report)

	if len(recs) == 0 {
		slog.Debug("slack notifier: no recommendations to send")
		return nil
	}

	// Filter by severity and dedup.
	var filtered []models.Recommendation
	now := n.now()
	for _, rec := range recs {
		severity := ClassifySeverity(rec)
		if !meetsMinSeverity(severity, n.config.MinSeverity) {
			continue
		}

		key := RecKey(rec)
		if n.isDuplicate(key, now) {
			continue
		}

		filtered = append(filtered, rec)
		n.markSeen(key, now)
	}

	// Prune stale entries from the dedup map.
	n.pruneSeen(now)

	if len(filtered) == 0 {
		slog.Debug("slack notifier: all recommendations filtered or deduped")
		return nil
	}

	payload := BuildDigestMessage(filtered)
	payload.Channel = n.config.Channel

	slog.Info("sending slack digest", "recommendations", len(filtered))
	return n.webhook.Send(ctx, payload)
}

// isDuplicate checks if a recommendation key was notified within the dedup window.
func (n *Notifier) isDuplicate(key string, now time.Time) bool {
	n.mu.Lock()
	defer n.mu.Unlock()

	lastSeen, ok := n.seen[key]
	if !ok {
		return false
	}
	return now.Sub(lastSeen) < dedupWindow
}

// markSeen records when a recommendation was last notified.
func (n *Notifier) markSeen(key string, now time.Time) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.seen[key] = now
}

// pruneSeen removes entries older than the dedup window to prevent unbounded growth.
func (n *Notifier) pruneSeen(now time.Time) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for key, ts := range n.seen {
		if now.Sub(ts) > dedupWindow {
			delete(n.seen, key)
		}
	}
}

// meetsMinSeverity returns true if severity meets the minimum threshold.
func meetsMinSeverity(severity, min Severity) bool {
	order := map[Severity]int{
		SeverityCritical:      3,
		SeverityOptimisation:  2,
		SeverityInformational: 1,
	}
	return order[severity] >= order[min]
}
