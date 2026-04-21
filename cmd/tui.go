package cmd

import (
	"github.com/spf13/cobra"
	"github.com/tiramission/oci-sync/internal/tui"
)

func newTuiCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "[EXPERIMENTAL] Launch interactive terminal UI",
		Long: `Launch an interactive terminal user interface for managing artifacts.

The TUI provides a convenient way to:
- Browse configured shortcuts
- View artifacts in each shortcut repository
- Upload new artifacts
- Download artifacts
- Delete artifacts`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return tui.Run(cmd.Context())
		},
	}

	return cmd
}
