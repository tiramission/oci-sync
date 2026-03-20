package cmd

import (
	"context"
	"fmt"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/tiramission/oci-sync/internal/archive"
	"github.com/tiramission/oci-sync/internal/crypto"
	"github.com/tiramission/oci-sync/internal/oci"
)

func newPushCmd() *cobra.Command {
	var passphrase string

	cmd := &cobra.Command{
		Use:   "push <local_path> <remote_path>",
		Short: "Push local files or directories to an OCI registry",
		Long: `Pack local files or directories (tar.gz), optionally encrypt (AES-256-GCM),
and push to an OCI-compatible image registry.

remote_path format: <registry>/<repository>:<tag>
Example: registry-1.docker.io/myuser/myrepo:latest`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			localPath := args[0]
			remotePath := args[1]
			return runPush(cmd.Context(), localPath, remotePath, passphrase)
		},
	}

	cmd.Flags().StringVar(&passphrase, "passphrase", "", "passphrase for encryption (leave empty for no encryption)")
	return cmd
}

func runPush(ctx context.Context, localPath, remotePath, passphrase string) error {
	log.Info("Packing files...", "path", localPath)
	data, err := archive.Pack(localPath)
	if err != nil {
		return fmt.Errorf("pack failed: %w", err)
	}
	log.Info("Pack complete", "size", formatBytes(len(data)))

	encrypted := passphrase != ""
	if encrypted {
		log.Info("Encrypting...")
		data, err = crypto.Encrypt(data, passphrase)
		if err != nil {
			return fmt.Errorf("encryption failed: %w", err)
		}
		log.Info("Encryption complete", "size", formatBytes(len(data)))
	}

	log.Info("Pushing to registry...", "ref", remotePath)
	if err := oci.Push(ctx, data, remotePath, encrypted); err != nil {
		return fmt.Errorf("push failed: %w", err)
	}

	log.Info("Push successful ✓", "ref", remotePath)
	return nil
}
