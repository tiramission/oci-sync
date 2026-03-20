package cmd

import (
	"fmt"
	"os"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "oci-sync",
	Short: "Sync local files to OCI-compatible image registries",
	Long: `oci-sync packs, compresses, and optionally encrypts local files or directories,
and pushes them as OCI artifacts to OCI-compatible image registries.
Authentication uses Docker credential store (compatible with docker login).`,
	Version: "0.1.0",
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// Configure logger
	log.SetLevel(log.InfoLevel)
	log.SetTimeFormat("15:04:05")

	rootCmd.AddCommand(newPushCmd())
	rootCmd.AddCommand(newPullCmd())
	rootCmd.AddCommand(newDeleteCmd())
	rootCmd.AddCommand(newListCmd())
}
