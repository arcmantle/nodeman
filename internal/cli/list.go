package cli

import (
	"fmt"

	"github.com/arcmantle/nodeman/internal/config"
	"github.com/arcmantle/nodeman/internal/versions"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List installed Node.js versions",
		RunE: func(cmd *cobra.Command, args []string) error {
			installed, err := versions.ListInstalled()
			if err != nil {
				return err
			}

			if len(installed) == 0 {
				fmt.Println("No Node.js versions installed. Run 'nodeman install <version>' to get started.")
				return nil
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}

			for _, v := range installed {
				marker := "  "
				if v.Version == cfg.ActiveVersion {
					marker = "* "
				}
				fmt.Printf("%s%s\n", marker, v.Version)
			}
			return nil
		},
	}
}
