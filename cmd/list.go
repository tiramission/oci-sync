package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"charm.land/log/v2"
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

	// Print tab-separated table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "REPO\tTAG\tENCRYPTED\tVERSION\tDIGEST")
	for _, a := range artifacts {
		encStr := "yes"
		if !a.Encrypted {
			encStr = "no"
		}
		// truncate digest for display
		digestShort := a.Digest
		if len(digestShort) > 15 {
			digestShort = digestShort[:15] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", a.Repo, a.Tag, encStr, a.Version, digestShort)
	}
	w.Flush()

	return nil
}
