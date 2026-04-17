package cmd

import (
	"fmt"
	"os"

	"charm.land/log/v2"
	"github.com/spf13/cobra"

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
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// Initialize configuration
	config.InitConfig()

	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Omit informational output")

	rootCmd.AddCommand(newPushCmd())
	rootCmd.AddCommand(newPullCmd())
	rootCmd.AddCommand(newDeleteCmd())
	rootCmd.AddCommand(newListCmd())

	// Add experimental commands only if enabled
	if config.ExperimentalEnabled() {
		rootCmd.AddCommand(newExperimentalCmd())
	}
}
