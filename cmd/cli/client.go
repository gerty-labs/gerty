package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/gerty-labs/gerty/internal/models"
)

// Client talks to the gerty-server REST API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a Client targeting the given base URL.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// apiGet performs a GET request and decodes the APIResponse envelope.
// The .Data field is re-marshalled into target if non-nil.
func (c *Client) apiGet(path string, target interface{}) error {
	resp, err := c.httpClient.Get(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	var envelope models.APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("failed to decode response (status %d): %w", resp.StatusCode, err)
	}

	if envelope.Status == "error" {
		return fmt.Errorf("server error: %s", envelope.Error)
	}

	if target != nil && envelope.Data != nil {
		raw, err := json.Marshal(envelope.Data)
		if err != nil {
			return fmt.Errorf("failed to re-marshal data: %w", err)
		}
		if err := json.Unmarshal(raw, target); err != nil {
			return fmt.Errorf("failed to decode data: %w", err)
		}
	}

	return nil
}

// GetClusterReport fetches the cluster-wide waste report.
func (c *Client) GetClusterReport() (*models.ClusterReport, error) {
	var report models.ClusterReport
	if err := c.apiGet("/api/v1/report", &report); err != nil {
		return nil, err
	}
	return &report, nil
}

// GetNamespaceReport fetches a namespace-scoped waste report.
func (c *Client) GetNamespaceReport(namespace string) (*models.NamespaceReport, error) {
	var report models.NamespaceReport
	if err := c.apiGet("/api/v1/report?namespace="+url.QueryEscape(namespace), &report); err != nil {
		return nil, err
	}
	return &report, nil
}

// GetWorkloads fetches all workload (OwnerWaste) entries.
func (c *Client) GetWorkloads() ([]models.OwnerWaste, error) {
	var workloads []models.OwnerWaste
	if err := c.apiGet("/api/v1/workloads", &workloads); err != nil {
		return nil, err
	}
	return workloads, nil
}

// GetWorkload fetches a single workload by namespace/kind/name.
func (c *Client) GetWorkload(ns, kind, name string) (*models.OwnerWaste, error) {
	path := fmt.Sprintf("/api/v1/workloads/%s/%s/%s",
		url.PathEscape(ns), url.PathEscape(kind), url.PathEscape(name))
	var workload models.OwnerWaste
	if err := c.apiGet(path, &workload); err != nil {
		return nil, err
	}
	return &workload, nil
}

// GetRecommendations fetches recommendations with optional filters.
func (c *Client) GetRecommendations(risk, namespace string) ([]models.Recommendation, error) {
	q := url.Values{}
	if risk != "" {
		q.Set("risk", risk)
	}
	if namespace != "" {
		q.Set("namespace", namespace)
	}
	path := "/api/v1/recommendations"
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var recs []models.Recommendation
	if err := c.apiGet(path, &recs); err != nil {
		return nil, err
	}
	return recs, nil
}
