package cmd

import (
	"github.com/spf13/cobra"
)

func newGuiCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gui",
		Short: "Launch the desktop GUI",
		Long: `Launch the native desktop GUI.

Browse artifacts across your configured shortcuts, pull them to a local
directory and delete remote tags, without leaving the keyboard. The GUI runs
inside the same binary as the CLI and closes when its window closes.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGui(cmd.Context())
		},
	}
	return cmd
}
