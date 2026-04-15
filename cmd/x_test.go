package cmd

import (
	"os"
	"testing"

	"github.com/tiramission/oci-sync/internal/config"
)

func TestBuildExperimentalRemoteRef(t *testing.T) {
	// Save original env and restore after test
	origEnv := os.Getenv("OCI_SYNC_EXPERIMENTAL_REPO")
	t.Cleanup(func() {
		if origEnv != "" {
			os.Setenv("OCI_SYNC_EXPERIMENTAL_REPO", origEnv)
		} else {
			os.Unsetenv("OCI_SYNC_EXPERIMENTAL_REPO")
		}
	})

	t.Run("builds ref from repo and tag", func(t *testing.T) {
		t.Setenv("OCI_SYNC_EXPERIMENTAL_REPO", "registry.example.com/team/repo")

		got, err := buildExperimentalRemoteRef("v1")
		if err != nil {
			t.Fatalf("buildExperimentalRemoteRef returned error: %v", err)
		}
		if got != "registry.example.com/team/repo:v1" {
			t.Fatalf("unexpected ref: %s", got)
		}
	})

	t.Run("allows registry port", func(t *testing.T) {
		t.Setenv("OCI_SYNC_EXPERIMENTAL_REPO", "registry.example.com:5000/team/repo")

		got, err := buildExperimentalRemoteRef("v2")
		if err != nil {
			t.Fatalf("buildExperimentalRemoteRef returned error: %v", err)
		}
		if got != "registry.example.com:5000/team/repo:v2" {
			t.Fatalf("unexpected ref: %s", got)
		}
	})

	t.Run("rejects empty tag", func(t *testing.T) {
		t.Setenv("OCI_SYNC_EXPERIMENTAL_REPO", "registry.example.com/team/repo")

		if _, err := buildExperimentalRemoteRef(""); err == nil {
			t.Fatal("expected error for empty tag")
		}
	})

	t.Run("rejects tagged repository in environment variable", func(t *testing.T) {
		t.Setenv("OCI_SYNC_EXPERIMENTAL_REPO", "registry.example.com/team/repo:latest")

		if _, err := buildExperimentalRemoteRef("v1"); err == nil {
			t.Fatal("expected error for tagged environment variable")
		}
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

	t.Run("returns repository from environment variable", func(t *testing.T) {
		t.Setenv("OCI_SYNC_EXPERIMENTAL_REPO", "registry.example.com/team/repo")

		got, err := experimentalRepo()
		if err != nil {
			t.Fatalf("experimentalRepo returned error: %v", err)
		}
		if got != "registry.example.com/team/repo" {
			t.Fatalf("unexpected repo: %s", got)
		}
	})

	t.Run("rejects digest reference in environment variable", func(t *testing.T) {
		t.Setenv("OCI_SYNC_EXPERIMENTAL_REPO", "registry.example.com/team/repo@sha256:abc")

		if _, err := experimentalRepo(); err == nil {
			t.Fatal("expected error for digest reference")
		}
	})
}

func TestConfigExperimentalRepo(t *testing.T) {
	// Save original env and restore after test
	origEnv := os.Getenv("OCI_SYNC_EXPERIMENTAL_REPO")
	t.Cleanup(func() {
		if origEnv != "" {
			os.Setenv("OCI_SYNC_EXPERIMENTAL_REPO", origEnv)
		} else {
			os.Unsetenv("OCI_SYNC_EXPERIMENTAL_REPO")
		}
	})

	// Initialize config for tests
	config.InitConfig()

	t.Run("returns repository from environment variable", func(t *testing.T) {
		t.Setenv("OCI_SYNC_EXPERIMENTAL_REPO", "registry.example.com/team/repo")

		got, err := config.ExperimentalRepo()
		if err != nil {
			t.Fatalf("ExperimentalRepo returned error: %v", err)
		}
		if got != "registry.example.com/team/repo" {
			t.Fatalf("unexpected repo: %s", got)
		}
	})

	t.Run("allows registry port", func(t *testing.T) {
		t.Setenv("OCI_SYNC_EXPERIMENTAL_REPO", "registry.example.com:5000/team/repo")

		got, err := config.ExperimentalRepo()
		if err != nil {
			t.Fatalf("ExperimentalRepo returned error: %v", err)
		}
		if got != "registry.example.com:5000/team/repo" {
			t.Fatalf("unexpected repo: %s", got)
		}
	})

	t.Run("rejects digest reference", func(t *testing.T) {
		t.Setenv("OCI_SYNC_EXPERIMENTAL_REPO", "registry.example.com/team/repo@sha256:abc")

		if _, err := config.ExperimentalRepo(); err == nil {
			t.Fatal("expected error for digest reference")
		}
	})

	t.Run("rejects tagged repository", func(t *testing.T) {
		t.Setenv("OCI_SYNC_EXPERIMENTAL_REPO", "registry.example.com/team/repo:latest")

		if _, err := config.ExperimentalRepo(); err == nil {
			t.Fatal("expected error for tagged repository")
		}
	})
}
