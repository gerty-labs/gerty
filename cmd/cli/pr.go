package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/gregorytcarroll/k8s-sage/internal/pr"
	"github.com/spf13/cobra"
)

// cliAllowedCommands is the set of binaries that cliRunner will execute.
var cliAllowedCommands = map[string]bool{
	"gh":      true,
	"git":     true,
	"kubectl": true,
}

// cliRunner implements pr.CommandRunner using os/exec.
// It restricts execution to cliAllowedCommands to prevent command injection.
type cliRunner struct{}

func (r *cliRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if !cliAllowedCommands[name] {
		return nil, fmt.Errorf("command %q not in allowlist", name)
	}
	return exec.CommandContext(ctx, name, args...).Output()
}

func (r *cliRunner) RunInDir(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
	if !cliAllowedCommands[name] {
		return nil, fmt.Errorf("command %q not in allowlist", name)
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd.Output()
}

func prCmd() *cobra.Command {
	var (
		namespace    string
		branchPrefix string
		dryRun       bool
	)

	cmd := &cobra.Command{
		Use:   "pr <kind/name>",
		Short: "Create a PR with right-sizing recommendations",
		Long: `Create a GitHub pull request that applies right-sizing recommendations
to the source manifest or Helm values file.

Requires:
  - gh CLI installed and authenticated (gh auth login)
  - Target workload annotated with sage.io/repo and sage.io/path
  - sage-server running with recommendations available

Use --dry-run to preview changes without creating a PR.`,
		Example: `  sage pr deployment/api-gateway
  sage pr deployment/api-gateway -n production
  sage pr statefulset/redis --dry-run
  sage pr deployment/api-gateway --branch-prefix sage-resize`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]
			parts := strings.SplitN(target, "/", 2)
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				return fmt.Errorf("target must be in kind/name format (got %q)", target)
			}

			kind := parts[0]
			name := parts[1]
			serverURL := resolveServerAddr()

			creator := pr.NewPRCreator(&cliRunner{}, serverURL)
			result, err := creator.Create(context.Background(), pr.Options{
				Kind:         kind,
				Name:         name,
				Namespace:    namespace,
				BranchPrefix: branchPrefix,
				DryRun:       dryRun,
			})
			if err != nil {
				return err
			}

			if result.DryRun {
				fmt.Fprintln(os.Stdout, "=== Dry Run ===")
				fmt.Fprintf(os.Stdout, "Target:     %s/%s\n", kind, name)
				fmt.Fprintf(os.Stdout, "Namespace:  %s\n", namespace)
				fmt.Fprintf(os.Stdout, "File:       %s\n", result.FilePath)
				fmt.Fprintf(os.Stdout, "Branch:     %s\n", result.Branch)
				fmt.Fprintf(os.Stdout, "Title:      %s\n", result.Title)
				fmt.Fprintln(os.Stdout, "")
				if result.Recommendation != nil {
					rec := result.Recommendation
					fmt.Fprintf(os.Stdout, "Resource:    %s\n", rec.Resource)
					fmt.Fprintf(os.Stdout, "Pattern:     %s\n", rec.Pattern)
					fmt.Fprintf(os.Stdout, "Confidence:  %.0f%%\n", rec.Confidence*100)
					fmt.Fprintf(os.Stdout, "Current:     %d -> Recommended: %d\n", rec.CurrentRequest, rec.RecommendedReq)
				}
				fmt.Fprintln(os.Stdout, "")
				fmt.Fprintln(os.Stdout, "--- PR Body ---")
				fmt.Fprintln(os.Stdout, result.Body)
				return nil
			}

			fmt.Fprintf(os.Stdout, "PR created: %s\n", result.PRURL)
			return nil
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Kubernetes namespace")
	cmd.Flags().StringVar(&branchPrefix, "branch-prefix", "sage-rightsizing", "Branch name prefix")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview changes without creating a PR")

	return cmd
}
