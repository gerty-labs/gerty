package gitops

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRunner implements CommandRunner for testing.
type mockRunner struct {
	responses map[string]mockResponse
}

type mockResponse struct {
	output []byte
	err    error
}

func (m *mockRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	// Build a key from the command + first few args for matching.
	key := name
	for _, a := range args {
		key += " " + a
	}

	for pattern, resp := range m.responses {
		if key == pattern || contains(key, pattern) {
			return resp.output, resp.err
		}
	}
	return nil, fmt.Errorf("no mock for: %s", key)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && searchSubstr(s, substr))
}

func searchSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func makeArgoApps(apps ...argoApp) []byte {
	list := argoAppList{Items: apps}
	b, _ := json.Marshal(list)
	return b
}

func makeFluxKustomizations(kss ...fluxKustomization) []byte {
	list := fluxKustomizationList{Items: kss}
	b, _ := json.Marshal(list)
	return b
}

func makeFluxGitRepos(repos ...fluxGitRepo) []byte {
	list := fluxGitRepoList{Items: repos}
	b, _ := json.Marshal(list)
	return b
}

func TestDiscover_ArgoCDFound(t *testing.T) {
	app := argoApp{}
	app.Metadata.Name = "my-app"
	app.Metadata.Namespace = "argocd"
	app.Spec.Source.RepoURL = "https://github.com/acme/manifests"
	app.Spec.Source.Path = "apps/web"
	app.Status.Resources = []argoResource{
		{Kind: "Deployment", Name: "web", Namespace: "production"},
		{Kind: "Service", Name: "web-svc", Namespace: "production"},
		{Kind: "StatefulSet", Name: "redis", Namespace: "production"},
	}

	runner := &mockRunner{responses: map[string]mockResponse{
		"kubectl api-resources --api-group=argoproj.io --no-headers":                         {output: []byte("applications")},
		"kubectl get applications.argoproj.io --all-namespaces -o json":                      {output: makeArgoApps(app)},
		"kubectl api-resources --api-group=kustomize.toolkit.fluxcd.io --no-headers":         {err: fmt.Errorf("not found")},
	}}

	d := NewDiscoverer(runner)
	mappings, err := d.Discover(context.Background())
	require.NoError(t, err)
	assert.Len(t, mappings, 2) // Deployment + StatefulSet (Service filtered)

	assert.Equal(t, "Deployment", mappings[0].Kind)
	assert.Equal(t, "web", mappings[0].Name)
	assert.Equal(t, "production", mappings[0].Namespace)
	assert.Equal(t, "https://github.com/acme/manifests", mappings[0].RepoURL)
	assert.Equal(t, "apps/web", mappings[0].Path)
	assert.Equal(t, "argocd", mappings[0].SourceTool)

	assert.Equal(t, "StatefulSet", mappings[1].Kind)
	assert.Equal(t, "redis", mappings[1].Name)
}

func TestDiscover_ArgoCDNotInstalled(t *testing.T) {
	runner := &mockRunner{responses: map[string]mockResponse{
		"kubectl api-resources --api-group=argoproj.io --no-headers":                 {err: fmt.Errorf("not found")},
		"kubectl api-resources --api-group=kustomize.toolkit.fluxcd.io --no-headers": {err: fmt.Errorf("not found")},
	}}

	d := NewDiscoverer(runner)
	mappings, err := d.Discover(context.Background())
	require.NoError(t, err)
	assert.Empty(t, mappings)
}

func TestDiscover_FluxFound(t *testing.T) {
	repo := fluxGitRepo{}
	repo.Metadata.Name = "infra"
	repo.Metadata.Namespace = "flux-system"
	repo.Spec.URL = "https://github.com/acme/infra"

	ks := fluxKustomization{}
	ks.Metadata.Name = "apps"
	ks.Metadata.Namespace = "flux-system"
	ks.Spec.Path = "./clusters/prod"
	ks.Spec.SourceRef.Kind = "GitRepository"
	ks.Spec.SourceRef.Name = "infra"
	ks.Spec.SourceRef.Namespace = "flux-system"
	ks.Status.Inventory.Entries = []fluxInventoryEntry{
		{ID: "default_nginx_apps_Deployment"},
		{ID: "default_nginx-svc_core_Service"},
		{ID: "monitoring_prometheus_apps_StatefulSet"},
	}

	runner := &mockRunner{responses: map[string]mockResponse{
		"kubectl api-resources --api-group=argoproj.io --no-headers":                                           {err: fmt.Errorf("not found")},
		"kubectl api-resources --api-group=kustomize.toolkit.fluxcd.io --no-headers":                           {output: []byte("kustomizations")},
		"kubectl get gitrepositories.source.toolkit.fluxcd.io --all-namespaces -o json":                        {output: makeFluxGitRepos(repo)},
		"kubectl get kustomizations.kustomize.toolkit.fluxcd.io --all-namespaces -o json":                      {output: makeFluxKustomizations(ks)},
	}}

	d := NewDiscoverer(runner)
	mappings, err := d.Discover(context.Background())
	require.NoError(t, err)
	assert.Len(t, mappings, 2) // Deployment + StatefulSet (Service filtered)

	assert.Equal(t, "Deployment", mappings[0].Kind)
	assert.Equal(t, "nginx", mappings[0].Name)
	assert.Equal(t, "default", mappings[0].Namespace)
	assert.Equal(t, "https://github.com/acme/infra", mappings[0].RepoURL)
	assert.Equal(t, "flux", mappings[0].SourceTool)

	assert.Equal(t, "StatefulSet", mappings[1].Kind)
	assert.Equal(t, "prometheus", mappings[1].Name)
	assert.Equal(t, "monitoring", mappings[1].Namespace)
}

