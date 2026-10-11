package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/tiramission/oci-sync/internal/config"
	"github.com/tiramission/oci-sync/internal/xdg"
	"gopkg.in/yaml.v3"
)

func newAliasCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alias",
		Short: "Manage and list configured shortcuts",
	}
	cmd.AddCommand(newAliasListCmd())
	cmd.AddCommand(newAliasAddCmd())
	cmd.AddCommand(newAliasRemoveCmd())
	return cmd
}

func newAliasListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all configured shortcuts",
		Long:  `List all shortcuts configured in the config file with their repository paths.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAliasList(cmd.Context())
		},
	}
	return cmd
}

func runAliasList(ctx context.Context) error {
	shortcuts := config.GetAllShortcuts()
	if len(shortcuts) == 0 {
		fmt.Println("No shortcuts configured")
		return nil
	}

	fmt.Println()
	if cfgFile := config.ConfigFileUsed(); cfgFile != "" {
		fmt.Printf("  Config: %s\n\n", cfgFile)
	}

	data := pterm.TableData{
		{"NAME", "REPO"},
	}

	for _, s := range shortcuts {
		data = append(data, []string{s.Name, s.Repo})
	}

	output, _ := pterm.DefaultTable.
		WithHasHeader(true).
		WithData(data).
		WithBoxed(true).
		WithSeparator(" │ ").
		Srender()
	fmt.Print(output)

	pterm.Println()
	fmt.Printf("  Total: %d shortcut(s)\n", len(shortcuts))
	return nil
}

func newAliasAddCmd() *cobra.Command {
	var repo string

	cmd := &cobra.Command{
		Use:   "add [flags]",
		Short: "Add a new shortcut",
		Long:  `Add a new shortcut to the config file.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("requires exactly one argument: <name>")
			}
			return runAliasAdd(cmd.Context(), args[0], repo)
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "repository path (e.g., registry.example.com/repo)")
	cmd.MarkFlagRequired("repo")
	return cmd
}

func runAliasAdd(ctx context.Context, name, repo string) error {
	if repo == "" {
		return fmt.Errorf("--repo is required")
	}

	if strings.Contains(repo, "@") {
		return fmt.Errorf("repository must not be a digest reference (contains '@')")
	}
	lastColon := strings.LastIndex(repo, ":")
	lastSlash := strings.LastIndex(repo, "/")
	if lastColon > lastSlash {
		return fmt.Errorf("repository must not include a tag (found ':' after last '/')")
	}

	cfgPath, err := getConfigPath()
	if err != nil {
		return err
	}

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		return err
	}

	if cfg.Shortcuts == nil {
		cfg.Shortcuts = make(map[string]config.Shortcut)
	}
	cfg.Shortcuts[name] = config.Shortcut{Repo: repo}

	if err := saveConfig(cfgPath, cfg); err != nil {
		return fmt.Errorf("cannot write config file %s: %w", cfgPath, err)
	}

	fmt.Printf("Shortcut %q added with repo %q\n", name, repo)
	fmt.Printf("  Config: %s\n", cfgPath)
	return nil
}

func newAliasRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove [flags]",
		Short: "Remove a shortcut",
		Long:  `Remove a shortcut from the config file.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("requires exactly one argument: <name>")
			}
			return runAliasRemove(cmd.Context(), args[0])
		},
	}
	return cmd
}

func runAliasRemove(ctx context.Context, name string) error {
	if !config.HasShortcut(name) {
		return fmt.Errorf("shortcut %q not found", name)
	}

	cfgPath, err := getConfigPath()
	if err != nil {
		return err
	}

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		return err
	}

	delete(cfg.Shortcuts, name)

	if err := saveConfig(cfgPath, cfg); err != nil {
		return fmt.Errorf("cannot write config file %s: %w", cfgPath, err)
	}

	fmt.Printf("Shortcut %q removed\n", name)
	fmt.Printf("  Config: %s\n", cfgPath)
	return nil
}

// getConfigPath returns the config file to write shortcuts into. When a
// config file is already in use it is rewritten in place, so an existing
// project-local ./oci-sync.yaml stays authoritative; otherwise the XDG
// location is used. This mirrors the lookup order in config.InitConfig —
// writing anywhere else would silently fork the config in two.
func getConfigPath() (string, error) {
	if used := config.ConfigFileUsed(); used != "" {
		return used, nil
	}
	configDir := filepath.Join(xdg.ConfigDir(), "oci-sync")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}
	return filepath.Join(configDir, "oci-sync.yaml"), nil
}

func loadConfig(path string) (*config.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &config.Config{}, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

func saveConfig(path string, cfg *config.Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
