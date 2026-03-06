package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func recommendCmd() *cobra.Command {
	var risk string

	cmd := &cobra.Command{
		Use:   "recommend",
		Short: "Show right-sizing recommendations",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := NewClient(resolveServerAddr())

			recs, err := client.GetRecommendations(risk, "")
			if err != nil {
				return fmt.Errorf("fetching recommendations: %w", err)
			}

			if outputFmt == "json" {
				return printJSON(os.Stdout, recs)
			}

			if len(recs) == 0 {
				fmt.Fprintln(os.Stdout, "No recommendations available.")
				return nil
			}

			writeRecommendationsTable(os.Stdout, recs)
			return nil
		},
	}

	cmd.Flags().StringVar(&risk, "risk", "", "filter by risk level: LOW, MEDIUM, HIGH")
	return cmd
}
