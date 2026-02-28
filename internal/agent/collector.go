package agent

import (
	"context"
	"log/slog"
	"time"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
)

// Collector scrapes the kubelet Summary API at a configurable interval
// and feeds raw metrics into the Store.
type Collector struct {
	client   KubeletClient
	store    *Store
	interval time.Duration
}

// NewCollector creates a Collector with a real kubelet HTTP client.
func NewCollector(kubeletURL string, store *Store, interval time.Duration) *Collector {
	return &Collector{
		client:   NewHTTPKubeletClient(kubeletURL),
		store:    store,
		interval: interval,
	}
}

// NewCollectorWithClient creates a Collector with an injected KubeletClient (for testing).
func NewCollectorWithClient(client KubeletClient, store *Store, interval time.Duration) *Collector {
	return &Collector{
		client:   client,
		store:    store,
		interval: interval,
	}
}

// Run starts the collection loop. It blocks until ctx is cancelled.
func (c *Collector) Run(ctx context.Context) {
	slog.Info("collector started", "interval", c.interval)

	// Collect immediately on start, then on interval.
	c.collect(ctx)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("collector stopped")
			return
		case <-ticker.C:
			c.collect(ctx)
		}
	}
}

// collect performs a single scrape of the kubelet Summary API and records metrics.
func (c *Collector) collect(ctx context.Context) {
	summary, err := c.client.GetSummary(ctx)
	if err != nil {
		slog.Error("failed to collect metrics from kubelet", "error", err)
		return
	}

	now := time.Now()
	recorded := 0

	for _, pod := range summary.Pods {
		for _, container := range pod.Containers {
			m := models.ContainerMetrics{
				PodName:       pod.PodRef.Name,
				PodNamespace:  pod.PodRef.Namespace,
				ContainerName: container.Name,
				Timestamp:     now,
			}

			if container.CPU != nil && container.CPU.UsageNanoCores != nil {
				m.CPUUsageNanoCores = *container.CPU.UsageNanoCores
			}

			if container.Memory != nil {
				if container.Memory.UsageBytes != nil {
					m.MemoryUsageBytes = *container.Memory.UsageBytes
				}
				if container.Memory.WorkingSetBytes != nil {
					m.MemoryWorkingSetBytes = *container.Memory.WorkingSetBytes
				}
			}

			c.store.Record(m)
			recorded++
		}
	}

	slog.Debug("metrics collected", "pods", len(summary.Pods), "containers", recorded)
}

// CollectOnce performs a single collection. Useful for testing.
func (c *Collector) CollectOnce(ctx context.Context) {
	c.collect(ctx)
}
