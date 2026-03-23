package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

const experimentalRepoEnv = "OCI_SYNC_EXPERIMENTAL_REPO"

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
		Long: fmt.Sprintf(`Push local files or directories to the repository configured in %s.
Only --tag is required for the remote side.`, experimentalRepoEnv),
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
		Long: fmt.Sprintf(`Pull files or directories from the repository configured in %s.
Only --tag is required for the remote side.`, experimentalRepoEnv),
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
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List artifacts in the experimental repository configured by environment variable",
		Long: fmt.Sprintf(`List artifacts in the repository configured in %s.
This command resolves the repository from the environment variable and lists all tags.`, experimentalRepoEnv),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExperimentalList(cmd.Context())
		},
	}

	return cmd
}

func newExperimentalDeleteCmd() *cobra.Command {
	var tag string

	cmd := &cobra.Command{
		Use:   "delete [flags]",
		Short: "Delete an artifact in the experimental repository configured by environment variable",
		Long: fmt.Sprintf(`Delete an artifact in the repository configured in %s.
Only --tag is required for the remote side.`, experimentalRepoEnv),
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
	repo := strings.TrimSpace(os.Getenv(experimentalRepoEnv))
	if repo == "" {
		return "", fmt.Errorf("environment variable %s is required", experimentalRepoEnv)
	}
	if strings.Contains(repo, "@") {
		return "", fmt.Errorf("environment variable %s must contain a repository, not a digest reference", experimentalRepoEnv)
	}

	lastColon := strings.LastIndex(repo, ":")
	lastSlash := strings.LastIndex(repo, "/")
	if lastColon > lastSlash {
		return "", fmt.Errorf("environment variable %s must not include a tag", experimentalRepoEnv)
	}

	return repo, nil
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

func runExperimentalList(ctx context.Context) error {
	repo, err := experimentalRepo()
	if err != nil {
		return err
	}
	return runList(ctx, repo)
}

func runExperimentalDelete(ctx context.Context, tag string) error {
	remotePath, err := buildExperimentalRemoteRef(tag)
	if err != nil {
		return err
	}
	return runDelete(ctx, remotePath)
}
