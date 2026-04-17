package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tiramission/oci-sync/internal/config"
)

func TestBuildExperimentalRemoteRef(t *testing.T) {
	t.Run("builds ref from repo and tag", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte("experimental:\n  repo: registry.example.com/team/repo\n"), 0644)
		if err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		config.InitConfig()

		got, err := buildExperimentalRemoteRef("v1")
		if err != nil {
			t.Fatalf("buildExperimentalRemoteRef returned error: %v", err)
		}
		if got != "registry.example.com/team/repo:v1" {
			t.Fatalf("unexpected ref: %s", got)
		}
	})

	t.Run("allows registry port", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte("experimental:\n  repo: registry.example.com:5000/team/repo\n"), 0644)
		if err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		config.InitConfig()

		got, err := buildExperimentalRemoteRef("v2")
		if err != nil {
			t.Fatalf("buildExperimentalRemoteRef returned error: %v", err)
		}
		if got != "registry.example.com:5000/team/repo:v2" {
			t.Fatalf("unexpected ref: %s", got)
		}
	})

	t.Run("rejects empty tag", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte("experimental:\n  repo: registry.example.com/team/repo\n"), 0644)
		if err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		config.InitConfig()

		if _, err := buildExperimentalRemoteRef(""); err == nil {
			t.Fatal("expected error for empty tag")
		}
	})

	t.Run("rejects tagged repository in config", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte("experimental:\n  repo: registry.example.com/team/repo:latest\n"), 0644)
		if err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		config.InitConfig()

		if _, err := buildExperimentalRemoteRef("v1"); err == nil {
			t.Fatal("expected error for tagged repository in config")
		}
	})
}

func TestExperimentalRepo(t *testing.T) {
	t.Run("returns repository from config file", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte("experimental:\n  repo: registry.example.com/team/repo\n"), 0644)
		if err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		config.InitConfig()

		got, err := experimentalRepo()
		if err != nil {
			t.Fatalf("experimentalRepo returned error: %v", err)
		}
		if got != "registry.example.com/team/repo" {
			t.Fatalf("unexpected repo: %s", got)
		}
	})

	t.Run("rejects digest reference in config", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte("experimental:\n  repo: registry.example.com/team/repo@sha256:abc\n"), 0644)
		if err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		config.InitConfig()

		if _, err := experimentalRepo(); err == nil {
			t.Fatal("expected error for digest reference")
		}
	})
}

func TestConfigExperimentalRepo(t *testing.T) {
	t.Run("returns repository from config file", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte("experimental:\n  repo: registry.example.com/team/repo\n"), 0644)
		if err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		config.InitConfig()

		got, err := config.ExperimentalRepo()
		if err != nil {
			t.Fatalf("ExperimentalRepo returned error: %v", err)
		}
		if got != "registry.example.com/team/repo" {
			t.Fatalf("unexpected repo: %s", got)
		}
	})

	t.Run("allows registry port", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte("experimental:\n  repo: registry.example.com:5000/team/repo\n"), 0644)
		if err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		config.InitConfig()

		got, err := config.ExperimentalRepo()
		if err != nil {
			t.Fatalf("ExperimentalRepo returned error: %v", err)
		}
		if got != "registry.example.com:5000/team/repo" {
			t.Fatalf("unexpected repo: %s", got)
		}
	})

	t.Run("rejects digest reference", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte("experimental:\n  repo: registry.example.com/team/repo@sha256:abc\n"), 0644)
		if err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		config.InitConfig()

		if _, err := config.ExperimentalRepo(); err == nil {
			t.Fatal("expected error for digest reference")
		}
	})

	t.Run("rejects tagged repository", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "oci-sync.yaml")
		err := os.WriteFile(configPath, []byte("experimental:\n  repo: registry.example.com/team/repo:latest\n"), 0644)
		if err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		origCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origCwd)

		config.InitConfig()

		if _, err := config.ExperimentalRepo(); err == nil {
			t.Fatal("expected error for tagged repository")
		}
	})
}
