package cmd

import (
	"github.com/spf13/cobra"
	"github.com/tiramission/oci-sync/internal/gui"
)

func newGUICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gui",
		Short: "Launch graphical artifact manager",
		Long: `Launch a graphical user interface for managing artifacts in shortcut repositories.

The GUI provides a convenient way to:
- Browse configured shortcuts
- View artifacts in each shortcut repository
- Upload new artifacts
- Download artifacts
- Delete artifacts`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return gui.Run(cmd.Context())
		},
	}

	return cmd
}
