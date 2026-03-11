package cli

import (
	"github.com/spf13/cobra"
)

// NewRootCmd creates the top-level nodeman command with all subcommands.
func NewRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:     "nodeman",
		Short:   "A cross-platform Node.js version manager",
		Version: version,
	}

	root.AddCommand(
		newInstallCmd(),
		newUninstallCmd(),
		newUseCmd(),
		newListCmd(),
		newListRemoteCmd(),
		newCurrentCmd(),
		newSetupCmd(),
		newAdoptCmd(),
		newCleanCmd(),
		newDirCmd(),
		newShimsCmd(),
		newDoctorCmd(),
		newGlobalsCmd(),
		newUpgradeCmd(version),
	)

	return root
}
