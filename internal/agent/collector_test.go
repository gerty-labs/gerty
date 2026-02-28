package agent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockKubeletClient implements KubeletClient for testing.
type mockKubeletClient struct {
	response *SummaryResponse
	err      error
	calls    int
}

func (m *mockKubeletClient) GetSummary(ctx context.Context) (*SummaryResponse, error) {
	m.calls++
	return m.response, m.err
}

func uint64Ptr(v uint64) *uint64 { return &v }

func newTestSummary() *SummaryResponse {
	return &SummaryResponse{
		Node: NodeStats{NodeName: "test-node"},
		Pods: []PodStats{
			{
				PodRef: PodReference{Name: "nginx-abc123", Namespace: "default"},
				Containers: []ContainerStats{
					{
						Name: "nginx",
						CPU: &CPUStats{
							Time:           time.Now(),
							UsageNanoCores: uint64Ptr(150_000_000), // 150m
						},
						Memory: &MemStats{
							Time:            time.Now(),
							UsageBytes:      uint64Ptr(104_857_600),  // 100Mi
							WorkingSetBytes: uint64Ptr(83_886_080),   // 80Mi
						},
					},
				},
			},
			{
				PodRef: PodReference{Name: "api-def456", Namespace: "production"},
				Containers: []ContainerStats{
					{
						Name: "api",
						CPU: &CPUStats{
							Time:           time.Now(),
							UsageNanoCores: uint64Ptr(500_000_000), // 500m
						},
						Memory: &MemStats{
							Time:            time.Now(),
							UsageBytes:      uint64Ptr(524_288_000),  // 500Mi
							WorkingSetBytes: uint64Ptr(419_430_400),  // 400Mi
						},
					},
					{
						Name: "sidecar",
						CPU: &CPUStats{
							Time:           time.Now(),
							UsageNanoCores: uint64Ptr(10_000_000), // 10m
						},
						Memory: &MemStats{
							Time:            time.Now(),
							UsageBytes:      uint64Ptr(33_554_432),   // 32Mi
							WorkingSetBytes: uint64Ptr(25_165_824),   // 24Mi
						},
					},
				},
			},
		},
	}
}

func TestCollector_CollectOnce_RecordsAllContainers(t *testing.T) {
	store := NewStore()
	mock := &mockKubeletClient{response: newTestSummary()}
	collector := NewCollectorWithClient(mock, store, 30*time.Second)

	collector.CollectOnce(context.Background())

	assert.Equal(t, 1, mock.calls)

	keys := store.ContainerKeys()
	assert.Len(t, keys, 3)
	assert.Contains(t, keys, "default/nginx-abc123/nginx")
	assert.Contains(t, keys, "production/api-def456/api")
	assert.Contains(t, keys, "production/api-def456/sidecar")
}

func TestCollector_CollectOnce_ExtractsCorrectValues(t *testing.T) {
	store := NewStore()
	mock := &mockKubeletClient{response: newTestSummary()}
	collector := NewCollectorWithClient(mock, store, 30*time.Second)

	collector.CollectOnce(context.Background())

	// Check that raw samples contain the correct CPU value for nginx.
	key := "default/nginx-abc123/nginx"
	summary, ok := store.GetContainerSummary(key)
	require.True(t, ok)
	// Raw samples haven't been aggregated yet (within 5min buffer), so
	// the live aggregate should show our single sample value.
	assert.Equal(t, float64(150_000_000), summary.CPUNanoCores.P50)
	assert.Equal(t, float64(83_886_080), summary.MemoryWorkingSet.P50)
}

func TestCollector_CollectOnce_HandlesNilCPU(t *testing.T) {
	summary := &SummaryResponse{
		Node: NodeStats{NodeName: "test-node"},
		Pods: []PodStats{
			{
				PodRef: PodReference{Name: "no-cpu-pod", Namespace: "default"},
				Containers: []ContainerStats{
					{
						Name:   "container",
						CPU:    nil,
						Memory: &MemStats{WorkingSetBytes: uint64Ptr(1000)},
					},
				},
			},
		},
	}

	store := NewStore()
	mock := &mockKubeletClient{response: summary}
	collector := NewCollectorWithClient(mock, store, 30*time.Second)

	// Should not panic with nil CPU.
	collector.CollectOnce(context.Background())

	keys := store.ContainerKeys()
	require.Len(t, keys, 1)
	assert.Equal(t, "default/no-cpu-pod/container", keys[0])

	// CPU should be recorded as zero when nil.
	s, ok := store.GetContainerSummary("default/no-cpu-pod/container")
	require.True(t, ok)
	assert.Equal(t, float64(0), s.CPUNanoCores.P50, "CPU should be zero when kubelet returns nil CPU")
	// Memory working set should be recorded correctly.
	assert.Equal(t, float64(1000), s.MemoryWorkingSet.P50)
}

func TestCollector_CollectOnce_HandlesError(t *testing.T) {
	store := NewStore()
	mock := &mockKubeletClient{
		response: nil,
		err:      assert.AnError,
	}
	collector := NewCollectorWithClient(mock, store, 30*time.Second)

	// Should not panic, should log error internally.
	collector.CollectOnce(context.Background())

	keys := store.ContainerKeys()
	assert.Len(t, keys, 0)
}

func TestCollector_Run_RespectsContextCancellation(t *testing.T) {
	store := NewStore()
	mock := &mockKubeletClient{response: newTestSummary()}
	collector := NewCollectorWithClient(mock, store, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	collector.Run(ctx)

	// Should have collected at least once (immediate) plus potentially more on tick.
	assert.GreaterOrEqual(t, mock.calls, 1)
}

func TestCollector_MultipleScrapes_AccumulateSamples(t *testing.T) {
	store := NewStore()
	mock := &mockKubeletClient{response: newTestSummary()}
	collector := NewCollectorWithClient(mock, store, 30*time.Second)

	// Scrape 5 times.
	for i := 0; i < 5; i++ {
		collector.CollectOnce(context.Background())
	}

	assert.Equal(t, 5, mock.calls)
	// All 3 containers should still be tracked.
	assert.Len(t, store.ContainerKeys(), 3)
}
