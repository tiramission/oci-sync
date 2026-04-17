package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tiramission/oci-sync/internal/config"
)

func TestBuildShortcutRemoteRef(t *testing.T) {
	t.Run("builds ref from repo and tag", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte(`shortcuts:
  x:
    repo: registry.example.com/team/repo
`), 0644)
		if err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		config.InitConfig()

		got, err := buildShortcutRemoteRef("x", "v1")
		if err != nil {
			t.Fatalf("buildShortcutRemoteRef returned error: %v", err)
		}
		if got != "registry.example.com/team/repo:v1" {
			t.Fatalf("unexpected ref: %s", got)
		}
	})

	t.Run("allows registry port", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte(`shortcuts:
  x:
    repo: registry.example.com:5000/team/repo
`), 0644)
		if err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		config.InitConfig()

		got, err := buildShortcutRemoteRef("x", "v2")
		if err != nil {
			t.Fatalf("buildShortcutRemoteRef returned error: %v", err)
		}
		if got != "registry.example.com:5000/team/repo:v2" {
			t.Fatalf("unexpected ref: %s", got)
		}
	})

	t.Run("rejects empty tag", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte(`shortcuts:
  x:
    repo: registry.example.com/team/repo
`), 0644)
		if err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		config.InitConfig()

		if _, err := buildShortcutRemoteRef("x", ""); err == nil {
			t.Fatal("expected error for empty tag")
		}
	})

	t.Run("rejects tagged repository in config", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte(`shortcuts:
  x:
    repo: registry.example.com/team/repo:latest
`), 0644)
		if err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		config.InitConfig()

		if _, err := buildShortcutRemoteRef("x", "v1"); err == nil {
			t.Fatal("expected error for tagged repository in config")
		}
	})
}

func TestShortcutRepo(t *testing.T) {
	t.Run("returns repository from config file", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte(`shortcuts:
  x:
    repo: registry.example.com/team/repo
`), 0644)
		if err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		config.InitConfig()

		got, err := config.GetShortcutRepo("x")
		if err != nil {
			t.Fatalf("GetShortcutRepo returned error: %v", err)
		}
		if got != "registry.example.com/team/repo" {
			t.Fatalf("unexpected repo: %s", got)
		}
	})

	t.Run("rejects digest reference in config", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte(`shortcuts:
  x:
    repo: registry.example.com/team/repo@sha256:abc
`), 0644)
		if err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		config.InitConfig()

		if _, err := config.GetShortcutRepo("x"); err == nil {
			t.Fatal("expected error for digest reference")
		}
	})

	t.Run("rejects empty repo", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte(`shortcuts:
  x:
    repo: ""
`), 0644)
		if err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		config.InitConfig()

		if _, err := config.GetShortcutRepo("x"); err == nil {
			t.Fatal("expected error for empty repo")
		}
	})
}
