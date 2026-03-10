package cli

import (
	"fmt"
	"strings"

	"github.com/roen/nodeman/internal/versions"
	"github.com/spf13/cobra"
)

func newInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install <version>",
		Short: "Download and install a Node.js version",
		Long: `Download and install a Node.js version from nodejs.org.

Accepts version specifiers:
  nodeman install 22          # latest 22.x
  nodeman install 22.14       # latest 22.14.x
  nodeman install 22.14.0     # exact version
  nodeman install lts          # latest LTS
  nodeman install latest       # latest overall`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Fetching available versions...")
			remote, err := versions.FetchRemoteVersions()
			if err != nil {
				return err
			}

			resolved, err := versions.ResolveVersion(args[0], remote)
			if err != nil {
				return err
			}

			versionNum := strings.TrimPrefix(resolved, "v")
			installed, err := versions.IsInstalled(versionNum)
			if err != nil {
				return err
			}
			if installed {
				fmt.Printf("Node.js %s is already installed.\n", versionNum)
				return nil
			}

			return versions.Install(resolved)
		},
	}
}
