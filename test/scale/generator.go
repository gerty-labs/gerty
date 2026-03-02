package scale

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
)

// ScaleConfig defines the parameters for a scale test scenario.
type ScaleConfig struct {
	Nodes       int
	PodsPerNode int
	Namespaces  int
	Owners      int
}

// TotalPods returns the total pod count for this config.
func (c ScaleConfig) TotalPods() int {
	return c.Nodes * c.PodsPerNode
}

// ownerDef holds the definition of a generated workload owner.
type ownerDef struct {
	kind      string
	name      string
	namespace string
	replicas  int
	pattern   models.WorkloadPattern
}

// GenerateNodeReports creates synthetic NodeReports for the given scale config.
// Uses a deterministic seed for reproducibility.
func GenerateNodeReports(cfg ScaleConfig, seed int64) []models.NodeReport {
	rng := rand.New(rand.NewSource(seed))
	now := time.Now()

	// Generate namespaces with weighted distribution.
	namespaces := make([]string, cfg.Namespaces)
	for i := range namespaces {
		namespaces[i] = fmt.Sprintf("ns-%03d", i)
	}

	// Generate owners distributed across namespaces.
	owners := generateOwners(rng, cfg.Owners, namespaces)

	// Distribute pods across nodes.
	reports := make([]models.NodeReport, cfg.Nodes)
	podIdx := 0
	totalPods := cfg.TotalPods()

	for n := 0; n < cfg.Nodes; n++ {
		nodeName := fmt.Sprintf("node-%04d", n)
		pods := make([]models.PodWaste, 0, cfg.PodsPerNode)

		for p := 0; p < cfg.PodsPerNode && podIdx < totalPods; p++ {
			// Pick an owner (round-robin with some variation).
			owner := owners[podIdx%len(owners)]
			pod := generatePod(rng, owner, podIdx, now)
			pods = append(pods, pod)
			podIdx++
		}

		var totalCPU, totalMem float64
		for i := range pods {
			totalCPU += pods[i].TotalCPUWasteMillis
			totalMem += pods[i].TotalMemWasteBytes
		}

		reports[n] = models.NodeReport{
			NodeName:            nodeName,
			ReportTime:          now,
			Pods:                pods,
			TotalCPUWasteMillis: totalCPU,
			TotalMemWasteBytes:  totalMem,
		}
	}

	return reports
}

// generateOwners creates a set of workload owners with realistic distributions.
func generateOwners(rng *rand.Rand, count int, namespaces []string) []ownerDef {
	owners := make([]ownerDef, count)

	// Workload mix: 50% steady, 25% burstable, 15% batch, 5% idle, 5% anomalous.
	patterns := []struct {
		pattern models.WorkloadPattern
		weight  float64
	}{
		{models.PatternSteady, 0.50},
		{models.PatternBurstable, 0.25},
		{models.PatternBatch, 0.15},
		{models.PatternIdle, 0.05},
		{models.PatternAnomalous, 0.05},
	}

	for i := 0; i < count; i++ {
		ns := namespaces[i%len(namespaces)]

		// Pick pattern based on weights.
		r := rng.Float64()
		var cumulative float64
		pattern := models.PatternSteady
		for _, pw := range patterns {
			cumulative += pw.weight
			if r < cumulative {
				pattern = pw.pattern
				break
			}
		}

		// Deployment (80%) or StatefulSet (20%).
		kind := "Deployment"
		replicas := 3 + rng.Intn(8) // 3-10 replicas.
		if rng.Float64() < 0.2 {
			kind = "StatefulSet"
			replicas = 1 + rng.Intn(3) // 1-3 replicas.
		}

		owners[i] = ownerDef{
			kind:      kind,
			name:      fmt.Sprintf("%s-%s-%03d", patternPrefix(pattern), kind[:3], i),
			namespace: ns,
			replicas:  replicas,
			pattern:   pattern,
		}
	}

	return owners
}

func patternPrefix(p models.WorkloadPattern) string {
	switch p {
	case models.PatternSteady:
		return "sdy"
	case models.PatternBurstable:
		return "bst"
	case models.PatternBatch:
		return "bat"
	case models.PatternIdle:
		return "idl"
	case models.PatternAnomalous:
		return "ano"
	default:
		return "unk"
	}
}

// generatePod creates a single PodWaste with metrics matching the owner's pattern.
func generatePod(rng *rand.Rand, owner ownerDef, podIdx int, now time.Time) models.PodWaste {
	podName := fmt.Sprintf("%s-%05d", owner.name, podIdx)

	container := generateContainer(rng, owner.pattern)
	cpuWaste := container.CPUWasteMillis
	memWaste := container.MemWasteBytes

	return models.PodWaste{
		PodName:      podName,
		PodNamespace: owner.namespace,
		QoSClass:     "Burstable",
		OwnerRef: models.OwnerReference{
			Kind:      owner.kind,
			Name:      owner.name,
			Namespace: owner.namespace,
		},
		Containers:          []models.ContainerWaste{container},
		TotalCPUWasteMillis: cpuWaste,
		TotalMemWasteBytes:  memWaste,
	}
}

