package gitops

import (
	"context"
	"fmt"
	"strings"
)

// CommandRunner abstracts os/exec for testability.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// GitOpsMapping maps a Kubernetes workload to its GitOps source.
type GitOpsMapping struct {
	Namespace  string `json:"namespace"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	RepoURL    string `json:"repoURL"`
	Path       string `json:"path"`
	SourceTool string `json:"sourceTool"` // "argocd" or "flux"
}

// Discoverer finds GitOps-managed workloads using kubectl.
type Discoverer struct {
	runner CommandRunner
}

// NewDiscoverer creates a Discoverer with the given command runner.
func NewDiscoverer(runner CommandRunner) *Discoverer {
	return &Discoverer{runner: runner}
}

// Discover checks for ArgoCD and Flux CRDs and returns mappings for all
// discovered workloads. Returns an empty slice (not an error) if neither
// tool is installed.
func (d *Discoverer) Discover(ctx context.Context) ([]GitOpsMapping, error) {
	var mappings []GitOpsMapping

	argoMappings, err := d.discoverArgoCD(ctx)
	if err == nil {
		mappings = append(mappings, argoMappings...)
	}

	fluxMappings, err := d.discoverFlux(ctx)
	if err == nil {
		mappings = append(mappings, fluxMappings...)
	}

	return mappings, nil
}

// AnnotateCommand holds a structured gerty annotate invocation.
// Use Args with exec.Command("gerty", args...) — do not join into a shell string.
type AnnotateCommand struct {
	Args    []string // e.g., ["annotate", "deployment/nginx", "-n", "default", "--repo", "...", "--path", "..."]
	Display string   // human-readable form for display only
}

// GenerateAnnotateCommands returns structured gerty annotate commands for each mapping.
func GenerateAnnotateCommands(mappings []GitOpsMapping) []AnnotateCommand {
	var cmds []AnnotateCommand
	for _, m := range mappings {
		target := strings.ToLower(m.Kind) + "/" + m.Name
		args := []string{"annotate", target, "-n", m.Namespace, "--repo", m.RepoURL, "--path", m.Path}
		display := fmt.Sprintf("gerty annotate %s -n %s --repo %s --path %s",
			target, m.Namespace, m.RepoURL, m.Path)
		cmds = append(cmds, AnnotateCommand{Args: args, Display: display})
	}
	return cmds
}
