package cli

import (
	"fmt"

	"github.com/arcmantle/nodeman/internal/config"
	"github.com/arcmantle/nodeman/internal/versions"
	"github.com/spf13/cobra"
)

func newUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "uninstall <version>",
		Aliases: []string{"rm", "remove"},
		Short:   "Remove an installed Node.js version",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			versionNum := args[0]

			installed, err := versions.IsInstalled(versionNum)
			if err != nil {
				return err
			}
			if !installed {
				return fmt.Errorf("version %s is not installed", versionNum)
			}

			// Check if it's the active version
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.ActiveVersion == versionNum {
				cfg.ActiveVersion = ""
				if err := config.Save(cfg); err != nil {
					return err
				}
				fmt.Println("Cleared active version (was set to the removed version).")
			}

			return versions.Uninstall(versionNum)
		},
	}
}
