package agent

import (
	"testing"
	"time"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_Record_CreatesNewContainer(t *testing.T) {
	store := NewStore()

	store.Record(models.ContainerMetrics{
		PodName:            "nginx-abc",
		PodNamespace:       "default",
		ContainerName:      "nginx",
		Timestamp:          time.Now(),
		CPUUsageNanoCores:  100_000_000,
		MemoryUsageBytes:   50_000_000,
		MemoryWorkingSetBytes: 40_000_000,
	})

	keys := store.ContainerKeys()
	assert.Len(t, keys, 1)
	assert.Equal(t, "default/nginx-abc/nginx", keys[0])
}

func TestStore_Record_UpdatesMetadata(t *testing.T) {
	store := NewStore()

	store.Record(models.ContainerMetrics{
		PodName:          "nginx-abc",
		PodNamespace:     "default",
		ContainerName:    "nginx",
		Timestamp:        time.Now(),
		CPURequestMillis: 500,
		MemoryRequestBytes: 256_000_000,
		RestartCount:     2,
		QoSClass:         "Burstable",
	})

	meta, ok := store.GetContainerMeta("default/nginx-abc/nginx")
	require.True(t, ok)
	assert.Equal(t, int64(500), meta.CPURequestMillis)
	assert.Equal(t, int64(256_000_000), meta.MemRequestBytes)
	assert.Equal(t, int32(2), meta.RestartCount)
	assert.Equal(t, "Burstable", meta.QoSClass)
}

func TestStore_GetContainerSummary_SingleSample(t *testing.T) {
	store := NewStore()

	store.Record(models.ContainerMetrics{
		PodName:               "pod1",
		PodNamespace:          "ns1",
		ContainerName:         "c1",
		Timestamp:             time.Now(),
		CPUUsageNanoCores:     200_000_000,
		MemoryWorkingSetBytes: 100_000_000,
	})

	summary, ok := store.GetContainerSummary("ns1/pod1/c1")
	require.True(t, ok)
	// Single sample: all percentiles should equal the sample value.
	assert.Equal(t, float64(200_000_000), summary.CPUNanoCores.P50)
	assert.Equal(t, float64(200_000_000), summary.CPUNanoCores.P95)
	assert.Equal(t, float64(200_000_000), summary.CPUNanoCores.Max)
	assert.Equal(t, float64(100_000_000), summary.MemoryWorkingSet.P50)
}

func TestStore_GetContainerSummary_NotFound(t *testing.T) {
	store := NewStore()

	_, ok := store.GetContainerSummary("ns1/pod1/c1")
	assert.False(t, ok)
}

func TestStore_Percentile_Calculation(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		p      float64
		want   float64
	}{
		{
			name:   "single value",
			values: []float64{42},
			p:      0.50,
			want:   42,
		},
		{
			name:   "two values P50",
			values: []float64{10, 20},
			p:      0.50,
			want:   10,
		},
		{
			name:   "two values P95",
			values: []float64{10, 20},
			p:      0.95,
			want:   20,
		},
		{
			name:   "ten values P50",
			values: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			p:      0.50,
			want:   5,
		},
		{
			name:   "ten values P95",
			values: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			p:      0.95,
			want:   10,
		},
		{
			name:   "ten values P99",
			values: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			p:      0.99,
			want:   10,
		},
		{
			name:   "empty",
			values: []float64{},
			p:      0.50,
			want:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := percentile(tt.values, tt.p)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestStore_ComputeAggregate(t *testing.T) {
	values := []float64{100, 200, 50, 300, 150, 80, 250, 170, 90, 400}

	agg := computeAggregate(values)

	// Values sorted: 50, 80, 90, 100, 150, 170, 200, 250, 300, 400
	assert.Equal(t, float64(150), agg.P50) // ceil(0.5*10)-1 = 4 → 150
	assert.Equal(t, float64(400), agg.P95) // ceil(0.95*10)-1 = 9 → 400
	assert.Equal(t, float64(400), agg.P99) // ceil(0.99*10)-1 = 9 → 400
	assert.Equal(t, float64(400), agg.Max)
}

func TestStore_Aggregation_AfterBufferExpiry(t *testing.T) {
	now := time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC)
	store := NewStore()
	store.now = func() time.Time { return now }

	// Record samples at "10 minutes ago" (well outside the 5-min buffer).
	oldTime := now.Add(-10 * time.Minute)
	for i := 0; i < 5; i++ {
		store.Record(models.ContainerMetrics{
			PodName:           "pod1",
			PodNamespace:      "ns1",
			ContainerName:     "c1",
			Timestamp:         oldTime.Add(time.Duration(i) * 10 * time.Second),
			CPUUsageNanoCores: uint64(100_000_000 + i*10_000_000),
		})
	}

	// The old samples should have been aggregated into a fine bucket.
	store.mu.RLock()
	ts := store.containers["ns1/pod1/c1"]
	store.mu.RUnlock()

	require.NotNil(t, ts)
	assert.Len(t, ts.rawSamples, 0, "old samples should be aggregated out of raw buffer")
	assert.Greater(t, len(ts.fineBuckets), 0, "should have at least one fine bucket")
}

func TestStore_Downsampling_FineToCoarse(t *testing.T) {
	now := time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC)
	store := NewStore()
	store.now = func() time.Time { return now }

	// Record a sample from 25 hours ago (beyond fine retention of 24h).
	oldTime := now.Add(-25 * time.Hour)
	store.Record(models.ContainerMetrics{
		PodName:           "pod1",
		PodNamespace:      "ns1",
		ContainerName:     "c1",
		Timestamp:         oldTime,
		CPUUsageNanoCores: 200_000_000,
	})

	store.mu.RLock()
	ts := store.containers["ns1/pod1/c1"]
	store.mu.RUnlock()

	require.NotNil(t, ts)
	// The sample is old enough to go through aggregation → fine → coarse.
	assert.Len(t, ts.fineBuckets, 0, "sample should have been downsampled from fine")
	assert.Greater(t, len(ts.coarseBuckets), 0, "should have at least one coarse bucket")
}

func TestStore_Eviction_CoarseBucketsOlderThan7Days(t *testing.T) {
	now := time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC)
	store := NewStore()
	store.now = func() time.Time { return now }

	// Record a sample from 8 days ago (beyond coarse retention of 7d).
	oldTime := now.Add(-8 * 24 * time.Hour)
	store.Record(models.ContainerMetrics{
		PodName:           "pod1",
		PodNamespace:      "ns1",
		ContainerName:     "c1",
		Timestamp:         oldTime,
		CPUUsageNanoCores: 200_000_000,
	})

	store.mu.RLock()
	ts := store.containers["ns1/pod1/c1"]
	store.mu.RUnlock()

	require.NotNil(t, ts)
	assert.Len(t, ts.coarseBuckets, 0, "8-day old data should be evicted")
}

