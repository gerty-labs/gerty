package server

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeReport(nodeName string, pods ...models.PodWaste) models.NodeReport {
	return models.NodeReport{
		NodeName:   nodeName,
		ReportTime: time.Now(),
		Pods:       pods,
	}
}

func makePod(ns, name string, cpuWaste, memWaste float64) models.PodWaste {
	return models.PodWaste{
		PodName:      name,
		PodNamespace: ns,
		QoSClass:     "Burstable",
		Containers: []models.ContainerWaste{
			{
				ContainerName:    "main",
				CPURequestMillis: 1000,
				CPUUsage:         models.MetricAggregate{P50: 100, P95: 200, P99: 300, Max: 400},
				CPUWasteMillis:   cpuWaste,
				CPUWastePercent:  cpuWaste / 10, // simplified
				MemoryRequestBytes: 1_000_000_000,
				MemoryUsage:        models.MetricAggregate{P50: 100_000_000, P95: 200_000_000, P99: 300_000_000, Max: 400_000_000},
				MemWasteBytes:      memWaste,
				MemWastePercent:    memWaste / 10_000_000, // simplified
				DataWindow:         24 * time.Hour,
			},
		},
		TotalCPUWasteMillis: cpuWaste,
		TotalMemWasteBytes:  memWaste,
	}
}

func makePodWithOwner(ns, name string, owner models.OwnerReference) models.PodWaste {
	pod := makePod(ns, name, 500, 500_000_000)
	pod.OwnerRef = owner
	return pod
}

func TestAggregator_Ingest_SingleReport(t *testing.T) {
	agg := NewAggregator()

	report := makeReport("node-1",
		makePod("default", "nginx-abc", 800, 400_000_000),
		makePod("production", "api-def", 500, 200_000_000),
	)

	ingested, atCapacity := agg.Ingest(report)

	assert.Equal(t, 2, ingested)
	assert.False(t, atCapacity)
	assert.Equal(t, 2, agg.PodCount())
	assert.Equal(t, 1, agg.NodeCount())
}

func TestAggregator_Ingest_MultipleNodes(t *testing.T) {
	agg := NewAggregator()

	agg.Ingest(makeReport("node-1", makePod("default", "pod-a", 100, 100)))
	agg.Ingest(makeReport("node-2", makePod("default", "pod-b", 200, 200)))

	assert.Equal(t, 2, agg.PodCount())
	assert.Equal(t, 2, agg.NodeCount())
}

func TestAggregator_Ingest_UpdateExistingPod(t *testing.T) {
	agg := NewAggregator()

	pod := makePod("default", "nginx-abc", 800, 400_000_000)
	agg.Ingest(makeReport("node-1", pod))
	assert.Equal(t, 1, agg.PodCount())

	// Same pod with updated waste.
	pod.TotalCPUWasteMillis = 500
	agg.Ingest(makeReport("node-1", pod))
	assert.Equal(t, 1, agg.PodCount()) // Still 1, not duplicated.

	report := agg.ClusterReport()
	ns := report.Namespaces["default"]
	require.NotNil(t, ns)
	assert.Equal(t, float64(500), ns.Pods[0].TotalCPUWasteMillis)
}

func TestAggregator_Ingest_DuplicateReportsOverwrite(t *testing.T) {
	agg := NewAggregator()

	report := makeReport("node-1", makePod("default", "nginx", 100, 100))

	// Ingest the same report 5 times.
	for i := 0; i < 5; i++ {
		agg.Ingest(report)
	}

	assert.Equal(t, 1, agg.PodCount(), "duplicate reports should not create extra entries")
}

func TestAggregator_Ingest_AtCapacity(t *testing.T) {
	agg := NewAggregator()
	now := time.Now()

	// Directly fill the map to capacity to avoid allocating 10k pod structs
	// through the Ingest path (which OOMs under race detection).
	agg.mu.Lock()
	for i := 0; i < maxPodsPerCluster; i++ {
		key := fmt.Sprintf("ns/filled-pod-%d", i)
		agg.pods[key] = &podEntry{
			waste:    models.PodWaste{PodName: fmt.Sprintf("filled-pod-%d", i), PodNamespace: "ns"},
			node:     "node-fill",
			lastSeen: now,
		}
	}
	agg.mu.Unlock()

	assert.Equal(t, maxPodsPerCluster, agg.PodCount())

	// Now try to add one more new pod via Ingest.
	_, atCapacity := agg.Ingest(makeReport("node-extra",
		makePod("overflow", "extra-pod", 1, 1),
	))

	assert.True(t, atCapacity)
}

func TestAggregator_Ingest_ExistingPodAtCapacityStillUpdates(t *testing.T) {
	agg := NewAggregator()
	now := time.Now()

	// Fill to capacity directly.
	agg.mu.Lock()
	for i := 0; i < maxPodsPerCluster; i++ {
		key := fmt.Sprintf("ns/filled-pod-%d", i)
		agg.pods[key] = &podEntry{
			waste:    models.PodWaste{PodName: fmt.Sprintf("filled-pod-%d", i), PodNamespace: "ns", TotalCPUWasteMillis: 0},
			node:     "node-fill",
			lastSeen: now,
		}
	}
	agg.mu.Unlock()

	// Update an existing pod — should succeed even at capacity.
	updated := makePod("ns", "filled-pod-0", 999, 999)
	ingested, _ := agg.Ingest(makeReport("node-fill", updated))

	assert.Equal(t, 1, ingested, "existing pods should still be updatable at capacity")
}