// generateContainer creates metrics for a container matching the given pattern.
func generateContainer(rng *rand.Rand, pattern models.WorkloadPattern) models.ContainerWaste {
	var cpuReq, memReq int64
	var cpuUsage, memUsage models.MetricAggregate
	dataWindow := 7 * 24 * time.Hour // 7 days of data.

	switch pattern {
	case models.PatternSteady:
		// Steady: low CV, over-provisioned.
		cpuReq = 1000 // 1000m requested.
		cpuP50 := 200.0 + rng.Float64()*100
		cpuUsage = models.MetricAggregate{
			P50: cpuP50,
			P95: cpuP50 * 1.1,
			P99: cpuP50 * 1.15,
			Max: cpuP50 * 1.2,
		}
		memReq = 512 * 1024 * 1024 // 512Mi.
		memP50 := 100.0 * 1024 * 1024
		memUsage = models.MetricAggregate{
			P50: memP50,
			P95: memP50 * 1.05,
			P99: memP50 * 1.1,
			Max: memP50 * 1.15,
		}

	case models.PatternBurstable:
		// Burstable: periodic spikes, moderate CV.
		cpuReq = 2000
		cpuP50 := 300.0 + rng.Float64()*200
		cpuUsage = models.MetricAggregate{
			P50: cpuP50,
			P95: cpuP50 * 2.5,
			P99: cpuP50 * 3.5,
			Max: cpuP50 * 4.0,
		}
		memReq = 1024 * 1024 * 1024
		memP50 := 200.0 * 1024 * 1024
		memUsage = models.MetricAggregate{
			P50: memP50,
			P95: memP50 * 1.5,
			P99: memP50 * 2.0,
			Max: memP50 * 2.5,
		}

	case models.PatternBatch:
		// Batch: extreme spikes.
		cpuReq = 4000
		cpuP50 := 100.0 + rng.Float64()*100
		cpuUsage = models.MetricAggregate{
			P50: cpuP50,
			P95: cpuP50 * 8,
			P99: cpuP50 * 15,
			Max: cpuP50 * 20,
		}
		memReq = 2048 * 1024 * 1024
		memP50 := 300.0 * 1024 * 1024
		memUsage = models.MetricAggregate{
			P50: memP50,
			P95: memP50 * 3,
			P99: memP50 * 5,
			Max: memP50 * 8,
		}

	case models.PatternIdle:
		// Idle: < 5% utilisation.
		cpuReq = 500
		cpuP50 := 10.0 + rng.Float64()*10
		cpuUsage = models.MetricAggregate{
			P50: cpuP50,
			P95: cpuP50 * 1.2,
			P99: cpuP50 * 1.5,
			Max: cpuP50 * 2.0,
		}
		memReq = 256 * 1024 * 1024
		memP50 := 10.0 * 1024 * 1024
		memUsage = models.MetricAggregate{
			P50: memP50,
			P95: memP50 * 1.1,
			P99: memP50 * 1.2,
			Max: memP50 * 1.3,
		}
		dataWindow = 72 * time.Hour // >48h for idle classification.

	case models.PatternAnomalous:
		// Anomalous: monotonic memory growth.
		cpuReq = 1000
		cpuP50 := 400.0 + rng.Float64()*200
		cpuUsage = models.MetricAggregate{
			P50: cpuP50,
			P95: cpuP50 * 1.2,
			P99: cpuP50 * 1.3,
			Max: cpuP50 * 1.5,
		}
		memReq = 1024 * 1024 * 1024
		memP50 := 200.0 * 1024 * 1024
		memUsage = models.MetricAggregate{
			P50: memP50,
			P95: memP50 * 2.5,
			P99: memP50 * 3.0,  // Growth ratio P99/P50 >= 2.0.
			Max: memP50 * 3.2,  // Max proximity: P99/Max >= 0.85.
		}
	}

	cpuWaste := math.Max(0, float64(cpuReq)-cpuUsage.P95)
	cpuWastePct := 0.0
	if cpuReq > 0 {
		cpuWastePct = (cpuWaste / float64(cpuReq)) * 100
	}
	memWaste := math.Max(0, float64(memReq)-memUsage.P95)
	memWastePct := 0.0
	if memReq > 0 {
		memWastePct = (memWaste / float64(memReq)) * 100
	}

	return models.ContainerWaste{
		ContainerName:    "main",
		CPURequestMillis: cpuReq,
		CPUUsage:         cpuUsage,
		CPUWasteMillis:   cpuWaste,
		CPUWastePercent:  cpuWastePct,
		MemoryRequestBytes: memReq,
		MemoryUsage:        memUsage,
		MemWasteBytes:      memWaste,
		MemWastePercent:    memWastePct,
		RestartCount:       0,
		DataWindow:         dataWindow,
	}
}
