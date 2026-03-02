package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/gregorytcarroll/k8s-sage/internal/gitops"
	"github.com/spf13/cobra"
)

// allowedCommands is the set of binaries that execRunner will execute.
var allowedCommands = map[string]bool{
	"kubectl": true,
}

// execRunner implements gitops.CommandRunner using os/exec.
// It restricts execution to allowedCommands to prevent command injection.
type execRunner struct{}

func (e *execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if !allowedCommands[name] {
		return nil, fmt.Errorf("command %q not in allowlist", name)
	}
	return exec.CommandContext(ctx, name, args...).Output()
}

func discoverCmd() *cobra.Command {
	var outputFormat string

	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Discover GitOps-managed workloads (ArgoCD + Flux)",
		Long: `Scan the cluster for ArgoCD Applications and Flux Kustomizations,
then map each managed Deployment/StatefulSet/DaemonSet to its Git source.

By default, prints sage annotate commands for each discovered mapping.
Use --output json for machine-readable output.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			discoverer := gitops.NewDiscoverer(&execRunner{})
			mappings, err := discoverer.Discover(context.Background())
			if err != nil {
				return fmt.Errorf("discovery failed: %w", err)
			}

			if len(mappings) == 0 {
				fmt.Fprintln(os.Stdout, "No GitOps-managed workloads found.")
				fmt.Fprintln(os.Stdout, "Tip: ensure ArgoCD or Flux is installed and kubectl has cluster access.")
				return nil
			}

			if outputFormat == "json" {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(mappings)
			}

			cmds := gitops.GenerateAnnotateCommands(mappings)
			for _, c := range cmds {
				fmt.Fprintln(os.Stdout, c)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "output format: text or json")

	return cmd
}
