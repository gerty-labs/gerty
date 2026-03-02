package slm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComplete_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/completion", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req CompletionRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "test prompt", req.Prompt)
		assert.Equal(t, 512, req.MaxTokens)

		resp := CompletionResponse{Content: `{"cpu_request":"250m"}`}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, 5*time.Second)
	result, err := client.Complete(context.Background(), CompletionRequest{
		Prompt:      "test prompt",
		MaxTokens:   512,
		Temperature: 0.1,
	})

	require.NoError(t, err)
	assert.Equal(t, `{"cpu_request":"250m"}`, result)
}

func TestComplete_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "model not loaded", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, 5*time.Second)
	_, err := client.Complete(context.Background(), CompletionRequest{
		Prompt:    "test",
		MaxTokens: 100,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestComplete_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer server.Close()

	client := NewClient(server.URL, 10*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.Complete(ctx, CompletionRequest{
		Prompt:    "test",
		MaxTokens: 100,
	})

	require.Error(t, err)
}

func TestHealthCheck_OK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/health", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, 5*time.Second)
	err := client.HealthCheck(context.Background())
	require.NoError(t, err)
}

func TestHealthCheck_Unhealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient(server.URL, 5*time.Second)
	err := client.HealthCheck(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "503")
}
