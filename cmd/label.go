package cmd

import (
	"context"
	"fmt"
	"strings"

	"charm.land/log/v2"
	"github.com/spf13/cobra"
	"github.com/tiramission/oci-sync/internal/oci"
)

func newLabelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "label",
		Short: "Manage labels on OCI artifacts",
	}
	cmd.AddCommand(newLabelSetCmd())
	cmd.AddCommand(newLabelUnsetCmd())
	return cmd
}

func newLabelSetCmd() *cobra.Command {
	var remote string

	cmd := &cobra.Command{
		Use:   "set [flags]",
		Short: "Set or update labels on an OCI artifact",
		Long: `Set or update one or more labels on an existing OCI artifact.
Labels are stored in the manifest annotations.

Examples:
  oci-sync label set myrepo:latest key1=value1 key2=value2
  oci-sync label set registry.example.com/myrepo:latest app=myapp version=1.0`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLabelSet(cmd.Context(), remote, args)
		},
	}

	cmd.Flags().StringVarP(&remote, "remote", "r", "", "remote OCI registry reference")
	cmd.MarkFlagRequired("remote")
	return cmd
}

func runLabelSet(ctx context.Context, remote string, labels []string) error {
	updates := make(map[string]string)
	for _, l := range labels {
		parts := strings.SplitN(l, "=", 2)
		if len(parts) < 2 {
			return fmt.Errorf("invalid label format %q, expected key=value", l)
		}
		updates[parts[0]] = parts[1]
	}

	log.Info("Updating labels...", "ref", remote)
	if err := oci.UpdateAnnotations(ctx, remote, updates, nil); err != nil {
		return fmt.Errorf("set labels failed: %w", err)
	}

	log.Info("Labels updated ✓", "ref", remote)
	return nil
}

func newLabelUnsetCmd() *cobra.Command {
	var remote string

	cmd := &cobra.Command{
		Use:   "unset [flags]",
		Short: "Remove labels from an OCI artifact",
		Long: `Remove one or more labels from an existing OCI artifact.

Examples:
  oci-sync label unset myrepo:latest key1 key2
  oci-sync label unset registry.example.com/myrepo:latest app version`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLabelUnset(cmd.Context(), remote, args)
		},
	}

	cmd.Flags().StringVarP(&remote, "remote", "r", "", "remote OCI registry reference")
	cmd.MarkFlagRequired("remote")
	return cmd
}

func runLabelUnset(ctx context.Context, remote string, keys []string) error {
	if len(keys) == 0 {
		return fmt.Errorf("at least one label key required")
	}

	log.Info("Removing labels...", "ref", remote, "keys", keys)
	if err := oci.UpdateAnnotations(ctx, remote, nil, keys); err != nil {
		return fmt.Errorf("unset labels failed: %w", err)
	}

	log.Info("Labels removed ✓", "ref", remote)
	return nil
}
