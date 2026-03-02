package slack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
	"github.com/gregorytcarroll/k8s-sage/internal/rules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Webhook tests ---

func TestWebhookClient_Send_Success(t *testing.T) {
	var received WebhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&received))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewWebhookClient(server.URL)
	err := client.Send(context.Background(), WebhookPayload{
		Text:    "test message",
		Channel: "#test",
	})

	require.NoError(t, err)
	assert.Equal(t, "test message", received.Text)
	assert.Equal(t, "#test", received.Channel)
}

func TestWebhookClient_Send_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid_token", http.StatusForbidden)
	}))
	defer server.Close()

	client := NewWebhookClient(server.URL)
	err := client.Send(context.Background(), WebhookPayload{Text: "test"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}

// --- Message building tests ---

func TestClassifySeverity(t *testing.T) {
	tests := []struct {
		name string
		rec  models.Recommendation
		want Severity
	}{
		{"high risk → critical", models.Recommendation{Risk: models.RiskHigh}, SeverityCritical},
		{"high confidence → optimisation", models.Recommendation{Risk: models.RiskLow, Confidence: 0.85}, SeverityOptimisation},
		{"low confidence → informational", models.Recommendation{Risk: models.RiskLow, Confidence: 0.5}, SeverityInformational},
		{"medium risk high conf → optimisation", models.Recommendation{Risk: models.RiskMedium, Confidence: 0.7}, SeverityOptimisation},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ClassifySeverity(tt.rec))
		})
	}
}

func TestRecKey(t *testing.T) {
	rec := models.Recommendation{
		Target:    models.OwnerReference{Namespace: "prod", Kind: "Deployment", Name: "api"},
		Container: "main",
		Resource:  "cpu",
	}

	key := RecKey(rec)
	assert.Equal(t, "prod/Deployment/api/main/cpu", key)
}

func TestBuildDigestMessage_Empty(t *testing.T) {
	msg := BuildDigestMessage(nil)
	assert.Contains(t, msg.Text, "No new recommendations")
	assert.Empty(t, msg.Blocks)
}

func TestBuildDigestMessage_GroupsByNamespace(t *testing.T) {
	recs := []models.Recommendation{
		{Target: models.OwnerReference{Namespace: "prod", Kind: "Deployment", Name: "api"}, Container: "main", Resource: "cpu", Confidence: 0.9, Risk: models.RiskLow},
		{Target: models.OwnerReference{Namespace: "prod", Kind: "Deployment", Name: "web"}, Container: "nginx", Resource: "memory", Confidence: 0.8, Risk: models.RiskLow},
		{Target: models.OwnerReference{Namespace: "staging", Kind: "Deployment", Name: "worker"}, Container: "app", Resource: "cpu", Confidence: 0.7, Risk: models.RiskMedium},
	}

	msg := BuildDigestMessage(recs)
	assert.Contains(t, msg.Text, "3 new recommendation(s)")
	assert.NotEmpty(t, msg.Blocks)

	// Should have header + sections for both namespaces
	headerCount := 0
	for _, b := range msg.Blocks {
		if b.Type == "header" {
			headerCount++
		}
	}
	assert.Equal(t, 1, headerCount)
}

func TestBuildSingleMessage(t *testing.T) {
	rec := models.Recommendation{
		Target:    models.OwnerReference{Namespace: "prod", Kind: "Deployment", Name: "api"},
		Container: "main",
		Resource:  "cpu",
		Risk:      models.RiskLow,
		Confidence: 0.85,
	}

	msg := BuildSingleMessage(rec)
	assert.Contains(t, msg.Text, "cpu")
	assert.NotEmpty(t, msg.Blocks)
}

// --- Dedup tests ---

func TestNotifier_Dedup(t *testing.T) {
	n := &Notifier{
		seen: make(map[string]time.Time),
		now:  func() time.Time { return time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC) },
	}

	now := n.now()
	key := "prod/Deployment/api/main/cpu"

	// First time: not duplicate
	assert.False(t, n.isDuplicate(key, now))

	// Mark seen
	n.markSeen(key, now)

	// Immediately: duplicate
	assert.True(t, n.isDuplicate(key, now))

	// After 6 days: still duplicate
	sixDays := now.Add(6 * 24 * time.Hour)
	assert.True(t, n.isDuplicate(key, sixDays))

	// After 8 days: no longer duplicate
	eightDays := now.Add(8 * 24 * time.Hour)
	assert.False(t, n.isDuplicate(key, eightDays))
}

// --- Severity filter tests ---

func TestMeetsMinSeverity(t *testing.T) {
	tests := []struct {
		severity Severity
		min      Severity
		want     bool
	}{
		{SeverityCritical, SeverityCritical, true},
		{SeverityCritical, SeverityOptimisation, true},
		{SeverityCritical, SeverityInformational, true},
		{SeverityOptimisation, SeverityCritical, false},
		{SeverityOptimisation, SeverityOptimisation, true},
		{SeverityOptimisation, SeverityInformational, true},
		{SeverityInformational, SeverityCritical, false},
		{SeverityInformational, SeverityOptimisation, false},
		{SeverityInformational, SeverityInformational, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.severity)+"_min_"+string(tt.min), func(t *testing.T) {
			assert.Equal(t, tt.want, meetsMinSeverity(tt.severity, tt.min))
		})
	}
}

// --- Integration-style notifier test ---

type mockClusterReporter struct {
	report models.ClusterReport
}

func (m *mockClusterReporter) ClusterReport() models.ClusterReport {
	return m.report
}

func TestNotifier_SendDigest(t *testing.T) {
	var received WebhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&received))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Build a report with a workload that will generate a recommendation
	source := &mockClusterReporter{
		report: models.ClusterReport{
			ReportTime: time.Now(),
			NodeCount:  1,
			PodCount:   1,
			Namespaces: map[string]*models.NamespaceReport{
				"default": {
					Namespace: "default",
					Owners: []models.OwnerWaste{
						{
							Owner: models.OwnerReference{Kind: "Deployment", Name: "api", Namespace: "default"},
							PodCount: 1,
							Containers: []models.ContainerWaste{
								{
									ContainerName:      "main",
									CPURequestMillis:   1000,
									CPUUsage:           models.MetricAggregate{P50: 100, P95: 200, P99: 250, Max: 300},
									MemoryRequestBytes: 1 << 30,
									MemoryUsage:        models.MetricAggregate{P50: 100e6, P95: 200e6, P99: 250e6, Max: 300e6},
									DataWindow:         2 * time.Hour,
								},
							},
						},
					},
				},
			},
		},
	}

	notifier := NewNotifier(Config{
		WebhookURL:     server.URL,
		Channel:        "#test",
		DigestInterval: 1 * time.Minute,
		MinSeverity:    SeverityInformational,
	}, source, rules.NewEngine())

	err := notifier.sendDigest(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "#test", received.Channel)
	assert.NotEmpty(t, received.Text)
}
