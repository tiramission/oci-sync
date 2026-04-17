package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tiramission/oci-sync/internal/config"
)

func newShortcutCmd(name string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   name,
		Short: fmt.Sprintf("Commands for shortcut %q (default repo: from config)", name),
		Long: fmt.Sprintf(`Run convenience commands for shortcut %q that resolve the repository from
the config file (shortcuts.%s.repo) and only require a tag flag for the remote reference.`, name, name),
	}

	cmd.AddCommand(newShortcutPushCmd(name))
	cmd.AddCommand(newShortcutPullCmd(name))
	cmd.AddCommand(newShortcutListCmd(name))
	cmd.AddCommand(newShortcutDeleteCmd(name))
	return cmd
}

func newShortcutPushCmd(name string) *cobra.Command {
	var local, tag, passphrase string

	cmd := &cobra.Command{
		Use:   "push [flags]",
		Short: fmt.Sprintf("Push to the %q shortcut repository", name),
		Long: fmt.Sprintf(`Push local files or directories to the configured shortcut repository.
Only --tag is required for the remote side. (Set shortcuts.%s.repo in config file)`, name),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShortcutPush(cmd.Context(), name, local, tag, passphrase)
		},
	}

	cmd.Flags().StringVarP(&local, "local", "l", "", "local file or directory path")
	cmd.Flags().StringVar(&tag, "tag", "", fmt.Sprintf("artifact tag for the %q shortcut repository", name))
	cmd.Flags().StringVar(&passphrase, "passphrase", "", "passphrase for encryption (leave empty for no encryption)")
	cmd.MarkFlagRequired("local")
	cmd.MarkFlagRequired("tag")
	return cmd
}

func newShortcutPullCmd(name string) *cobra.Command {
	var local, tag, passphrase string

	cmd := &cobra.Command{
		Use:   "pull [flags]",
		Short: fmt.Sprintf("Pull from the %q shortcut repository", name),
		Long: fmt.Sprintf(`Pull files or directories from the configured shortcut repository.
Only --tag is required for the remote side. (Set shortcuts.%s.repo in config file)`, name),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShortcutPull(cmd.Context(), name, tag, local, passphrase)
		},
	}

	cmd.Flags().StringVarP(&local, "local", "l", "", "local destination directory")
	cmd.Flags().StringVar(&tag, "tag", "", fmt.Sprintf("artifact tag for the %q shortcut repository", name))
	cmd.Flags().StringVar(&passphrase, "passphrase", "", "passphrase for decryption (required if content is encrypted)")
	cmd.MarkFlagRequired("local")
	cmd.MarkFlagRequired("tag")
	return cmd
}

func newShortcutListCmd(name string) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "list",
		Short: fmt.Sprintf("List artifacts in the %q shortcut repository", name),
		Long: fmt.Sprintf(`List artifacts in the configured shortcut repository.
This command resolves the repository from config and lists all tags. (Set shortcuts.%s.repo in config file)`, name),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShortcutList(cmd.Context(), name, format)
		},
	}

	cmd.Flags().StringVarP(&format, "format", "f", "table", "output format (table, json, yaml)")
	return cmd
}

func newShortcutDeleteCmd(name string) *cobra.Command {
	var tag string

	cmd := &cobra.Command{
		Use:   "delete [flags]",
		Short: fmt.Sprintf("Delete an artifact in the %q shortcut repository", name),
		Long: fmt.Sprintf(`Delete an artifact in the configured shortcut repository.
Only --tag is required for the remote side. (Set shortcuts.%s.repo in config file)`, name),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShortcutDelete(cmd.Context(), name, tag)
		},
	}

	cmd.Flags().StringVar(&tag, "tag", "", fmt.Sprintf("artifact tag for the %q shortcut repository", name))
	cmd.MarkFlagRequired("tag")
	return cmd
}

func buildShortcutRemoteRef(name, tag string) (string, error) {
	repo, err := config.GetShortcutRepo(name)
	if err != nil {
		return "", err
	}

	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "", fmt.Errorf("tag cannot be empty for shortcut %q", name)
	}

	return repo + ":" + tag, nil
}

func runShortcutPush(ctx context.Context, name, localPath, tag, passphrase string) error {
	remotePath, err := buildShortcutRemoteRef(name, tag)
	if err != nil {
		return err
	}
	return runPush(ctx, localPath, remotePath, passphrase)
}

func runShortcutPull(ctx context.Context, name, tag, localPath, passphrase string) error {
	remotePath, err := buildShortcutRemoteRef(name, tag)
	if err != nil {
		return err
	}
	return runPull(ctx, remotePath, localPath, passphrase)
}

func runShortcutList(ctx context.Context, name, format string) error {
	repo, err := config.GetShortcutRepo(name)
	if err != nil {
		return err
	}
	return runList(ctx, repo, format)
}

func runShortcutDelete(ctx context.Context, name, tag string) error {
	remotePath, err := buildShortcutRemoteRef(name, tag)
	if err != nil {
		return err
	}
	return runDelete(ctx, remotePath)
}
