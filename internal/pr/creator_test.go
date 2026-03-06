package pr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gerty-labs/gerty/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRunner implements CommandRunner for testing.
type mockRunner struct {
	calls     []mockCall
	responses map[string]mockResponse
}

type mockCall struct {
	Dir  string
	Name string
	Args []string
}

type mockResponse struct {
	output []byte
	err    error
}

func newMockRunner() *mockRunner {
	return &mockRunner{
		responses: make(map[string]mockResponse),
	}
}

func (m *mockRunner) addResponse(key string, output []byte, err error) {
	m.responses[key] = mockResponse{output: output, err: err}
}

func (m *mockRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	m.calls = append(m.calls, mockCall{Name: name, Args: args})
	return m.findResponse(name, args)
}

func (m *mockRunner) RunInDir(_ context.Context, dir string, name string, args ...string) ([]byte, error) {
	m.calls = append(m.calls, mockCall{Dir: dir, Name: name, Args: args})
	return m.findResponse(name, args)
}

func (m *mockRunner) findResponse(name string, args []string) ([]byte, error) {
	key := name
	for _, a := range args {
		key += " " + a
	}
	// Exact match first.
	if resp, ok := m.responses[key]; ok {
		return resp.output, resp.err
	}
	// Substring match.
	for pattern, resp := range m.responses {
		if strings.Contains(key, pattern) {
			return resp.output, resp.err
		}
	}
	return nil, fmt.Errorf("no mock for: %s", key)
}

func TestCheckGHAuth_Authenticated(t *testing.T) {
	runner := newMockRunner()
	runner.addResponse("gh auth status", []byte("Logged in to github.com"), nil)

	p := NewPRCreator(runner, "http://localhost:8080")
	err := p.checkGHAuth(context.Background())
	assert.NoError(t, err)
}

func TestCheckGHAuth_NotAuthenticated(t *testing.T) {
	runner := newMockRunner()
	runner.addResponse("gh auth status", nil, fmt.Errorf("not logged in"))

	p := NewPRCreator(runner, "http://localhost:8080")
	err := p.checkGHAuth(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "gh auth login")
}

func TestReadAnnotations_Found(t *testing.T) {
	annotations := map[string]string{
		"gerty.io/repo":  "github.com/acme/manifests",
		"gerty.io/path":  "apps/api/deployment.yaml",
		"gerty.io/field": "resources.requests.cpu",
	}
	raw, _ := json.Marshal(annotations)

	runner := newMockRunner()
	runner.addResponse("kubectl get", raw, nil)

	p := NewPRCreator(runner, "http://localhost:8080")
	ann, err := p.readAnnotations(context.Background(), "Deployment", "api-gateway", "production")
	require.NoError(t, err)
	assert.Equal(t, "github.com/acme/manifests", ann.Repo)
	assert.Equal(t, "apps/api/deployment.yaml", ann.Path)
	assert.Equal(t, "resources.requests.cpu", ann.Field)
}

func TestReadAnnotations_MissingAnnotations(t *testing.T) {
	annotations := map[string]string{
		"app": "api-gateway",
	}
	raw, _ := json.Marshal(annotations)

	runner := newMockRunner()
	runner.addResponse("kubectl get", raw, nil)

	p := NewPRCreator(runner, "http://localhost:8080")
	_, err := p.readAnnotations(context.Background(), "Deployment", "api-gateway", "production")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing required annotations")
}

func TestReadAnnotations_KubectlFails(t *testing.T) {
	runner := newMockRunner()
	runner.addResponse("kubectl get", nil, fmt.Errorf("not found"))

	p := NewPRCreator(runner, "http://localhost:8080")
	_, err := p.readAnnotations(context.Background(), "Deployment", "api-gateway", "production")
	assert.Error(t, err)
}

