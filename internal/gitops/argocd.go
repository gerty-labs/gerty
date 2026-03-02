package gitops

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// argoAppList is the kubectl JSON output for ArgoCD Application list.
type argoAppList struct {
	Items []argoApp `json:"items"`
}

type argoApp struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec struct {
		Source struct {
			RepoURL string `json:"repoURL"`
			Path    string `json:"path"`
		} `json:"source"`
	} `json:"spec"`
	Status struct {
		Resources []argoResource `json:"resources"`
	} `json:"status"`
}

type argoResource struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// discoverArgoCD checks for ArgoCD CRD and parses Application resources.
func (d *Discoverer) discoverArgoCD(ctx context.Context) ([]GitOpsMapping, error) {
	// Check if ArgoCD CRD exists.
	_, err := d.runner.Run(ctx, "kubectl", "api-resources", "--api-group=argoproj.io", "--no-headers")
	if err != nil {
		return nil, fmt.Errorf("argocd not installed: %w", err)
	}

	// List all ArgoCD Applications.
	out, err := d.runner.Run(ctx, "kubectl", "get", "applications.argoproj.io",
		"--all-namespaces", "-o", "json")
	if err != nil {
		return nil, fmt.Errorf("listing argocd applications: %w", err)
	}

	var appList argoAppList
	if err := json.Unmarshal(out, &appList); err != nil {
		return nil, fmt.Errorf("parsing argocd applications: %w", err)
	}

	var mappings []GitOpsMapping
	for _, app := range appList.Items {
		repoURL := app.Spec.Source.RepoURL
		path := app.Spec.Source.Path

		for _, res := range app.Status.Resources {
			if !isWorkloadKind(res.Kind) {
				continue
			}
			mappings = append(mappings, GitOpsMapping{
				Namespace:  res.Namespace,
				Kind:       res.Kind,
				Name:       res.Name,
				RepoURL:    repoURL,
				Path:       path,
				SourceTool: "argocd",
			})
		}
	}

	return mappings, nil
}

// isWorkloadKind returns true for Deployment, StatefulSet, DaemonSet.
func isWorkloadKind(kind string) bool {
	switch strings.ToLower(kind) {
	case "deployment", "statefulset", "daemonset":
		return true
	}
	return false
}
