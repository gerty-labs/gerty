package agent

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// SummaryResponse mirrors the kubelet /stats/summary JSON structure.
// Only the fields we need are included.
type SummaryResponse struct {
	Node NodeStats  `json:"node"`
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
	Name      string    `json:"name"`
	CPU       *CPUStats `json:"cpu,omitempty"`
	Memory    *MemStats `json:"memory,omitempty"`
	StartTime time.Time `json:"startTime"`
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

// PodListResponse mirrors the kubelet /pods JSON structure (a PodList).
type PodListResponse struct {
	Items []PodItem `json:"items"`
}

// PodItem is a minimal representation of a Pod from the kubelet /pods endpoint.
type PodItem struct {
	Metadata PodItemMeta `json:"metadata"`
	Spec     PodSpec     `json:"spec"`
	Status   PodStatus   `json:"status"`
}

type PodItemMeta struct {
	Name            string              `json:"name"`
	Namespace       string              `json:"namespace"`
	OwnerReferences []PodOwnerReference `json:"ownerReferences,omitempty"`
}

// PodOwnerReference is a minimal representation of a Kubernetes OwnerReference.
type PodOwnerReference struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Controller *bool  `json:"controller,omitempty"`
}

type PodSpec struct {
	Containers []PodSpecContainer `json:"containers"`
}

type PodSpecContainer struct {
	Name      string                `json:"name"`
	Resources ContainerResourceSpec `json:"resources"`
}

type ContainerResourceSpec struct {
	Requests ResourceValues `json:"requests"`
	Limits   ResourceValues `json:"limits"`
}

// ResourceValues holds CPU and memory resource strings from the pod spec.
type ResourceValues struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

type PodStatus struct {
	QOSClass         string            `json:"qosClass"`
	ContainerStatuses []ContainerStatus `json:"containerStatuses"`
}

type ContainerStatus struct {
	Name         string `json:"name"`
	RestartCount int32  `json:"restartCount"`
}

// KubeletClient abstracts access to the kubelet Summary API for testability.
type KubeletClient interface {
	GetSummary(ctx context.Context) (*SummaryResponse, error)
	GetPods(ctx context.Context) (*PodListResponse, error)
}

// httpKubeletClient implements KubeletClient by calling the kubelet REST API.
type httpKubeletClient struct {
	baseURL    string
	httpClient *http.Client
	token      string
}

// NewHTTPKubeletClient creates a kubelet client that talks to the real kubelet API.
// It uses the in-cluster service account token for authentication and skips TLS
// verification (kubelet uses self-signed certs).
func NewHTTPKubeletClient(baseURL string) *httpKubeletClient {
	tokenBytes, _ := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")

	return &httpKubeletClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true, // #nosec G402 // nosemgrep — kubelet uses self-signed certs
				},
			},
		},
		token: string(tokenBytes),
	}
}

func (c *httpKubeletClient) GetPods(ctx context.Context) (*PodListResponse, error) {
	url := fmt.Sprintf("%s/pods", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating kubelet pods request: %w", err)
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling kubelet pods API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("kubelet pods returned status %d: %s", resp.StatusCode, string(body))
	}

	var podList PodListResponse
	if err := json.NewDecoder(resp.Body).Decode(&podList); err != nil {
		return nil, fmt.Errorf("decoding kubelet pods response: %w", err)
	}

	return &podList, nil
}

func (c *httpKubeletClient) GetSummary(ctx context.Context) (*SummaryResponse, error) {
	url := fmt.Sprintf("%s/stats/summary", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating kubelet request: %w", err)
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling kubelet summary API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("kubelet returned status %d: %s", resp.StatusCode, string(body))
	}

	var summary SummaryResponse
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		return nil, fmt.Errorf("decoding kubelet summary response: %w", err)
	}

	return &summary, nil
}
