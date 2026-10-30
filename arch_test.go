package main

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoreHasNoWailsImports(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "-test", "./core/...").Output()
	require.NoError(t, err)
	assert.NotContains(t, string(out), "github.com/wailsapp/")
}
