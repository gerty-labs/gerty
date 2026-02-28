package agent

import (
	"context"
	"time"
)

// KubeletClient abstracts access to the kubelet Summary API for testability.
type KubeletClient interface {
	GetSummary(ctx context.Context) (*SummaryResponse, error)
}

// Collector scrapes the kubelet Summary API at a configurable interval
// and feeds raw metrics into the Store.
type Collector struct {
	client   KubeletClient
	store    *Store
	interval time.Duration
}

// NewCollector creates a Collector. For now this creates a placeholder HTTP client;
// full implementation will follow.
func NewCollector(kubeletURL string, store *Store, interval time.Duration) *Collector {
	return &Collector{
		client:   &httpKubeletClient{baseURL: kubeletURL},
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
	// Placeholder — full implementation in Task 2
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Will call c.collect(ctx) once implemented
		}
	}
}
