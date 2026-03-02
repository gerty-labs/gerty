package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func reportCmd() *cobra.Command {
	var namespace string

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Show cluster or namespace waste report",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := NewClient(resolveServerAddr())

			if namespace != "" {
				report, err := client.GetNamespaceReport(namespace)
				if err != nil {
					return fmt.Errorf("fetching namespace report: %w", err)
				}
				if outputFmt == "json" {
					return printJSON(os.Stdout, report)
				}
				writeNamespaceReportTable(os.Stdout, report)
				return nil
			}

			report, err := client.GetClusterReport()
			if err != nil {
				return fmt.Errorf("fetching cluster report: %w", err)
			}
			if outputFmt == "json" {
				return printJSON(os.Stdout, report)
			}
			writeClusterReportTable(os.Stdout, report)
			return nil
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "filter to a specific namespace")
	return cmd
}
