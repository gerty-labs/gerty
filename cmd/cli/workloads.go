package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func workloadsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workloads [ns/kind/name]",
		Short: "List workloads or show workload detail",
		Long:  "List all workloads, or show detail for a specific workload by namespace/kind/name.",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := NewClient(resolveServerAddr())

			if len(args) == 0 {
				// List mode.
				workloads, err := client.GetWorkloads()
				if err != nil {
					return fmt.Errorf("fetching workloads: %w", err)
				}
				if outputFmt == "json" {
					return printJSON(os.Stdout, workloads)
				}
				if len(workloads) == 0 {
					fmt.Fprintln(os.Stdout, "No workloads found.")
					return nil
				}
				writeWorkloadsTable(os.Stdout, workloads)
				return nil
			}

			// Detail mode: parse ns/kind/name
			parts := strings.SplitN(args[0], "/", 3)
			if len(parts) != 3 {
				return fmt.Errorf("expected format: namespace/kind/name (got %q)", args[0])
			}

			ow, err := client.GetWorkload(parts[0], parts[1], parts[2])
			if err != nil {
				return fmt.Errorf("fetching workload: %w", err)
			}
			if outputFmt == "json" {
				return printJSON(os.Stdout, ow)
			}
			writeWorkloadDetailTable(os.Stdout, ow)
			return nil
		},
	}

	return cmd
}
