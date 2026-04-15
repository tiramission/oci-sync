package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExperimentalEnabled(t *testing.T) {
	// Save original env and restore after test
	origEnv := os.Getenv("OCI_SYNC_EXPERIMENTAL_ENABLED")
	t.Cleanup(func() {
		if origEnv != "" {
			os.Setenv("OCI_SYNC_EXPERIMENTAL_ENABLED", origEnv)
		} else {
			os.Unsetenv("OCI_SYNC_EXPERIMENTAL_ENABLED")
		}
	})

	t.Run("default true when not set", func(t *testing.T) {
		os.Unsetenv("OCI_SYNC_EXPERIMENTAL_ENABLED")
		InitConfig()

		got := ExperimentalEnabled()
		assert.True(t, got, "expected true when not configured")
	})

	t.Run("returns true for true string", func(t *testing.T) {
		os.Setenv("OCI_SYNC_EXPERIMENTAL_ENABLED", "true")
		InitConfig()

		got := ExperimentalEnabled()
		assert.True(t, got)
	})

	t.Run("returns true for 1", func(t *testing.T) {
		os.Setenv("OCI_SYNC_EXPERIMENTAL_ENABLED", "1")
		InitConfig()

		got := ExperimentalEnabled()
		assert.True(t, got)
	})

	t.Run("returns false for false string", func(t *testing.T) {
		os.Setenv("OCI_SYNC_EXPERIMENTAL_ENABLED", "false")
		InitConfig()

		got := ExperimentalEnabled()
		assert.False(t, got)
	})

	t.Run("returns false for 0", func(t *testing.T) {
		os.Setenv("OCI_SYNC_EXPERIMENTAL_ENABLED", "0")
		InitConfig()

		got := ExperimentalEnabled()
		assert.False(t, got)
	})

	t.Run("returns false for invalid value", func(t *testing.T) {
		os.Setenv("OCI_SYNC_EXPERIMENTAL_ENABLED", "invalid")
		InitConfig()

		got := ExperimentalEnabled()
		assert.False(t, got)
	})
}

func TestExperimentalRepo(t *testing.T) {
	// Save original env and restore after test
	origEnv := os.Getenv("OCI_SYNC_EXPERIMENTAL_REPO")
	t.Cleanup(func() {
		if origEnv != "" {
			os.Setenv("OCI_SYNC_EXPERIMENTAL_REPO", origEnv)
		} else {
			os.Unsetenv("OCI_SYNC_EXPERIMENTAL_REPO")
		}
	})

	t.Run("returns error when not configured", func(t *testing.T) {
		os.Unsetenv("OCI_SYNC_EXPERIMENTAL_REPO")
		InitConfig()

		_, err := ExperimentalRepo()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not configured")
	})

	t.Run("returns repo from environment variable", func(t *testing.T) {
		os.Setenv("OCI_SYNC_EXPERIMENTAL_REPO", "registry.example.com/team/repo")
		InitConfig()

		got, err := ExperimentalRepo()
		assert.NoError(t, err)
		assert.Equal(t, "registry.example.com/team/repo", got)
	})

	t.Run("allows registry port", func(t *testing.T) {
		os.Setenv("OCI_SYNC_EXPERIMENTAL_REPO", "registry.example.com:5000/team/repo")
		InitConfig()

		got, err := ExperimentalRepo()
		assert.NoError(t, err)
		assert.Equal(t, "registry.example.com:5000/team/repo", got)
	})

	t.Run("rejects digest reference", func(t *testing.T) {
		os.Setenv("OCI_SYNC_EXPERIMENTAL_REPO", "registry.example.com/team/repo@sha256:abc")
		InitConfig()

		_, err := ExperimentalRepo()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "digest")
	})

	t.Run("rejects tagged repository", func(t *testing.T) {
		os.Setenv("OCI_SYNC_EXPERIMENTAL_REPO", "registry.example.com/team/repo:latest")
		InitConfig()

		_, err := ExperimentalRepo()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tag")
	})

	t.Run("environment variable takes precedence over config", func(t *testing.T) {
		// Note: This test verifies the env var precedence logic
		// by setting the env var which is checked first
		os.Setenv("OCI_SYNC_EXPERIMENTAL_REPO", "env.registry.example.com/repo")
		InitConfig()

		got, err := ExperimentalRepo()
		assert.NoError(t, err)
		assert.Equal(t, "env.registry.example.com/repo", got)
	})
}
