package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

func annotateCmd() *cobra.Command {
	var (
		repo      string
		path      string
		field     string
		namespace string
		apply     bool
	)

	cmd := &cobra.Command{
		Use:   "annotate <kind/name>",
		Short: "Add GitOps source annotations to a Kubernetes resource",
		Long: `Annotate a Kubernetes resource with GitOps source metadata.

By default, prints the kubectl annotate command. Use --apply to execute it.

Annotations set:
  gerty.io/repo   — Git repository URL
  gerty.io/path   — File path within the repo
  gerty.io/field  — Resource field path (optional)`,
		Example: `  gerty annotate deployment/payment-service \
    --repo github.com/acme/manifests \
    --path apps/payment-service/values.yaml \
    --field resources.requests.memory

  gerty annotate statefulset/redis \
    --repo github.com/acme/infra \
    --path redis/deployment.yaml \
    --apply -n cache`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]

			// Validate target format: kind/name
			parts := strings.SplitN(target, "/", 2)
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				return fmt.Errorf("target must be in kind/name format (got %q)", target)
			}

			if repo == "" {
				return fmt.Errorf("--repo is required")
			}
			if path == "" {
				return fmt.Errorf("--path is required")
			}

			// Reject values that could be misinterpreted as kubectl flags.
			for _, v := range []string{repo, path, field} {
				if strings.HasPrefix(v, "-") {
					return fmt.Errorf("flag values must not start with '-' (got %q)", v)
				}
			}

			annotations := []string{
				fmt.Sprintf("gerty.io/repo=%s", repo),
				fmt.Sprintf("gerty.io/path=%s", path),
			}
			if field != "" {
				annotations = append(annotations, fmt.Sprintf("gerty.io/field=%s", field))
			}

			kubectlArgs := []string{"annotate", target}
			if namespace != "" {
				kubectlArgs = append(kubectlArgs, "-n", namespace)
			}
			kubectlArgs = append(kubectlArgs, annotations...)
			kubectlArgs = append(kubectlArgs, "--overwrite")

			if !apply {
				fmt.Fprintf(os.Stdout, "kubectl %s\n", strings.Join(kubectlArgs, " "))
				return nil
			}

			execCmd := exec.Command("kubectl", kubectlArgs...)
			execCmd.Stdout = os.Stdout
			execCmd.Stderr = os.Stderr
			if err := execCmd.Run(); err != nil {
				return fmt.Errorf("kubectl annotate failed: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "Git repository URL (required)")
	cmd.Flags().StringVar(&path, "path", "", "File path within the repo (required)")
	cmd.Flags().StringVar(&field, "field", "", "Resource field path (optional)")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Kubernetes namespace")
	cmd.Flags().BoolVar(&apply, "apply", false, "Execute the kubectl command instead of printing it")

	return cmd
}
