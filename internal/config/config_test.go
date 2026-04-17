package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExperimentalEnabled(t *testing.T) {
	t.Run("default true when not configured", func(t *testing.T) {
		tmpDir := t.TempDir()
		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		globalConfig = nil
		InitConfig()

		got := ExperimentalEnabled()
		assert.True(t, got, "expected true when not configured")
	})

	t.Run("returns configured enabled value", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte("experimental:\n  enabled: false\n"), 0644)
		assert.NoError(t, err)

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		globalConfig = nil
		err = InitConfig()
		assert.NoError(t, err)

		got := ExperimentalEnabled()
		assert.False(t, got)
	})
}

func TestExperimentalRepo(t *testing.T) {
	t.Run("returns error when experimental repo not set", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte("experimental:\n  enabled: true\n"), 0644)
		assert.NoError(t, err)

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		globalConfig = nil
		err = InitConfig()
		assert.NoError(t, err)

		_, err = ExperimentalRepo()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not configured")
	})

	t.Run("returns repo from config file", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte("experimental:\n  repo: registry.example.com/team/repo\n"), 0644)
		assert.NoError(t, err)

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		globalConfig = nil
		err = InitConfig()
		assert.NoError(t, err)

		got, err := ExperimentalRepo()
		assert.NoError(t, err)
		assert.Equal(t, "registry.example.com/team/repo", got)
	})

	t.Run("allows registry port", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte("experimental:\n  repo: registry.example.com:5000/team/repo\n"), 0644)
		assert.NoError(t, err)

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		globalConfig = nil
		err = InitConfig()
		assert.NoError(t, err)

		got, err := ExperimentalRepo()
		assert.NoError(t, err)
		assert.Equal(t, "registry.example.com:5000/team/repo", got)
	})

	t.Run("rejects digest reference", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte("experimental:\n  repo: registry.example.com/team/repo@sha256:abc\n"), 0644)
		assert.NoError(t, err)

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		globalConfig = nil
		err = InitConfig()
		assert.NoError(t, err)

		_, err = ExperimentalRepo()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "digest")
	})

	t.Run("rejects tagged repository", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte("experimental:\n  repo: registry.example.com/team/repo:latest\n"), 0644)
		assert.NoError(t, err)

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		globalConfig = nil
		err = InitConfig()
		assert.NoError(t, err)

		_, err = ExperimentalRepo()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tag")
	})
}