func TestStore_DataWindow(t *testing.T) {
	now := time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC)
	store := NewStore()
	store.now = func() time.Time { return now }

	key := "ns1/pod1/c1"

	// No data yet.
	assert.Equal(t, time.Duration(0), store.DataWindow(key))

	// Record two samples 10 minutes apart.
	store.Record(models.ContainerMetrics{
		PodName:           "pod1",
		PodNamespace:      "ns1",
		ContainerName:     "c1",
		Timestamp:         now.Add(-10 * time.Minute),
		CPUUsageNanoCores: 100_000_000,
	})
	store.Record(models.ContainerMetrics{
		PodName:           "pod1",
		PodNamespace:      "ns1",
		ContainerName:     "c1",
		Timestamp:         now,
		CPUUsageNanoCores: 200_000_000,
	})

	window := store.DataWindow(key)
	assert.Greater(t, window, time.Duration(0))
}

func TestStore_ConcurrentAccess(t *testing.T) {
	store := NewStore()
	done := make(chan struct{})

	// Writer goroutine.
	go func() {
		for i := 0; i < 100; i++ {
			store.Record(models.ContainerMetrics{
				PodName:           "pod1",
				PodNamespace:      "ns1",
				ContainerName:     "c1",
				Timestamp:         time.Now(),
				CPUUsageNanoCores: uint64(i * 1_000_000),
			})
		}
		close(done)
	}()

	// Reader goroutine — should not race.
	for i := 0; i < 50; i++ {
		store.ContainerKeys()
		store.GetContainerSummary("ns1/pod1/c1")
		store.GetContainerMeta("ns1/pod1/c1")
	}

	<-done
}

func TestContainerKey(t *testing.T) {
	assert.Equal(t, "default/nginx/container", models.ContainerKey("default", "nginx", "container"))
	assert.Equal(t, "production/api-server/main", models.ContainerKey("production", "api-server", "main"))
}
