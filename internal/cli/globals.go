package cli

import (
	"github.com/arcmantle/nodeman/internal/globals"
	"github.com/spf13/cobra"
)

func newGlobalsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "globals",
		Short: "Manage tracked global npm packages",
		Long: `Manage the list of global npm packages that are automatically reinstalled
when you switch Node.js versions with 'nodeman use'.`,
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List tracked global packages",
			RunE: func(cmd *cobra.Command, args []string) error {
				return globals.List()
			},
		},
		&cobra.Command{
			Use:   "add <package> [packages...]",
			Short: "Add package(s) to the globals manifest",
			Args:  cobra.MinimumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				for _, pkg := range args {
					if err := globals.Add(pkg); err != nil {
						return err
					}
				}
				return nil
			},
		},
		&cobra.Command{
			Use:   "remove <package> [packages...]",
			Short: "Remove package(s) from the globals manifest",
			Args:  cobra.MinimumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				for _, pkg := range args {
					if err := globals.Remove(pkg); err != nil {
						return err
					}
				}
				return nil
			},
		},
	)

	return cmd
}
