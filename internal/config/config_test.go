package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShortcutNames(t *testing.T) {
	t.Run("returns nil when shortcuts not configured", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte("auths: {}\n"), 0644)
		assert.NoError(t, err)

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		globalConfig = nil
		err = InitConfig()
		assert.NoError(t, err)

		got := ShortcutNames()
		assert.Nil(t, got)
	})

	t.Run("returns shortcut names", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte(`shortcuts:
  x:
    repo: registry.example.com/team/repo
  y:
    repo: registry.example.com/other/repo
`), 0644)
		assert.NoError(t, err)

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		globalConfig = nil
		err = InitConfig()
		assert.NoError(t, err)

		got := ShortcutNames()
		assert.Len(t, got, 2)
		assert.Contains(t, got, "x")
		assert.Contains(t, got, "y")
	})
}

func TestGetShortcutRepo(t *testing.T) {
	t.Run("returns error when config not initialized", func(t *testing.T) {
		globalConfig = nil
		_, err := GetShortcutRepo("x")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not initialized")
	})

	t.Run("returns error when shortcut not found", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte("auths: {}\n"), 0644)
		assert.NoError(t, err)

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		globalConfig = nil
		err = InitConfig()
		assert.NoError(t, err)

		_, err = GetShortcutRepo("nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("returns repo from config", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte(`shortcuts:
  x:
    repo: registry.example.com/team/repo
`), 0644)
		assert.NoError(t, err)

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		globalConfig = nil
		err = InitConfig()
		assert.NoError(t, err)

		got, err := GetShortcutRepo("x")
		assert.NoError(t, err)
		assert.Equal(t, "registry.example.com/team/repo", got)
	})

	t.Run("returns error when repo is empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte(`shortcuts:
  x:
    repo: ""
`), 0644)
		assert.NoError(t, err)

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		globalConfig = nil
		err = InitConfig()
		assert.NoError(t, err)

		_, err = GetShortcutRepo("x")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty")
	})

	t.Run("returns error when repo contains digest", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte(`shortcuts:
  x:
    repo: registry.example.com/team/repo@sha256:abc
`), 0644)
		assert.NoError(t, err)

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		globalConfig = nil
		err = InitConfig()
		assert.NoError(t, err)

		_, err = GetShortcutRepo("x")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "digest")
	})

	t.Run("returns error when repo contains tag", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte(`shortcuts:
  x:
    repo: registry.example.com/team/repo:latest
`), 0644)
		assert.NoError(t, err)

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		globalConfig = nil
		err = InitConfig()
		assert.NoError(t, err)

		_, err = GetShortcutRepo("x")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tag")
	})
}