func TestAggregator_PruneStalePods(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	agg := NewAggregator()
	agg.now = func() time.Time { return now }

	agg.Ingest(makeReport("node-1", makePod("default", "fresh-pod", 100, 100)))

	// Manually set lastSeen to 20 minutes ago.
	agg.mu.Lock()
	for _, entry := range agg.pods {
		entry.lastSeen = now.Add(-20 * time.Minute)
	}
	agg.mu.Unlock()

	pruned := agg.PruneStalePods()
	assert.Equal(t, 1, pruned)
	assert.Equal(t, 0, agg.PodCount())
}

func TestAggregator_PruneStalePods_KeepsFreshPods(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	agg := NewAggregator()
	agg.now = func() time.Time { return now }

	agg.Ingest(makeReport("node-1",
		makePod("default", "fresh-pod", 100, 100),
		makePod("default", "stale-pod", 200, 200),
	))

	// Make only the stale pod old.
	agg.mu.Lock()
	agg.pods["default/stale-pod"].lastSeen = now.Add(-20 * time.Minute)
	agg.mu.Unlock()

	pruned := agg.PruneStalePods()
	assert.Equal(t, 1, pruned)
	assert.Equal(t, 1, agg.PodCount())
}

func TestAggregator_ClusterReport_Empty(t *testing.T) {
	agg := NewAggregator()
	report := agg.ClusterReport()

	assert.Equal(t, 0, report.NodeCount)
	assert.Equal(t, 0, report.PodCount)
	assert.Empty(t, report.Namespaces)
	assert.Equal(t, float64(0), report.TotalCPUWasteMillis)
}

func TestAggregator_ClusterReport_AggregatesWaste(t *testing.T) {
	agg := NewAggregator()

	agg.Ingest(makeReport("node-1",
		makePod("default", "pod-a", 800, 400_000_000),
		makePod("default", "pod-b", 200, 100_000_000),
		makePod("production", "pod-c", 500, 300_000_000),
	))

	report := agg.ClusterReport()

	assert.Equal(t, 1, report.NodeCount)
	assert.Equal(t, 3, report.PodCount)
	assert.Len(t, report.Namespaces, 2)
	assert.Equal(t, float64(1500), report.TotalCPUWasteMillis)
	assert.Equal(t, float64(800_000_000), report.TotalMemWasteBytes)

	defaultNS := report.Namespaces["default"]
	require.NotNil(t, defaultNS)
	assert.Equal(t, float64(1000), defaultNS.TotalCPUWasteMillis)
	assert.Len(t, defaultNS.Pods, 2)
}

func TestAggregator_NamespaceReport_Found(t *testing.T) {
	agg := NewAggregator()

	agg.Ingest(makeReport("node-1",
		makePod("default", "pod-a", 100, 100),
		makePod("production", "pod-b", 200, 200),
	))

	nsReport := agg.NamespaceReport("production")
	require.NotNil(t, nsReport)
	assert.Equal(t, "production", nsReport.Namespace)
	assert.Len(t, nsReport.Pods, 1)
	assert.Equal(t, float64(200), nsReport.TotalCPUWasteMillis)
}

func TestAggregator_NamespaceReport_NotFound(t *testing.T) {
	agg := NewAggregator()
	agg.Ingest(makeReport("node-1", makePod("default", "pod-a", 100, 100)))

	nsReport := agg.NamespaceReport("nonexistent")
	assert.Nil(t, nsReport)
}

func TestAggregator_OwnerAggregation(t *testing.T) {
	agg := NewAggregator()

	owner := models.OwnerReference{Kind: "Deployment", Name: "api", Namespace: "prod"}

	agg.Ingest(makeReport("node-1",
		makePodWithOwner("prod", "api-abc", owner),
		makePodWithOwner("prod", "api-def", owner),
		makePodWithOwner("prod", "api-ghi", owner),
	))

	report := agg.ClusterReport()
	nsReport := report.Namespaces["prod"]
	require.NotNil(t, nsReport)
	require.Len(t, nsReport.Owners, 1)

	ownerWaste := nsReport.Owners[0]
	assert.Equal(t, "Deployment", ownerWaste.Owner.Kind)
	assert.Equal(t, "api", ownerWaste.Owner.Name)
	assert.Equal(t, 3, ownerWaste.PodCount)
	assert.Equal(t, float64(1500), ownerWaste.TotalCPUWasteMillis)
}

func TestAggregator_OwnerAggregation_StandalonePod(t *testing.T) {
	agg := NewAggregator()

	// Pod with no owner.
	pod := makePod("default", "standalone-pod", 100, 100)
	agg.Ingest(makeReport("node-1", pod))

	report := agg.ClusterReport()
	nsReport := report.Namespaces["default"]
	require.NotNil(t, nsReport)
	require.Len(t, nsReport.Owners, 1)

	ownerWaste := nsReport.Owners[0]
	assert.Equal(t, "Pod", ownerWaste.Owner.Kind)
	assert.Equal(t, "standalone-pod", ownerWaste.Owner.Name)
	assert.Equal(t, 1, ownerWaste.PodCount)
}

func TestAggregator_ConcurrentIngestion(t *testing.T) {
	agg := NewAggregator()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(nodeID int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				pod := makePod("ns", "pod-from-goroutine", float64(j), float64(j))
				agg.Ingest(makeReport("node-concurrent", pod))
			}
		}(i)
	}

	// Also read concurrently.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			agg.ClusterReport()
			agg.NamespaceReport("ns")
			agg.PodCount()
			agg.NodeCount()
		}()
	}

	wg.Wait()
	// If we get here without a race condition panic, the test passes.
	assert.GreaterOrEqual(t, agg.PodCount(), 1)
}
