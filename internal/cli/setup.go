package cli

import (
	"github.com/roen/nodeman/internal/shim"
	"github.com/spf13/cobra"
)

func newSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Create shim binaries and configure PATH",
		Long: `Creates shim binaries (node, npm, npx, corepack) in ~/.nodeman/shims/.

These shims forward commands to the currently active Node.js version.
After running setup, add ~/.nodeman/shims to your PATH.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return shim.Setup()
		},
	}
}
