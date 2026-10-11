package cmd

import (
	"fmt"
	"os"

	"charm.land/log/v2"
	"github.com/spf13/cobra"

	"github.com/tiramission/oci-sync/internal/cache"
	"github.com/tiramission/oci-sync/internal/config"
	"github.com/tiramission/oci-sync/internal/version"
)

var quiet bool

var rootCmd = &cobra.Command{
	Use:   "oci-sync",
	Short: "Sync local files to OCI-compatible image registries",
	Long: `oci-sync packs, compresses, and optionally encrypts local files or directories,
and pushes them as OCI artifacts to OCI-compatible image registries.
Authentication uses Docker credential store (compatible with docker login).`,
	Version: version.Version,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Configure logger based on flags
		if quiet {
			log.SetLevel(log.ErrorLevel)
		} else {
			log.SetLevel(log.InfoLevel)
		}
		log.SetTimeFormat("15:04:05")
	},
}

// Execute runs the root command.
func Execute() {
	// A config that fails to parse must be reported: silently falling back to
	// an empty config looks like "my credentials vanished". Cache failures
	// are non-fatal — they only cost the activity history.
	if err := config.InitConfig(); err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(1)
	}
	if err := cache.InitCache(); err != nil {
		fmt.Fprintln(os.Stderr, "warning: activity cache unavailable:", err)
	}
	registerShortcuts()

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Omit informational output")

	rootCmd.AddCommand(newPushCmd())
	rootCmd.AddCommand(newPullCmd())
	rootCmd.AddCommand(newDeleteCmd())
	rootCmd.AddCommand(newListCmd())
	rootCmd.AddCommand(newLabelCmd())
	rootCmd.AddCommand(newAliasCmd())
	rootCmd.AddCommand(newRecentCmd())
	rootCmd.AddCommand(newTuiCmd())
}

// registerShortcuts attaches one command tree per configured shortcut. It
// runs after the config is loaded, so shortcuts added by `alias add` are
// visible to this run.
func registerShortcuts() {
	for _, name := range config.ShortcutNames() {
		rootCmd.AddCommand(newShortcutCmd(name))
	}
}
