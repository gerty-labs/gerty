package agent

import (
	"context"
	"time"
)

// SummaryResponse mirrors the kubelet /stats/summary JSON structure.
// Only the fields we need are included.
type SummaryResponse struct {
	Node NodeStats `json:"node"`
	Pods []PodStats `json:"pods"`
}

type NodeStats struct {
	NodeName string `json:"nodeName"`
}

type PodStats struct {
	PodRef     PodReference     `json:"podRef"`
	Containers []ContainerStats `json:"containers"`
}

type PodReference struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type ContainerStats struct {
	Name      string     `json:"name"`
	CPU       *CPUStats  `json:"cpu,omitempty"`
	Memory    *MemStats  `json:"memory,omitempty"`
	StartTime time.Time  `json:"startTime"`
}

type CPUStats struct {
	Time                 time.Time `json:"time"`
	UsageNanoCores       *uint64   `json:"usageNanoCores,omitempty"`
	UsageCoreNanoSeconds *uint64   `json:"usageCoreNanoSeconds,omitempty"`
}

type MemStats struct {
	Time            time.Time `json:"time"`
	UsageBytes      *uint64   `json:"usageBytes,omitempty"`
	WorkingSetBytes *uint64   `json:"workingSetBytes,omitempty"`
	RSSBytes        *uint64   `json:"rssBytes,omitempty"`
	AvailableBytes  *uint64   `json:"availableBytes,omitempty"`
}

// httpKubeletClient is the real kubelet HTTP client.
type httpKubeletClient struct {
	baseURL string
}

func (c *httpKubeletClient) GetSummary(ctx context.Context) (*SummaryResponse, error) {
	// Placeholder — will use crypto/tls with service account token
	return nil, nil
}
