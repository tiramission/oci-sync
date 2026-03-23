package cmd

import (
	"context"
	"fmt"

	"charm.land/log/v2"
	"github.com/spf13/cobra"
	"github.com/tiramission/oci-sync/internal/oci"
)

func newDeleteCmd() *cobra.Command {
	var remote string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete an artifact from an OCI registry",
		Long: `Delete a previously pushed artifact from an OCI-compatible image registry.

Example:
  oci-sync delete --remote registry-1.docker.io/myuser/myrepo:latest`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDelete(cmd.Context(), remote)
		},
	}

	cmd.Flags().StringVarP(&remote, "remote", "r", "", "remote OCI registry reference (format: <registry>/<repository>:<tag>)")
	cmd.MarkFlagRequired("remote")

	return cmd
}

func runDelete(ctx context.Context, remotePath string) error {
	log.Info("Deleting from registry...", "ref", remotePath)
	if err := oci.Delete(ctx, remotePath); err != nil {
		return fmt.Errorf("delete failed: %w", err)
	}
	log.Info("Delete successful ✓", "ref", remotePath)
	return nil
}
