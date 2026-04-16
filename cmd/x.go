package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tiramission/oci-sync/internal/config"
)

func newExperimentalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "x",
		Short: "Experimental convenience commands",
		Long: `Run experimental convenience commands that resolve the repository from
an environment variable and only require a tag flag for the remote reference.`,
	}

	cmd.AddCommand(newExperimentalPushCmd())
	cmd.AddCommand(newExperimentalPullCmd())
	cmd.AddCommand(newExperimentalListCmd())
	cmd.AddCommand(newExperimentalDeleteCmd())
	return cmd
}

func newExperimentalPushCmd() *cobra.Command {
	var local, tag, passphrase string

	cmd := &cobra.Command{
		Use:   "push [flags]",
		Short: "Push to the experimental repository configured by environment variable",
		Long: `Push local files or directories to the configured repository.
Only --tag is required for the remote side. (Set OCI_SYNC_EXPERIMENTAL_REPO env var or experimental.repo in config file)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExperimentalPush(cmd.Context(), local, tag, passphrase)
		},
	}

	cmd.Flags().StringVarP(&local, "local", "l", "", "local file or directory path")
	cmd.Flags().StringVar(&tag, "tag", "", "artifact tag for the experimental repository")
	cmd.Flags().StringVar(&passphrase, "passphrase", "", "passphrase for encryption (leave empty for no encryption)")
	cmd.MarkFlagRequired("local")
	cmd.MarkFlagRequired("tag")
	return cmd
}

func newExperimentalPullCmd() *cobra.Command {
	var local, tag, passphrase string

	cmd := &cobra.Command{
		Use:   "pull [flags]",
		Short: "Pull from the experimental repository configured by environment variable",
		Long: `Pull files or directories from the configured repository.
Only --tag is required for the remote side. (Set OCI_SYNC_EXPERIMENTAL_REPO env var or experimental.repo in config file)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExperimentalPull(cmd.Context(), tag, local, passphrase)
		},
	}

	cmd.Flags().StringVarP(&local, "local", "l", "", "local destination directory")
	cmd.Flags().StringVar(&tag, "tag", "", "artifact tag for the experimental repository")
	cmd.Flags().StringVar(&passphrase, "passphrase", "", "passphrase for decryption (required if content is encrypted)")
	cmd.MarkFlagRequired("local")
	cmd.MarkFlagRequired("tag")
	return cmd
}

func newExperimentalListCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List artifacts in the experimental repository configured by environment variable",
		Long: `List artifacts in the configured repository.
This command resolves the repository from config and lists all tags. (Set OCI_SYNC_EXPERIMENTAL_REPO env var or experimental.repo in config file)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExperimentalList(cmd.Context(), format)
		},
	}

	cmd.Flags().StringVarP(&format, "format", "f", "table", "output format (table, json, yaml)")
	return cmd
}

func newExperimentalDeleteCmd() *cobra.Command {
	var tag string

	cmd := &cobra.Command{
		Use:   "delete [flags]",
		Short: "Delete an artifact in the experimental repository configured by environment variable",
		Long: `Delete an artifact in the configured repository.
Only --tag is required for the remote side. (Set OCI_SYNC_EXPERIMENTAL_REPO env var or experimental.repo in config file)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExperimentalDelete(cmd.Context(), tag)
		},
	}

	cmd.Flags().StringVar(&tag, "tag", "", "artifact tag for the experimental repository")
	cmd.MarkFlagRequired("tag")
	return cmd
}

func buildExperimentalRemoteRef(tag string) (string, error) {
	repo, err := experimentalRepo()
	if err != nil {
		return "", err
	}

	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "", fmt.Errorf("experimental tag cannot be empty")
	}

	return repo + ":" + tag, nil
}

func experimentalRepo() (string, error) {
	return config.ExperimentalRepo()
}

func runExperimentalPush(ctx context.Context, localPath, tag, passphrase string) error {
	remotePath, err := buildExperimentalRemoteRef(tag)
	if err != nil {
		return err
	}
	return runPush(ctx, localPath, remotePath, passphrase)
}

func runExperimentalPull(ctx context.Context, tag, localPath, passphrase string) error {
	remotePath, err := buildExperimentalRemoteRef(tag)
	if err != nil {
		return err
	}
	return runPull(ctx, remotePath, localPath, passphrase)
}

func runExperimentalList(ctx context.Context, format string) error {
	repo, err := experimentalRepo()
	if err != nil {
		return err
	}
	return runList(ctx, repo, format)
}

func runExperimentalDelete(ctx context.Context, tag string) error {
	remotePath, err := buildExperimentalRemoteRef(tag)
	if err != nil {
		return err
	}
	return runDelete(ctx, remotePath)
}
