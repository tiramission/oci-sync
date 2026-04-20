package cmd

import (
	"github.com/spf13/cobra"
	"github.com/tiramission/oci-sync/internal/tui"
)

func newTuiCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch interactive terminal UI for managing artifacts",
		Long: `Launch an interactive terminal user interface for managing artifacts and shortcuts.

The TUI provides a convenient way to:
- Browse configured shortcuts
- View artifacts in each shortcut repository
- Upload new artifacts
- Download artifacts
- Delete artifacts
- Edit artifact labels`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return tui.Run(cmd.Context())
		},
	}

	return cmd
}
