package cmd

import (
	"context"
	"fmt"

	"charm.land/log/v2"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/tiramission/oci-sync/internal/oci"
)

func newListCmd() *cobra.Command {
	var remote string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List oci-sync artifacts in an OCI registry repository",
		Long: `List all valid artifacts previously pushed by oci-sync in the specified repository.

Example:
  oci-sync list --remote registry-1.docker.io/myuser/myrepo`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd.Context(), remote)
		},
	}

	cmd.Flags().StringVarP(&remote, "remote", "r", "", "remote OCI registry reference (format: <registry>/<repository> or <registry>)")
	cmd.MarkFlagRequired("remote")

	return cmd
}

func runList(ctx context.Context, repoPath string) error {
	log.Info("Fetching tags from registry...", "repo", repoPath)
	artifacts, err := oci.List(ctx, repoPath)
	if err != nil {
		return fmt.Errorf("list failed: %w", err)
	}

	if len(artifacts) == 0 {
		log.Info("No oci-sync artifacts found in repository", "repo", repoPath)
		return nil
	}

	fmt.Println()
	fmt.Printf("  Repository: %s\n\n", repoPath)

	data := pterm.TableData{
		{"REPO", "TAG", "ENCRYPTED", "VERSION", "DIGEST"},
	}

	for _, a := range artifacts {
		encStr := "yes"
		if !a.Encrypted {
			encStr = "no"
		}
		digestShort := a.Digest
		if len(digestShort) > 18 {
			digestShort = digestShort[:18] + "..."
		}
		data = append(data, []string{a.Repo, a.Tag, encStr, a.Version, digestShort})
	}

	output, _ := pterm.DefaultTable.
		WithHasHeader(true).
		WithData(data).
		WithBoxed(true).
		WithSeparator(" │ ").
		Srender()
	fmt.Print(output)

	pterm.Println()
	fmt.Printf("  Total: %d artifact(s)\n", len(artifacts))

	return nil
}
