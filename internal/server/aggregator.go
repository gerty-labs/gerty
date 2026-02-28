package server

import (
	"log/slog"
	"sync"
	"time"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
)

const (
	// maxPodsPerCluster is the upper bound on tracked pods to prevent unbounded growth.
	maxPodsPerCluster = 10_000

	// stalePodTimeout is how long a pod can go unreported before being pruned.
	stalePodTimeout = 15 * time.Minute

	// pruneInterval is how often the background pruner runs.
	pruneInterval = 1 * time.Minute
)

// podEntry is the internal representation of a tracked pod in the cluster store.
type podEntry struct {
	waste    models.PodWaste
	node     string
	lastSeen time.Time
}

// ownerKey uniquely identifies a workload owner within a namespace.
func ownerKey(ref models.OwnerReference) string {
	return ref.Namespace + "/" + ref.Kind + "/" + ref.Name
}

// Aggregator collects NodeReports from agents and maintains a cluster-wide view.
// It is thread-safe for concurrent ingestion from multiple agents.
type Aggregator struct {
	mu    sync.RWMutex
	pods  map[string]*podEntry // keyed by namespace/podName
	nodes map[string]time.Time // node → last report time
	now   func() time.Time     // injectable clock for testing
}

// NewAggregator creates a new Aggregator with an empty cluster state.
func NewAggregator() *Aggregator {
	return &Aggregator{
		pods:  make(map[string]*podEntry),
		nodes: make(map[string]time.Time),
		now:   time.Now,
	}
}

// Ingest processes a NodeReport from an agent, updating the cluster-wide state.
// It returns the number of pods ingested and whether the store is at capacity.
func (a *Aggregator) Ingest(report models.NodeReport) (ingested int, atCapacity bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := a.now()
	a.nodes[report.NodeName] = now

	for i := range report.Pods {
		pod := &report.Pods[i]
		key := pod.PodNamespace + "/" + pod.PodName

		if len(a.pods) >= maxPodsPerCluster {
			if _, exists := a.pods[key]; !exists {
				slog.Warn("cluster store at capacity, dropping new pod",
					"pod", key, "maxPods", maxPodsPerCluster)
				atCapacity = true
				continue
			}
		}

		a.pods[key] = &podEntry{
			waste:    *pod,
			node:     report.NodeName,
			lastSeen: now,
		}
		ingested++
	}

	slog.Debug("ingested node report",
		"node", report.NodeName,
		"pods", ingested,
		"totalTracked", len(a.pods))

	return ingested, atCapacity
}

// PruneStalePods removes pods that haven't been reported within stalePodTimeout.
// Returns the number of pruned entries.
func (a *Aggregator) PruneStalePods() int {
	a.mu.Lock()
	defer a.mu.Unlock()

	cutoff := a.now().Add(-stalePodTimeout)
	pruned := 0

	for key, entry := range a.pods {
		if entry.lastSeen.Before(cutoff) {
			delete(a.pods, key)
			pruned++
		}
	}

	if pruned > 0 {
		slog.Info("pruned stale pods", "pruned", pruned, "remaining", len(a.pods))
	}

	return pruned
}

// StartPruner launches a background goroutine that prunes stale pods on a timer.
// It stops when the provided done channel is closed.
func (a *Aggregator) StartPruner(done <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(pruneInterval)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				a.PruneStalePods()
			}
		}
	}()
}

// ClusterReport builds a full ClusterReport with owner-level aggregation.
func (a *Aggregator) ClusterReport() models.ClusterReport {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.buildReport("")
}

// NamespaceReport builds a report filtered to a single namespace.
// Returns nil if the namespace has no data.
func (a *Aggregator) NamespaceReport(namespace string) *models.NamespaceReport {
	a.mu.RLock()
	defer a.mu.RUnlock()

	report := a.buildReport(namespace)
	nsReport, ok := report.Namespaces[namespace]
	if !ok {
		return nil
	}
	return nsReport
}

