package cli

import (
	"fmt"

	"github.com/roen/nodeman/internal/shim"
	"github.com/spf13/cobra"
)

func newShimsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shims",
		Short: "Manage nodeman shims",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "sync",
		Short: "Create shims for globally installed packages",
		Long: `Scans the active Node.js version's bin directory and creates shim entries
for any globally installed packages (e.g., pnpm, tsc, eslint).

This runs automatically during 'nodeman setup' and 'nodeman use', but you
can run it manually after installing global packages with npm:

  npm install -g pnpm
  nodeman shims sync`,
		RunE: func(cmd *cobra.Command, args []string) error {
			synced, err := shim.SyncShims()
			if err != nil {
				return err
			}
			if synced > 0 {
				fmt.Printf("Created %d new shim(s).\n", synced)
			} else {
				fmt.Println("All shims are up to date.")
			}
			return nil
		},
	})

	return cmd
}
