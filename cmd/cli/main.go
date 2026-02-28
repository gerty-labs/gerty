package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	serverAddr string
	outputFmt  string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "sage",
		Short: "sage-cli — Kubernetes efficiency insights",
	}

	rootCmd.PersistentFlags().StringVar(&serverAddr, "server", "", "sage-server address (default: env SAGE_SERVER or http://localhost:8080)")
	rootCmd.PersistentFlags().StringVarP(&outputFmt, "output", "o", "table", "output format: table or json")

	rootCmd.AddCommand(reportCmd())
	rootCmd.AddCommand(recommendCmd())
	rootCmd.AddCommand(workloadsCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// resolveServerAddr returns the sage-server address from flag, env, or default.
func resolveServerAddr() string {
	if serverAddr != "" {
		return serverAddr
	}
	if env := os.Getenv("SAGE_SERVER"); env != "" {
		return env
	}
	return "http://localhost:8080"
}
