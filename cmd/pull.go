package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/tiramission/oci-sync/internal/archive"
	"github.com/tiramission/oci-sync/internal/crypto"
	"github.com/tiramission/oci-sync/internal/oci"
)

func newPullCmd() *cobra.Command {
	var passphrase string

	cmd := &cobra.Command{
		Use:   "pull <remote_path> <local_path>",
		Short: "Pull files or directories from an OCI registry to local path",
		Long: `Pull an artifact from an OCI-compatible image registry, optionally decrypt,
and unpack to a local directory.

remote_path format: <registry>/<repository>:<tag>
Example: registry-1.docker.io/myuser/myrepo:latest`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			remotePath := args[0]
			localPath := args[1]
			return runPull(cmd.Context(), remotePath, localPath, passphrase)
		},
	}

	cmd.Flags().StringVar(&passphrase, "passphrase", "", "passphrase for decryption (required if content is encrypted)")
	return cmd
}

func runPull(ctx context.Context, remotePath, localPath, passphrase string) error {
	log.Info("Pulling from registry...", "ref", remotePath)
	result, err := oci.Pull(ctx, remotePath)
	if err != nil {
		return fmt.Errorf("pull failed: %w", err)
	}
	log.Info("Pull complete", "size", formatBytes(len(result.Data)), "encrypted", result.Encrypted)

	data := result.Data

	if result.Encrypted {
		if passphrase == "" {
			return fmt.Errorf("content is encrypted, please provide a decryption key via --passphrase")
		}
		log.Info("Decrypting...")
		data, err = crypto.Decrypt(data, passphrase)
		if err != nil {
			return fmt.Errorf("decryption failed: %w", err)
		}
		log.Info("Decryption complete")
	} else if passphrase != "" {
		log.Warn("content is not encrypted, ignoring --passphrase flag")
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(localPath, 0o755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	log.Info("Unpacking files...", "dest", localPath)
	if err := archive.Unpack(data, localPath); err != nil {
		return fmt.Errorf("unpack failed: %w", err)
	}

	log.Info("Pull successful ✓", "dest", localPath)
	return nil
}
