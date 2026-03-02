package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPusher_PushOnce_ValidReport(t *testing.T) {
	var received models.NodeReport
	var called atomic.Bool

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/ingest", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		err := json.NewDecoder(r.Body).Decode(&received)
		require.NoError(t, err)
		called.Store(true)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok"})
	}))
	defer ts.Close()

	store := NewStore()
	reporter := NewReporter("test-node", store)
	pusher := NewPusher(ts.URL, reporter, time.Second)

	err := pusher.pushOnce(context.Background())
	require.NoError(t, err)
	assert.True(t, called.Load(), "server handler should have been called")
	assert.Equal(t, "test-node", received.NodeName)
}

func TestPusher_PushOnce_ServerDown(t *testing.T) {
	// Use a URL that will fail to connect.
	store := NewStore()
	reporter := NewReporter("test-node", store)
	pusher := NewPusher("http://127.0.0.1:1", reporter, time.Second)

	err := pusher.pushOnce(context.Background())
	assert.Error(t, err, "push should fail when server is unreachable")
}

func TestPusher_PushOnce_ServerErrorStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	store := NewStore()
	reporter := NewReporter("test-node", store)
	pusher := NewPusher(ts.URL, reporter, time.Second)

	err := pusher.pushOnce(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestPusher_Run_CancelsGracefully(t *testing.T) {
	var callCount atomic.Int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok"})
	}))
	defer ts.Close()

	store := NewStore()
	reporter := NewReporter("test-node", store)
	pusher := NewPusher(ts.URL, reporter, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	pusher.Run(ctx)

	// Should have pushed at least once before context was cancelled.
	assert.GreaterOrEqual(t, callCount.Load(), int32(1))
}
