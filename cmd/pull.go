package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"charm.land/log/v2"
	"github.com/spf13/cobra"
	"github.com/tiramission/oci-sync/internal/archive"
	"github.com/tiramission/oci-sync/internal/cache"
	"github.com/tiramission/oci-sync/internal/oci"
)

func newPullCmd() *cobra.Command {
	var local, remote, passphrase string

	cmd := &cobra.Command{
		Use:   "pull [flags]",
		Short: "Pull files or directories from an OCI registry to local path",
		Long: `Pull an artifact from an OCI-compatible image registry, optionally decrypt,
and unpack to a local directory.

remote format: <registry>/<repository>:<tag>
Example: registry-1.docker.io/myuser/myrepo:latest`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPull(cmd.Context(), remote, local, passphrase, nil)
		},
	}

	cmd.Flags().StringVarP(&remote, "remote", "r", "", "remote OCI registry reference (format: <registry>/<repository>:<tag>)")
	cmd.Flags().StringVarP(&local, "local", "l", "", "local destination directory")
	cmd.Flags().StringVar(&passphrase, "passphrase", "", "passphrase for decryption (required if content is encrypted)")
	cmd.MarkFlagRequired("remote")
	cmd.MarkFlagRequired("local")
	return cmd
}

// PullStage describes the current stage of a pull operation. For the
// download phase, Done/Total carry byte counts; other phases are
// indeterminate (Total == 0).
type PullStage struct {
	Phase string // "check", "download", "decrypt", "unpack"
	Done  int64
	Total int64
}

func runPull(ctx context.Context, remotePath, localPath, passphrase string, onProgress func(PullStage)) error {
	report := func(s PullStage) {
		if onProgress != nil {
			onProgress(s)
		}
	}

	log.Info("Pulling from registry...", "ref", remotePath)

	// Check encryption status before downloading the full content
	report(PullStage{Phase: "check"})
	encrypted, err := oci.IsEncrypted(ctx, remotePath)
	if err != nil {
		return fmt.Errorf("failed to check encryption status: %w", err)
	}

	if encrypted && passphrase == "" {
		return fmt.Errorf("content is encrypted, please provide a decryption key via --passphrase")
	}
	if !encrypted && passphrase != "" {
		log.Warn("content is not encrypted, ignoring --passphrase flag")
	}

	// The layer is staged on disk rather than held in memory: an artifact
	// can be far larger than RAM, and the next stages (decrypt, gunzip,
	// untar) all read it as a stream anyway.
	staged, err := stageTempFile("oci-sync-pull")
	if err != nil {
		return err
	}
	defer os.Remove(staged)

	report(PullStage{Phase: "download"})
	if err := pullToStaged(ctx, staged, remotePath, report); err != nil {
		return fmt.Errorf("pull failed: %w", err)
	}
	size, err := fileSize(staged)
	if err != nil {
		return err
	}
	log.Info("Pull complete", "size", formatBytes(size), "encrypted", encrypted)

	// Unpack reads the staged file as a stream, decrypting on the way
	// through, so the plaintext archive never exists in memory either.
	report(PullStage{Phase: "decrypt", Total: size})
	reader, err := openDecrypted(staged, passphrase, encrypted)
	if err != nil {
		return err
	}
	defer reader.Close()

	// Ensure destination directory exists
	if err := os.MkdirAll(localPath, 0o755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	log.Info("Unpacking files...", "dest", localPath)
	report(PullStage{Phase: "unpack"})
	if err := archive.Unpack(reader, localPath); err != nil {
		return fmt.Errorf("unpack failed: %w", err)
	}

	log.Info("Pull successful ✓", "dest", localPath)

	recordActivity(cache.Activity{
		Type:      cache.ActivityPull,
		Timestamp: time.Now(),
		RemoteRef: remotePath,
		LocalPath: localPath,
		Success:   true,
	})

	return nil
}
