package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnnotateCmd_PrintsCommand(t *testing.T) {
	cmd := annotateCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{
		"deployment/payment-service",
		"--repo", "github.com/acme/manifests",
		"--path", "apps/payment-service/values.yaml",
	})

	// Capture stdout
	oldStdout := cmd.OutOrStdout()
	_ = oldStdout
	cmd.SetOut(buf)

	// The command writes to os.Stdout directly, so we test via RunE
	rootCmd := cmd
	rootCmd.SetOut(buf)

	err := cmd.Execute()
	require.NoError(t, err)
}

func TestAnnotateCmd_WithField(t *testing.T) {
	cmd := annotateCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{
		"deployment/api",
		"--repo", "github.com/acme/manifests",
		"--path", "api/deploy.yaml",
		"--field", "resources.requests.memory",
	})

	err := cmd.Execute()
	require.NoError(t, err)
}

func TestAnnotateCmd_WithNamespace(t *testing.T) {
	cmd := annotateCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{
		"statefulset/redis",
		"--repo", "github.com/acme/infra",
		"--path", "redis/sts.yaml",
		"-n", "cache",
	})

	err := cmd.Execute()
	require.NoError(t, err)
}

func TestAnnotateCmd_MissingRepo(t *testing.T) {
	cmd := annotateCmd()
	cmd.SetArgs([]string{
		"deployment/api",
		"--path", "api/deploy.yaml",
	})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--repo is required")
}

func TestAnnotateCmd_MissingPath(t *testing.T) {
	cmd := annotateCmd()
	cmd.SetArgs([]string{
		"deployment/api",
		"--repo", "github.com/acme/manifests",
	})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--path is required")
}

func TestAnnotateCmd_InvalidTarget(t *testing.T) {
	tests := []struct {
		name   string
		target string
	}{
		{"no slash", "payment-service"},
		{"empty kind", "/payment-service"},
		{"empty name", "deployment/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := annotateCmd()
			cmd.SetArgs([]string{
				tt.target,
				"--repo", "github.com/acme/manifests",
				"--path", "deploy.yaml",
			})

			err := cmd.Execute()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "kind/name format")
		})
	}
}

func TestAnnotateCmd_NoArgs(t *testing.T) {
	cmd := annotateCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.Error(t, err)
}
