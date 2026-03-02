package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// WebhookClient sends messages to a Slack incoming webhook URL.
type WebhookClient struct {
	url    string
	client *http.Client
}

// NewWebhookClient creates a client that POSTs to the given Slack webhook URL.
func NewWebhookClient(url string) *WebhookClient {
	return &WebhookClient{
		url: url,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// WebhookPayload is the JSON body sent to Slack's incoming webhook.
type WebhookPayload struct {
	Channel string      `json:"channel,omitempty"`
	Text    string      `json:"text,omitempty"`
	Blocks  []BlockItem `json:"blocks,omitempty"`
}

// Send posts a payload to the Slack webhook. Returns an error if the
// request fails or Slack returns a non-200 status.
func (w *WebhookClient) Send(ctx context.Context, payload WebhookPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("slack webhook returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
