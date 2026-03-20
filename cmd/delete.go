package cmd

import (
	"context"
	"fmt"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/tiramission/oci-sync/internal/oci"
)

func newDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <remote_path>",
		Short: "Delete an artifact from an OCI registry",
		Long: `Delete a previously pushed artifact from an OCI-compatible image registry.

remote_path format: <registry>/<repository>:<tag>
Example: registry-1.docker.io/myuser/myrepo:latest`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			remotePath := args[0]
			return runDelete(cmd.Context(), remotePath)
		},
	}

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
