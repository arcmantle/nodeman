package cli

import (
	"fmt"
	"path/filepath"

	"github.com/arcmantle/nodeman/internal/config"
	"github.com/arcmantle/nodeman/internal/platform"
	"github.com/spf13/cobra"
)

func newDirCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dir [subdir]",
		Short: "Print the path to nodeman directories",
		Long: `Print the path to nodeman directories.

  nodeman dir              ~/.nodeman
  nodeman dir shims        ~/.nodeman/shims
  nodeman dir versions     ~/.nodeman/versions
  nodeman dir active       ~/.nodeman/versions/<active version>`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				root, err := platform.RootDir()
				if err != nil {
					return err
				}
				fmt.Println(root)
				return nil
			}

			switch args[0] {
			case "shims":
				dir, err := platform.ShimsDir()
				if err != nil {
					return err
				}
				fmt.Println(dir)

			case "versions":
				dir, err := platform.VersionsDir()
				if err != nil {
					return err
				}
				fmt.Println(dir)

			case "active":
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				if cfg.ActiveVersion == "" {
					return fmt.Errorf("no active version set — run 'nodeman use <version>'")
				}
				versionsDir, err := platform.VersionsDir()
				if err != nil {
					return err
				}
				fmt.Println(filepath.Join(versionsDir, cfg.ActiveVersion))

			default:
				return fmt.Errorf("unknown directory %q — use shims, versions, or active", args[0])
			}

			return nil
		},
		ValidArgs: []string{"shims", "versions", "active"},
	}

	return cmd
}
