package agent

import (
	"context"
	"log/slog"
	"strings"
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

// podResourceInfo holds resource requests/limits and metadata for a container.
type podResourceInfo struct {
	cpuRequestMillis int64
	cpuLimitMillis   int64
	memRequestBytes  int64
	memLimitBytes    int64
	restartCount     int32
	qosClass         string
	ownerKind        string
	ownerName        string
}

// collect performs a single scrape of the kubelet Summary API and records metrics.
func (c *Collector) collect(ctx context.Context) {
	summary, err := c.client.GetSummary(ctx)
	if err != nil {
		slog.Error("failed to collect metrics from kubelet", "error", err)
		return
	}

	// Fetch pod specs to get resource requests/limits.
	resourceMap := c.fetchPodResources(ctx)

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

			// Merge resource requests/limits from pod spec.
			key := pod.PodRef.Namespace + "/" + pod.PodRef.Name + "/" + container.Name
			if info, ok := resourceMap[key]; ok {
				m.CPURequestMillis = info.cpuRequestMillis
				m.CPULimitMillis = info.cpuLimitMillis
				m.MemoryRequestBytes = info.memRequestBytes
				m.MemoryLimitBytes = info.memLimitBytes
				m.RestartCount = info.restartCount
				m.QoSClass = info.qosClass
				m.OwnerKind = info.ownerKind
				m.OwnerName = info.ownerName
			}

			c.store.Record(m)
			recorded++
		}
	}

	slog.Debug("metrics collected", "pods", len(summary.Pods), "containers", recorded)
}

// fetchPodResources calls the kubelet /pods endpoint to get resource requests/limits.
// Returns a map keyed by namespace/pod/container.
func (c *Collector) fetchPodResources(ctx context.Context) map[string]podResourceInfo {
	result := make(map[string]podResourceInfo)

	podList, err := c.client.GetPods(ctx)
	if err != nil {
		slog.Warn("failed to fetch pod specs from kubelet", "error", err)
		return result
	}

	for _, pod := range podList.Items {
		// Build restart count map from status.
		restartCounts := make(map[string]int32)
		for _, cs := range pod.Status.ContainerStatuses {
			restartCounts[cs.Name] = cs.RestartCount
		}

		// Resolve the controlling owner (e.g. ReplicaSet → Deployment).
		ownerKind, ownerName := resolveOwner(pod.Metadata.OwnerReferences)

		for _, container := range pod.Spec.Containers {
			key := pod.Metadata.Namespace + "/" + pod.Metadata.Name + "/" + container.Name
			result[key] = podResourceInfo{
				cpuRequestMillis: parseCPUToMillis(container.Resources.Requests.CPU),
				cpuLimitMillis:   parseCPUToMillis(container.Resources.Limits.CPU),
				memRequestBytes:  parseMemoryToBytes(container.Resources.Requests.Memory),
				memLimitBytes:    parseMemoryToBytes(container.Resources.Limits.Memory),
				restartCount:     restartCounts[container.Name],
				qosClass:         pod.Status.QOSClass,
				ownerKind:        ownerKind,
				ownerName:        ownerName,
			}
		}
	}

	return result
}

// CollectOnce performs a single collection. Useful for testing.
func (c *Collector) CollectOnce(ctx context.Context) {
	c.collect(ctx)
}

// resolveOwner extracts the controlling owner from a pod's OwnerReferences and
// resolves ReplicaSet owners to their parent Deployment using the naming convention.
func resolveOwner(refs []PodOwnerReference) (kind, name string) {
	// Prefer the controlling owner reference.
	for _, ref := range refs {
		if ref.Controller != nil && *ref.Controller {
			kind = ref.Kind
			name = ref.Name
			break
		}
	}
	// Fallback to the first reference if no controller is marked.
	if kind == "" && len(refs) > 0 {
		kind = refs[0].Kind
		name = refs[0].Name
	}

	// Resolve ReplicaSet → Deployment by stripping the pod-template-hash suffix.
	// K8s naming convention: ReplicaSet name = "<deployment>-<pod-template-hash>".
	if kind == "ReplicaSet" {
		if depName := resolveDeploymentName(name); depName != "" {
			kind = "Deployment"
			name = depName
		}
	}

	return kind, name
}

// resolveDeploymentName strips the trailing pod-template-hash from a ReplicaSet name
// to derive the parent Deployment name. Returns "" if the name has no hyphen.
func resolveDeploymentName(rsName string) string {
	idx := strings.LastIndex(rsName, "-")
	if idx <= 0 {
		return ""
	}
	return rsName[:idx]
}
