package gitops

import (
	"context"
	"encoding/json"
	"fmt"
)

// fluxKustomizationList is the kubectl JSON output for Flux Kustomization list.
type fluxKustomizationList struct {
	Items []fluxKustomization `json:"items"`
}

type fluxMetadata struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type fluxSourceRef struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type fluxKustomizationSpec struct {
	Path      string        `json:"path"`
	SourceRef fluxSourceRef `json:"sourceRef"`
}

type fluxInventory struct {
	Entries []fluxInventoryEntry `json:"entries"`
}

type fluxKustomizationStatus struct {
	Inventory fluxInventory `json:"inventory"`
}

type fluxKustomization struct {
	Metadata fluxMetadata            `json:"metadata"`
	Spec     fluxKustomizationSpec   `json:"spec"`
	Status   fluxKustomizationStatus `json:"status"`
}

type fluxInventoryEntry struct {
	ID string `json:"id"` // format: namespace_name_group_kind
}

// fluxGitRepoList is the kubectl JSON output for Flux GitRepository list.
type fluxGitRepoList struct {
	Items []fluxGitRepo `json:"items"`
}

type fluxGitRepoSpec struct {
	URL string `json:"url"`
}

type fluxGitRepo struct {
	Metadata fluxMetadata   `json:"metadata"`
	Spec     fluxGitRepoSpec `json:"spec"`
}

// discoverFlux checks for Flux CRDs and parses Kustomization + GitRepository resources.
func (d *Discoverer) discoverFlux(ctx context.Context) ([]GitOpsMapping, error) {
	// Check if Flux CRD exists.
	_, err := d.runner.Run(ctx, "kubectl", "api-resources", "--api-group=kustomize.toolkit.fluxcd.io", "--no-headers")
	if err != nil {
		return nil, fmt.Errorf("flux not installed: %w", err)
	}

	// Get all GitRepositories to resolve URLs.
	repoMap, err := d.getFluxRepoMap(ctx)
	if err != nil {
		return nil, err
	}

	// List all Kustomizations.
	out, err := d.runner.Run(ctx, "kubectl", "get", "kustomizations.kustomize.toolkit.fluxcd.io",
		"--all-namespaces", "-o", "json")
	if err != nil {
		return nil, fmt.Errorf("listing flux kustomizations: %w", err)
	}

	var ksList fluxKustomizationList
	if err := json.Unmarshal(out, &ksList); err != nil {
		return nil, fmt.Errorf("parsing flux kustomizations: %w", err)
	}

	var mappings []GitOpsMapping
	for _, ks := range ksList.Items {
		// Resolve repo URL from sourceRef.
		repoKey := ks.Spec.SourceRef.Namespace + "/" + ks.Spec.SourceRef.Name
		if ks.Spec.SourceRef.Namespace == "" {
			repoKey = ks.Metadata.Namespace + "/" + ks.Spec.SourceRef.Name
		}
		repoURL := repoMap[repoKey]

		// Parse inventory entries for workload resources.
		for _, entry := range ks.Status.Inventory.Entries {
			ns, name, kind, ok := parseFluxInventoryID(entry.ID)
			if !ok {
				continue
			}
			if !isWorkloadKind(kind) {
				continue
			}

			mappings = append(mappings, GitOpsMapping{
				Namespace:  ns,
				Kind:       kind,
				Name:       name,
				RepoURL:    repoURL,
				Path:       ks.Spec.Path,
				SourceTool: "flux",
			})
		}
	}

	return mappings, nil
}

// getFluxRepoMap builds a map of namespace/name → URL for all GitRepositories.
func (d *Discoverer) getFluxRepoMap(ctx context.Context) (map[string]string, error) {
	out, err := d.runner.Run(ctx, "kubectl", "get", "gitrepositories.source.toolkit.fluxcd.io",
		"--all-namespaces", "-o", "json")
	if err != nil {
		return nil, fmt.Errorf("listing flux gitrepositories: %w", err)
	}

	var repoList fluxGitRepoList
	if err := json.Unmarshal(out, &repoList); err != nil {
		return nil, fmt.Errorf("parsing flux gitrepositories: %w", err)
	}

	repoMap := make(map[string]string, len(repoList.Items))
	for _, repo := range repoList.Items {
		key := repo.Metadata.Namespace + "/" + repo.Metadata.Name
		repoMap[key] = repo.Spec.URL
	}
	return repoMap, nil
}

// parseFluxInventoryID parses a Flux inventory entry ID.
// Format: namespace_name_group_kind (e.g., "default_nginx_apps_Deployment")
func parseFluxInventoryID(id string) (namespace, name, kind string, ok bool) {
	// Split from the right to handle names with underscores.
	// Format: namespace_name_group_kind
	parts := splitFluxID(id)
	if len(parts) < 4 {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[len(parts)-1], true
}

// splitFluxID splits a Flux inventory ID into its components.
// The format is namespace_name_group_kind where name may contain underscores.
func splitFluxID(id string) []string {
	// Kind is always last, group is second-to-last.
	// We need at least: namespace, name, group, kind
	parts := make([]string, 0, 4)

	// Find kind (after last underscore)
	lastIdx := lastIndex(id, '_')
	if lastIdx < 0 {
		return nil
	}
	kind := id[lastIdx+1:]
	rest := id[:lastIdx]

	// Find group (after second-to-last underscore)
	groupIdx := lastIndex(rest, '_')
	if groupIdx < 0 {
		return nil
	}
	rest = rest[:groupIdx]

	// Find namespace (before first underscore)
	nsIdx := firstIndex(rest, '_')
	if nsIdx < 0 {
		return nil
	}
	namespace := rest[:nsIdx]
	name := rest[nsIdx+1:]

	parts = append(parts, namespace, name, "", kind)
	return parts
}

func lastIndex(s string, b byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func firstIndex(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
