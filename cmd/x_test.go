package cmd

import "testing"

func TestBuildExperimentalRemoteRef(t *testing.T) {
	t.Run("builds ref from repo and tag", func(t *testing.T) {
		t.Setenv(experimentalRepoEnv, "registry.example.com/team/repo")

		got, err := buildExperimentalRemoteRef("v1")
		if err != nil {
			t.Fatalf("buildExperimentalRemoteRef returned error: %v", err)
		}
		if got != "registry.example.com/team/repo:v1" {
			t.Fatalf("unexpected ref: %s", got)
		}
	})

	t.Run("allows registry port", func(t *testing.T) {
		t.Setenv(experimentalRepoEnv, "registry.example.com:5000/team/repo")

		got, err := buildExperimentalRemoteRef("v2")
		if err != nil {
			t.Fatalf("buildExperimentalRemoteRef returned error: %v", err)
		}
		if got != "registry.example.com:5000/team/repo:v2" {
			t.Fatalf("unexpected ref: %s", got)
		}
	})

	t.Run("rejects missing environment variable", func(t *testing.T) {
		t.Setenv(experimentalRepoEnv, "")

		if _, err := buildExperimentalRemoteRef("v1"); err == nil {
			t.Fatal("expected error for missing environment variable")
		}
	})

	t.Run("rejects tagged repository in environment variable", func(t *testing.T) {
		t.Setenv(experimentalRepoEnv, "registry.example.com/team/repo:latest")

		if _, err := buildExperimentalRemoteRef("v1"); err == nil {
			t.Fatal("expected error for tagged environment variable")
		}
	})
}

func TestExperimentalRepo(t *testing.T) {
	t.Run("returns repository from environment variable", func(t *testing.T) {
		t.Setenv(experimentalRepoEnv, "registry.example.com/team/repo")

		got, err := experimentalRepo()
		if err != nil {
			t.Fatalf("experimentalRepo returned error: %v", err)
		}
		if got != "registry.example.com/team/repo" {
			t.Fatalf("unexpected repo: %s", got)
		}
	})

	t.Run("rejects digest reference in environment variable", func(t *testing.T) {
		t.Setenv(experimentalRepoEnv, "registry.example.com/team/repo@sha256:abc")

		if _, err := experimentalRepo(); err == nil {
			t.Fatal("expected error for digest reference")
		}
	})
}
