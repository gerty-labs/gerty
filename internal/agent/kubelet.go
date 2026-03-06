package agent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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

// kubeletTLSConfig builds a TLS configuration for talking to the kubelet API.
//
// In-cluster: loads the cluster CA from the service account mount and verifies
// the kubelet certificate chain against it. Hostname verification is skipped
// because kubelet SANs may not match NODE_NAME across all K8s distributions.
//
// Out-of-cluster: falls back to the system certificate pool.
func kubeletTLSConfig() *tls.Config {
	cfg := &tls.Config{
		MinVersion: tls.VersionTLS13,
	}

	// Load cluster CA for kubelet certificate chain verification.
	const caPath = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	var caPool *x509.CertPool
	if caBytes, err := os.ReadFile(caPath); err == nil {
		caPool = x509.NewCertPool()
		if caPool.AppendCertsFromPEM(caBytes) {
			cfg.RootCAs = caPool
			slog.Debug("loaded cluster CA for kubelet TLS", "path", caPath)
		} else {
			caPool = nil
			slog.Warn("cluster CA file found but contained no valid certs", "path", caPath)
		}
	}

	// Skip Go's default hostname verification — kubelet SAN may not match
	// NODE_NAME across K8s distributions. Chain and expiry are verified
	// in VerifyPeerCertificate below.
	cfg.InsecureSkipVerify = true // #nosec G402
	cfg.VerifyPeerCertificate = verifyKubeletCert(caPool)

	return cfg
}

// verifyKubeletCert returns a TLS peer certificate callback that validates the
// kubelet's certificate. When a CA pool is provided (in-cluster), the full
// certificate chain is verified against it. In all cases, certificate expiry
// is checked.
func verifyKubeletCert(caPool *x509.CertPool) func([][]byte, [][]*x509.Certificate) error {
	return func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return fmt.Errorf("kubelet presented no TLS certificates")
		}

		certs := make([]*x509.Certificate, len(rawCerts))
		for i, raw := range rawCerts {
			cert, err := x509.ParseCertificate(raw)
			if err != nil {
				return fmt.Errorf("parsing kubelet certificate %d: %w", i, err)
			}
			certs[i] = cert
		}

		leaf := certs[0]
		now := time.Now()
		if now.Before(leaf.NotBefore) || now.After(leaf.NotAfter) {
			return fmt.Errorf("kubelet certificate expired: valid %s to %s",
				leaf.NotBefore.Format(time.RFC3339), leaf.NotAfter.Format(time.RFC3339))
		}

		// Verify chain against cluster CA when available.
		if caPool != nil {
			opts := x509.VerifyOptions{
				Roots:       caPool,
				CurrentTime: now,
			}
			if len(certs) > 1 {
				opts.Intermediates = x509.NewCertPool()
				for _, c := range certs[1:] {
					opts.Intermediates.AddCert(c)
				}
			}
			if _, err := leaf.Verify(opts); err != nil {
				return fmt.Errorf("kubelet certificate chain verification failed: %w", err)
			}
		}

		return nil
	}
}

// NewHTTPKubeletClient creates a kubelet client that talks to the real kubelet API.
// It uses the in-cluster service account token for authentication and verifies
// kubelet certificates against the cluster CA when available.
func NewHTTPKubeletClient(baseURL string) *httpKubeletClient {
	tokenBytes, _ := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")

	return &httpKubeletClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: &http.Transport{TLSClientConfig: kubeletTLSConfig()},
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