// buildReport constructs a ClusterReport, optionally filtering to a single namespace.
// filterNS == "" means include all namespaces. Caller must hold at least a read lock.
func (a *Aggregator) buildReport(filterNS string) models.ClusterReport {
	namespaces := make(map[string]*models.NamespaceReport)
	nodeSet := make(map[string]struct{})
	podCount := 0
	var totalCPUWaste, totalMemWaste float64

	for _, entry := range a.pods {
		pw := entry.waste
		if filterNS != "" && pw.PodNamespace != filterNS {
			continue
		}

		nodeSet[entry.node] = struct{}{}

		nsReport, ok := namespaces[pw.PodNamespace]
		if !ok {
			nsReport = &models.NamespaceReport{
				Namespace: pw.PodNamespace,
			}
			namespaces[pw.PodNamespace] = nsReport
		}

		nsReport.Pods = append(nsReport.Pods, pw)
		nsReport.TotalCPUWasteMillis += pw.TotalCPUWasteMillis
		nsReport.TotalMemWasteBytes += pw.TotalMemWasteBytes
		totalCPUWaste += pw.TotalCPUWasteMillis
		totalMemWaste += pw.TotalMemWasteBytes
		podCount++
	}

	// Build owner-level aggregation for each namespace.
	for _, nsReport := range namespaces {
		nsReport.Owners = aggregateByOwner(nsReport.Pods)
	}

	return models.ClusterReport{
		ReportTime:          a.now(),
		NodeCount:           len(nodeSet),
		PodCount:            podCount,
		Namespaces:          namespaces,
		TotalCPUWasteMillis: totalCPUWaste,
		TotalMemWasteBytes:  totalMemWaste,
	}
}

// aggregateByOwner groups pods by their OwnerReference and merges container waste.
func aggregateByOwner(pods []models.PodWaste) []models.OwnerWaste {
	type ownerAccum struct {
		owner      models.OwnerReference
		podCount   int
		containers map[string]*models.ContainerWaste // keyed by container name
		cpuWaste   float64
		memWaste   float64
	}

	accum := make(map[string]*ownerAccum)

	for _, pod := range pods {
		ref := pod.OwnerRef
		if ref.Kind == "" {
			// Standalone pod — use pod itself as the owner.
			ref = models.OwnerReference{
				Kind:      "Pod",
				Name:      pod.PodName,
				Namespace: pod.PodNamespace,
			}
		}

		key := ownerKey(ref)
		oa, ok := accum[key]
		if !ok {
			oa = &ownerAccum{
				owner:      ref,
				containers: make(map[string]*models.ContainerWaste),
			}
			accum[key] = oa
		}

		oa.podCount++
		oa.cpuWaste += pod.TotalCPUWasteMillis
		oa.memWaste += pod.TotalMemWasteBytes

		for _, cw := range pod.Containers {
			existing, found := oa.containers[cw.ContainerName]
			if !found {
				cwCopy := cw
				oa.containers[cw.ContainerName] = &cwCopy
			} else {
				mergeContainerWaste(existing, cw, oa.podCount)
			}
		}
	}

	owners := make([]models.OwnerWaste, 0, len(accum))
	for _, oa := range accum {
		containers := make([]models.ContainerWaste, 0, len(oa.containers))
		for _, cw := range oa.containers {
			containers = append(containers, *cw)
		}
		owners = append(owners, models.OwnerWaste{
			Owner:               oa.owner,
			PodCount:            oa.podCount,
			Containers:          containers,
			TotalCPUWasteMillis: oa.cpuWaste,
			TotalMemWasteBytes:  oa.memWaste,
		})
	}

	return owners
}

// mergeContainerWaste updates an accumulated ContainerWaste with data from an
// additional replica's container. Uses conservative max for percentiles.
func mergeContainerWaste(dst *models.ContainerWaste, src models.ContainerWaste, totalReplicas int) {
	dst.CPUUsage = mergeMetricAggregate(dst.CPUUsage, src.CPUUsage)
	dst.MemoryUsage = mergeMetricAggregate(dst.MemoryUsage, src.MemoryUsage)
	dst.CPUWasteMillis += src.CPUWasteMillis
	dst.MemWasteBytes += src.MemWasteBytes

	// Recalculate percentages based on accumulated waste.
	if dst.CPURequestMillis > 0 {
		avgWaste := dst.CPUWasteMillis / float64(totalReplicas)
		dst.CPUWastePercent = (avgWaste / float64(dst.CPURequestMillis)) * 100
	}
	if dst.MemoryRequestBytes > 0 {
		avgWaste := dst.MemWasteBytes / float64(totalReplicas)
		dst.MemWastePercent = (avgWaste / float64(dst.MemoryRequestBytes)) * 100
	}

	if src.RestartCount > dst.RestartCount {
		dst.RestartCount = src.RestartCount
	}
	if src.DataWindow > dst.DataWindow {
		dst.DataWindow = src.DataWindow
	}
}

// mergeMetricAggregate conservatively merges two MetricAggregates by taking
// the max of each percentile (worst-case view across replicas).
func mergeMetricAggregate(a, b models.MetricAggregate) models.MetricAggregate {
	return models.MetricAggregate{
		P50: maxFloat(a.P50, b.P50),
		P95: maxFloat(a.P95, b.P95),
		P99: maxFloat(a.P99, b.P99),
		Max: maxFloat(a.Max, b.Max),
	}
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// PodCount returns the number of currently tracked pods.
func (a *Aggregator) PodCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.pods)
}

// NodeCount returns the number of nodes that have reported.
func (a *Aggregator) NodeCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.nodes)
}
