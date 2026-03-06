package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrCmd_RequiresArg(t *testing.T) {
	cmd := prCmd()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err)
}

func TestPrCmd_InvalidTarget(t *testing.T) {
	cmd := prCmd()
	cmd.SetArgs([]string{"invalid-no-slash"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "kind/name format")
}

func TestPrCmd_Flags(t *testing.T) {
	cmd := prCmd()

	// Verify flags exist.
	assert.NotNil(t, cmd.Flags().Lookup("namespace"))
	assert.NotNil(t, cmd.Flags().Lookup("branch-prefix"))
	assert.NotNil(t, cmd.Flags().Lookup("dry-run"))

	// Verify defaults.
	bp, _ := cmd.Flags().GetString("branch-prefix")
	assert.Equal(t, "gerty-rightsizing", bp)

	dr, _ := cmd.Flags().GetBool("dry-run")
	assert.False(t, dr)
}

func TestPrCmd_ShortFlag(t *testing.T) {
	cmd := prCmd()
	f := cmd.Flags().ShorthandLookup("n")
	assert.NotNil(t, f)
	assert.Equal(t, "namespace", f.Name)
}