func TestDiscover_FluxNotInstalled(t *testing.T) {
	runner := &mockRunner{responses: map[string]mockResponse{
		"kubectl api-resources --api-group=argoproj.io --no-headers":                 {err: fmt.Errorf("not found")},
		"kubectl api-resources --api-group=kustomize.toolkit.fluxcd.io --no-headers": {err: fmt.Errorf("not found")},
	}}

	d := NewDiscoverer(runner)
	mappings, err := d.Discover(context.Background())
	require.NoError(t, err)
	assert.Empty(t, mappings)
}

func TestDiscover_BothTools(t *testing.T) {
	argoApp := argoApp{}
	argoApp.Metadata.Name = "argo-app"
	argoApp.Metadata.Namespace = "argocd"
	argoApp.Spec.Source.RepoURL = "https://github.com/acme/argo"
	argoApp.Spec.Source.Path = "apps"
	argoApp.Status.Resources = []argoResource{
		{Kind: "Deployment", Name: "api", Namespace: "prod"},
	}

	repo := fluxGitRepo{}
	repo.Metadata.Name = "flux-repo"
	repo.Metadata.Namespace = "flux-system"
	repo.Spec.URL = "https://github.com/acme/flux"

	ks := fluxKustomization{}
	ks.Metadata.Name = "infra"
	ks.Metadata.Namespace = "flux-system"
	ks.Spec.Path = "./infra"
	ks.Spec.SourceRef.Kind = "GitRepository"
	ks.Spec.SourceRef.Name = "flux-repo"
	ks.Spec.SourceRef.Namespace = "flux-system"
	ks.Status.Inventory.Entries = []fluxInventoryEntry{
		{ID: "monitoring_grafana_apps_Deployment"},
	}

	runner := &mockRunner{responses: map[string]mockResponse{
		"kubectl api-resources --api-group=argoproj.io --no-headers":                                           {output: []byte("applications")},
		"kubectl get applications.argoproj.io --all-namespaces -o json":                                        {output: makeArgoApps(argoApp)},
		"kubectl api-resources --api-group=kustomize.toolkit.fluxcd.io --no-headers":                           {output: []byte("kustomizations")},
		"kubectl get gitrepositories.source.toolkit.fluxcd.io --all-namespaces -o json":                        {output: makeFluxGitRepos(repo)},
		"kubectl get kustomizations.kustomize.toolkit.fluxcd.io --all-namespaces -o json":                      {output: makeFluxKustomizations(ks)},
	}}

	d := NewDiscoverer(runner)
	mappings, err := d.Discover(context.Background())
	require.NoError(t, err)
	assert.Len(t, mappings, 2)
	assert.Equal(t, "argocd", mappings[0].SourceTool)
	assert.Equal(t, "flux", mappings[1].SourceTool)
}

func TestDiscover_Neither(t *testing.T) {
	runner := &mockRunner{responses: map[string]mockResponse{
		"kubectl api-resources --api-group=argoproj.io --no-headers":                 {err: fmt.Errorf("not found")},
		"kubectl api-resources --api-group=kustomize.toolkit.fluxcd.io --no-headers": {err: fmt.Errorf("not found")},
	}}

	d := NewDiscoverer(runner)
	mappings, err := d.Discover(context.Background())
	require.NoError(t, err)
	assert.Empty(t, mappings)
}

func TestGenerateAnnotateCommands(t *testing.T) {
	mappings := []GitOpsMapping{
		{Namespace: "prod", Kind: "Deployment", Name: "api", RepoURL: "https://github.com/acme/manifests", Path: "apps/api"},
		{Namespace: "cache", Kind: "StatefulSet", Name: "redis", RepoURL: "https://github.com/acme/infra", Path: "redis"},
	}

	cmds := GenerateAnnotateCommands(mappings)
	require.Len(t, cmds, 2)
	assert.Equal(t, "gerty annotate deployment/api -n prod --repo https://github.com/acme/manifests --path apps/api", cmds[0].Display)
	assert.Equal(t, []string{"annotate", "deployment/api", "-n", "prod", "--repo", "https://github.com/acme/manifests", "--path", "apps/api"}, cmds[0].Args)
	assert.Equal(t, "gerty annotate statefulset/redis -n cache --repo https://github.com/acme/infra --path redis", cmds[1].Display)
}
