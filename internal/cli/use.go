package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/roen/nodeman/internal/config"
	"github.com/roen/nodeman/internal/globals"
	"github.com/roen/nodeman/internal/platform"
	"github.com/roen/nodeman/internal/shim"
	"github.com/roen/nodeman/internal/versions"
	"github.com/spf13/cobra"
)

func newUseCmd() *cobra.Command {
	var previous bool

	cmd := &cobra.Command{
		Use:   "use [version]",
		Short: "Set the active Node.js version",
		Long: `Set the active Node.js version. If the version is not installed, you will
be prompted to install it first.

If no version argument is given, nodeman looks for a .nvmrc or .node-version
file in the current directory or any parent directory.

Accepts the same version specifiers as 'install':
  nodeman use 22          # latest installed 22.x, or resolve + install
  nodeman use lts         # latest LTS
  nodeman use 22.14.0     # exact version
  nodeman use --previous  # switch back to the previously active version`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Determine the version input
			var input string
			if previous {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				if cfg.PreviousVersion == "" {
					return fmt.Errorf("no previous version recorded")
				}
				input = cfg.PreviousVersion
			} else if len(args) > 0 {
				input = args[0]
			} else {
				// Try to find a version file
				v, file, err := versions.FindVersionFile()
				if err != nil {
					return err
				}
				if v == "" {
					return fmt.Errorf("no version argument given and no .nvmrc or .node-version file found")
				}
				fmt.Printf("Found %s: %s\n", file, v)
				input = v
			}

			// First try to match against installed versions
			versionNum := input
			installed, err := versions.ListInstalled()
			if err != nil {
				return err
			}

			// Check if input matches an installed version directly or as a prefix
			var matchedVersion string
			for _, v := range installed {
				if v.Version == versionNum || strings.HasPrefix(v.Version, versionNum+".") {
					matchedVersion = v.Version
					break
				}
			}

			// If no local match, resolve remotely and install
			if matchedVersion == "" {
				fmt.Println("Version not installed locally. Fetching remote versions...")
				remote, err := versions.FetchRemoteVersions()
				if err != nil {
					return err
				}

				resolved, err := versions.ResolveVersion(input, remote)
				if err != nil {
					return err
				}

				vNum := strings.TrimPrefix(resolved, "v")
				isInst, err := versions.IsInstalled(vNum)
				if err != nil {
					return err
				}

				if !isInst {
					fmt.Printf("Installing Node.js %s...\n", vNum)
					if err := versions.Install(resolved); err != nil {
						return err
					}
				}
				matchedVersion = vNum
			}

			// Set as active
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			oldVersion := cfg.ActiveVersion
			cfg.PreviousVersion = oldVersion
			cfg.ActiveVersion = matchedVersion
			if err := config.Save(cfg); err != nil {
				return err
			}

			fmt.Printf("Now using Node.js %s\n", matchedVersion)

			// Reinstall global packages if we actually switched versions
			if oldVersion != matchedVersion {
				versionsDir, err := platform.VersionsDir()
				if err != nil {
					return err
				}
				binDir := platform.BinDir(filepath.Join(versionsDir, matchedVersion))
				if err := globals.ReinstallAll(binDir); err != nil {
					fmt.Printf("Warning: failed to reinstall globals: %s\n", err)
				}
			}

			// Sync and prune shims for globally installed packages
			if synced, pruned, err := shim.SyncShims(); err == nil && (synced > 0 || pruned > 0) {
				fmt.Printf("Shims synced: %d created/updated, %d pruned.\n", synced, pruned)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&previous, "previous", false, "Switch to the previously active version")
	return cmd
}