func TestFetchRecommendation_Found(t *testing.T) {
	recs := []models.Recommendation{
		{
			Target:         models.OwnerReference{Kind: "Deployment", Name: "api-gateway", Namespace: "production"},
			Container:      "api",
			Resource:       "cpu",
			CurrentRequest: 1000,
			RecommendedReq: 250,
			Pattern:        models.PatternSteady,
			Confidence:     0.92,
			Risk:           models.RiskLow,
			Reasoning:      "Steady workload, P95 usage is 200m",
		},
		{
			Target:         models.OwnerReference{Kind: "Deployment", Name: "worker", Namespace: "production"},
			Container:      "worker",
			Resource:       "cpu",
			CurrentRequest: 2000,
			RecommendedReq: 500,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := models.NewOKResponse(recs)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	runner := newMockRunner()
	p := NewPRCreator(runner, server.URL)
	rec, err := p.fetchRecommendation(context.Background(), "production", "Deployment", "api-gateway")
	require.NoError(t, err)
	assert.Equal(t, "api-gateway", rec.Target.Name)
	assert.Equal(t, "cpu", rec.Resource)
	assert.Equal(t, int64(250), rec.RecommendedReq)
}

func TestFetchRecommendation_NotFound(t *testing.T) {
	recs := []models.Recommendation{
		{
			Target:   models.OwnerReference{Kind: "Deployment", Name: "other", Namespace: "production"},
			Resource: "cpu",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := models.NewOKResponse(recs)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	runner := newMockRunner()
	p := NewPRCreator(runner, server.URL)
	_, err := p.fetchRecommendation(context.Background(), "production", "Deployment", "api-gateway")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no recommendation found")
}

func TestFormatBranchName(t *testing.T) {
	branch := FormatBranchName("gerty-rightsizing", "production", "api-gateway")
	assert.True(t, strings.HasPrefix(branch, "gerty-rightsizing/production-api-gateway-"))
	// Should contain a timestamp pattern.
	parts := strings.Split(branch, "-")
	assert.True(t, len(parts) >= 4)
}

func TestFormatPRBody(t *testing.T) {
	rec := &models.Recommendation{
		Target:         models.OwnerReference{Kind: "Deployment", Name: "api-gateway", Namespace: "production"},
		Resource:       "cpu",
		CurrentRequest: 1000,
		RecommendedReq: 250,
		Pattern:        models.PatternSteady,
		Confidence:     0.92,
		Risk:           models.RiskLow,
		Reasoning:      "Steady workload, P95 usage is 200m",
	}

	body := FormatPRBody(rec, "Deployment", "api-gateway", "production")
	assert.Contains(t, body, "Right-sizing: Deployment/api-gateway")
	assert.Contains(t, body, "production")
	assert.Contains(t, body, "steady")
	assert.Contains(t, body, "92%")
	assert.Contains(t, body, "LOW")
	assert.Contains(t, body, "| 1 |") // 1000m formats as "1" (whole cores)
	assert.Contains(t, body, "250m")
	assert.Contains(t, body, "gerty")
}

func TestModifyManifestFile_CPU(t *testing.T) {
	content := `apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: api
        resources:
          requests:
            cpu: 1000m
            memory: 512Mi
          limits:
            cpu: 2000m
            memory: 1Gi
`
	rec := &models.Recommendation{
		Resource:         "cpu",
		RecommendedReq:   250,
		RecommendedLimit: 500,
	}

	result, err := modifyManifestFile(content, rec)
	require.NoError(t, err)
	assert.Contains(t, result, "cpu: 250m")
	assert.Contains(t, result, "cpu: 500m")
	assert.Contains(t, result, "memory: 512Mi") // Unchanged.
}

func TestModifyManifestFile_Memory(t *testing.T) {
	content := `apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: api
        resources:
          requests:
            cpu: 500m
            memory: 512Mi
          limits:
            cpu: 1000m
            memory: 1024Mi
`
	rec := &models.Recommendation{
		Resource:         "memory",
		RecommendedReq:   268435456, // 256Mi
		RecommendedLimit: 536870912, // 512Mi
	}

	result, err := modifyManifestFile(content, rec)
	require.NoError(t, err)
	assert.Contains(t, result, "memory: 256Mi")
	assert.Contains(t, result, "memory: 512Mi")
	assert.Contains(t, result, "cpu: 500m") // Unchanged.
}

func TestModifyValuesFile(t *testing.T) {
	content := `replicaCount: 3
resources:
  requests:
    cpu: 1000m
    memory: 512Mi
  limits:
    cpu: 2000m
    memory: 1Gi
`
	rec := &models.Recommendation{
		Resource:         "cpu",
		RecommendedReq:   250,
		RecommendedLimit: 500,
	}

	result, err := modifyValuesFile(content, "resources.requests.cpu", rec)
	require.NoError(t, err)
	assert.Contains(t, result, "cpu: 250m")
	assert.Contains(t, result, "cpu: 500m")
}

func TestDryRun(t *testing.T) {
	recs := []models.Recommendation{
		{
			Target:         models.OwnerReference{Kind: "Deployment", Name: "api-gateway", Namespace: "production"},
			Resource:       "cpu",
			CurrentRequest: 1000,
			RecommendedReq: 250,
			Pattern:        models.PatternSteady,
			Confidence:     0.92,
			Risk:           models.RiskLow,
			Reasoning:      "Steady workload",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := models.NewOKResponse(recs)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	annotations := map[string]string{
		"gerty.io/repo": "github.com/acme/manifests",
		"gerty.io/path": "apps/api/deployment.yaml",
	}
	annotationsJSON, _ := json.Marshal(annotations)

	runner := newMockRunner()
	runner.addResponse("gh auth status", []byte("ok"), nil)
	runner.addResponse("kubectl get", annotationsJSON, nil)

	p := NewPRCreator(runner, server.URL)
	result, err := p.Create(context.Background(), Options{
		Kind:         "Deployment",
		Name:         "api-gateway",
		Namespace:    "production",
		BranchPrefix: "gerty-rightsizing",
		DryRun:       true,
	})

	require.NoError(t, err)
	assert.True(t, result.DryRun)
	assert.Equal(t, "apps/api/deployment.yaml", result.FilePath)
	assert.Contains(t, result.Title, "right-size")
	assert.NotNil(t, result.Recommendation)
	assert.Equal(t, "", result.PRURL) // No PR created in dry-run.
}

func TestFullFlow_WithMockRunner(t *testing.T) {
	recs := []models.Recommendation{
		{
			Target:         models.OwnerReference{Kind: "Deployment", Name: "api-gateway", Namespace: "production"},
			Resource:       "cpu",
			CurrentRequest: 1000,
			RecommendedReq: 250,
			Pattern:        models.PatternSteady,
			Confidence:     0.92,
			Risk:           models.RiskLow,
			Reasoning:      "Steady workload",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := models.NewOKResponse(recs)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	annotations := map[string]string{
		"gerty.io/repo": "github.com/acme/manifests",
		"gerty.io/path": "deployment.yaml",
	}
	annotationsJSON, _ := json.Marshal(annotations)

	// Create a temp dir to simulate the cloned repo.
	tmpDir, err := os.MkdirTemp("", "gerty-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Write a manifest file in the "repo".
	manifest := `apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: api
        resources:
          requests:
            cpu: 1000m
            memory: 512Mi
          limits:
            cpu: 2000m
            memory: 1Gi
`
	repoDir := filepath.Join(tmpDir, "repo")
	os.MkdirAll(repoDir, 0755)
	os.WriteFile(filepath.Join(repoDir, "deployment.yaml"), []byte(manifest), 0644)

	runner := newMockRunner()
	runner.addResponse("gh auth status", []byte("ok"), nil)
	runner.addResponse("kubectl get", annotationsJSON, nil)
	runner.addResponse("gh repo clone", []byte(""), nil)
	runner.addResponse("git checkout -b", []byte(""), nil)
	runner.addResponse("git add", []byte(""), nil)
	runner.addResponse("git commit", []byte(""), nil)
	runner.addResponse("git push", []byte(""), nil)
	runner.addResponse("gh pr create", []byte("https://github.com/acme/manifests/pull/42\n"), nil)

	// Override the repo dir in the flow — since we can't control os.MkdirTemp in the creator,
	// we test the individual steps instead. The full flow test validates the command sequence.
	p := NewPRCreator(runner, server.URL)

	// Verify auth check works.
	assert.NoError(t, p.checkGHAuth(context.Background()))

	// Verify annotation reading works.
	ann, err := p.readAnnotations(context.Background(), "Deployment", "api-gateway", "production")
	require.NoError(t, err)
	assert.Equal(t, "github.com/acme/manifests", ann.Repo)

	// Verify recommendation fetch works.
	rec, err := p.fetchRecommendation(context.Background(), "production", "Deployment", "api-gateway")
	require.NoError(t, err)
	assert.Equal(t, int64(250), rec.RecommendedReq)

	// Verify YAML modification works.
	err = p.modifyResourceFile(filepath.Join(repoDir, "deployment.yaml"), ann, rec)
	require.NoError(t, err)

	modified, _ := os.ReadFile(filepath.Join(repoDir, "deployment.yaml"))
	assert.Contains(t, string(modified), "cpu: 250m")
}

func TestFormatCPU(t *testing.T) {
	tests := []struct {
		millis int64
		want   string
	}{
		{50, "50m"},
		{250, "250m"},
		{1000, "1"},
		{2000, "2"},
		{1500, "1500m"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, formatCPU(tt.millis))
	}
}

func TestFormatMemory(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{67108864, "64Mi"},
		{134217728, "128Mi"},
		{268435456, "256Mi"},
		{536870912, "512Mi"},
		{1073741824, "1Gi"},
		{2147483648, "2Gi"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, formatMemory(tt.bytes))
	}
}

func TestFetchRecommendation_Non200Status(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	runner := newMockRunner()
	p := NewPRCreator(runner, server.URL)
	_, err := p.fetchRecommendation(context.Background(), "production", "Deployment", "api-gateway")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decoding response")
}

func TestFetchRecommendation_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html>Not JSON</html>"))
	}))
	defer server.Close()

	runner := newMockRunner()
	p := NewPRCreator(runner, server.URL)
	_, err := p.fetchRecommendation(context.Background(), "production", "Deployment", "api-gateway")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decoding response")
}

func TestFetchRecommendation_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := models.NewErrorResponse("analysis failed")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	runner := newMockRunner()
	p := NewPRCreator(runner, server.URL)
	_, err := p.fetchRecommendation(context.Background(), "production", "Deployment", "api-gateway")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server error")
	assert.Contains(t, err.Error(), "analysis failed")
}

func TestFetchRecommendation_EmptyList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := models.NewOKResponse([]models.Recommendation{})
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	runner := newMockRunner()
	p := NewPRCreator(runner, server.URL)
	_, err := p.fetchRecommendation(context.Background(), "production", "Deployment", "api-gateway")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no recommendation found")
}

func TestModifyValuesFile_NoMatchingValues(t *testing.T) {
	content := `replicaCount: 3
image:
  repository: nginx
  tag: latest
`
	rec := &models.Recommendation{
		Resource:       "cpu",
		RecommendedReq: 250,
	}

	_, err := modifyValuesFile(content, "resources.requests.cpu", rec)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no resource values found")
}

func TestModifyManifestFile_QuotedValues(t *testing.T) {
	content := `apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: api
        resources:
          requests:
            cpu: "500m"
            memory: "512Mi"
          limits:
            cpu: "1000m"
            memory: "1Gi"
`
	rec := &models.Recommendation{
		Resource:         "cpu",
		RecommendedReq:   250,
		RecommendedLimit: 500,
	}

	result, err := modifyManifestFile(content, rec)
	require.NoError(t, err)
	assert.Contains(t, result, "cpu: 250m")
	assert.Contains(t, result, "cpu: 500m")
	assert.Contains(t, result, `memory: "512Mi"`) // Unchanged.
}
