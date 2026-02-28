package models

import "time"

// PodWaste represents the waste analysis for a single pod.
type PodWaste struct {
	PodName             string           `json:"podName"`
	PodNamespace        string           `json:"podNamespace"`
	QoSClass            string           `json:"qosClass"`
	OwnerRef            OwnerReference   `json:"ownerRef,omitempty"`
	Containers          []ContainerWaste `json:"containers"`
	TotalCPUWasteMillis float64          `json:"totalCpuWasteMillis"`
	TotalMemWasteBytes  float64          `json:"totalMemWasteBytes"`
}

// ContainerWaste represents waste for a single container.
type ContainerWaste struct {
	ContainerName string `json:"containerName"`

	// CPU waste
	CPURequestMillis int64           `json:"cpuRequestMillis"`
	CPUUsage         MetricAggregate `json:"cpuUsage"`
	CPUWasteMillis   float64         `json:"cpuWasteMillis"`
	CPUWastePercent  float64         `json:"cpuWastePercent"`

	// Memory waste
	MemoryRequestBytes int64           `json:"memoryRequestBytes"`
	MemoryUsage        MetricAggregate `json:"memoryUsage"`
	MemWasteBytes      float64         `json:"memWasteBytes"`
	MemWastePercent    float64         `json:"memWastePercent"`

	RestartCount int32         `json:"restartCount"`
	DataWindow   time.Duration `json:"dataWindow"`
}

// NodeReport is the full waste report pushed from a single node's agent.
type NodeReport struct {
	NodeName            string     `json:"nodeName"`
	ReportTime          time.Time  `json:"reportTime"`
	Pods                []PodWaste `json:"pods"`
	TotalCPUWasteMillis float64    `json:"totalCpuWasteMillis"`
	TotalMemWasteBytes  float64    `json:"totalMemWasteBytes"`
}

// ClusterReport is the cluster-wide waste report returned by the server API.
type ClusterReport struct {
	ReportTime          time.Time              `json:"reportTime"`
	NodeCount           int                    `json:"nodeCount"`
	PodCount            int                    `json:"podCount"`
	Namespaces          map[string]*NamespaceReport `json:"namespaces"`
	TotalCPUWasteMillis float64                `json:"totalCpuWasteMillis"`
	TotalMemWasteBytes  float64                `json:"totalMemWasteBytes"`
	Recommendations     []Recommendation       `json:"recommendations,omitempty"`
}

// NamespaceReport is a waste summary for a single namespace.
type NamespaceReport struct {
	Namespace           string       `json:"namespace"`
	Pods                []PodWaste   `json:"pods"`
	Owners              []OwnerWaste `json:"owners,omitempty"`
	TotalCPUWasteMillis float64      `json:"totalCpuWasteMillis"`
	TotalMemWasteBytes  float64      `json:"totalMemWasteBytes"`
}

// OwnerWaste aggregates waste across all pods belonging to the same owner
// (e.g. a Deployment with multiple replicas).
type OwnerWaste struct {
	Owner               OwnerReference `json:"owner"`
	PodCount            int            `json:"podCount"`
	Containers          []ContainerWaste `json:"containers"`
	TotalCPUWasteMillis float64        `json:"totalCpuWasteMillis"`
	TotalMemWasteBytes  float64        `json:"totalMemWasteBytes"`
}
