package models

import "time"

// ContainerMetrics holds raw metrics from a single scrape of one container.
type ContainerMetrics struct {
	PodName       string  `json:"podName"`
	PodNamespace  string  `json:"podNamespace"`
	ContainerName string  `json:"containerName"`
	Timestamp     time.Time `json:"timestamp"`

	// CPU
	CPUUsageNanoCores uint64 `json:"cpuUsageNanoCores"`
	CPURequestMillis  int64  `json:"cpuRequestMillis"`
	CPULimitMillis    int64  `json:"cpuLimitMillis"`

	// Memory
	MemoryUsageBytes      uint64 `json:"memoryUsageBytes"`
	MemoryWorkingSetBytes uint64 `json:"memoryWorkingSetBytes"`
	MemoryRequestBytes    int64  `json:"memoryRequestBytes"`
	MemoryLimitBytes      int64  `json:"memoryLimitBytes"`

	// Metadata
	RestartCount int32  `json:"restartCount"`
	QoSClass     string `json:"qosClass"`
}

// MetricAggregate holds P50/P95/P99/Max for a metric over a time window.
type MetricAggregate struct {
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
	P99 float64 `json:"p99"`
	Max float64 `json:"max"`
}

// AggregatedMetrics holds downsampled metrics for a container over a time bucket.
type AggregatedMetrics struct {
	PodName       string    `json:"podName"`
	PodNamespace  string    `json:"podNamespace"`
	ContainerName string    `json:"containerName"`
	BucketStart   time.Time `json:"bucketStart"`
	BucketEnd     time.Time `json:"bucketEnd"`
	SampleCount   int       `json:"sampleCount"`

	CPUNanoCores      MetricAggregate `json:"cpuNanoCores"`
	MemoryUsageBytes  MetricAggregate `json:"memoryUsageBytes"`
	MemoryWorkingSet  MetricAggregate `json:"memoryWorkingSet"`
}

// ContainerKey uniquely identifies a container within a cluster node.
func ContainerKey(namespace, pod, container string) string {
	return namespace + "/" + pod + "/" + container
}
