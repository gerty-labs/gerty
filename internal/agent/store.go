package agent

import "github.com/gregorytcarroll/k8s-sage/internal/models"

// Store is the rolling window in-memory metric store.
// Placeholder — full implementation in Task 3.
type Store struct{}

// NewStore creates a new Store.
func NewStore() *Store {
	return &Store{}
}

// Record stores a raw metric sample.
func (s *Store) Record(m models.ContainerMetrics) {}

// GetAggregates returns aggregated metrics for all containers.
func (s *Store) GetAggregates() map[string][]models.AggregatedMetrics {
	return nil
}
