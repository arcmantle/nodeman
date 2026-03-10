package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/roen/nodeman/internal/config"
	"github.com/roen/nodeman/internal/discover"
	"github.com/roen/nodeman/internal/platform"
	"github.com/spf13/cobra"
)

func newAdoptCmd() *cobra.Command {
	var setActive bool

	cmd := &cobra.Command{
		Use:   "adopt [version]",
		Short: "Import an existing system Node.js installation into nodeman",
		Long: `Scans the system for Node.js installations not managed by nodeman and
offers to adopt them by copying the installation into ~/.nodeman/versions/.

If no version argument is given, all detected installations are shown and you
can pick which ones to adopt. If a version is specified, nodeman will look
for a matching installation and adopt it directly.

After adoption, the original installation is left untouched — you can
uninstall it separately (e.g., 'brew uninstall node').`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			installations, err := discover.FindAll()
			if err != nil {
				return fmt.Errorf("scanning for Node.js installations: %w", err)
			}

			// Filter out anything already managed by nodeman
			var external []discover.Installation
			for _, inst := range installations {
				if inst.Source != "nodeman" {
					external = append(external, inst)
				}
			}

			if len(external) == 0 {
				fmt.Println("No external Node.js installations found.")
				fmt.Println("Use 'nodeman install <version>' to install one.")
				return nil
			}

			// If a version argument was given, find a match
			if len(args) == 1 {
				target := strings.TrimPrefix(args[0], "v")
				for _, inst := range external {
					if inst.Version == target || strings.HasPrefix(inst.Version, target+".") || strings.HasPrefix(inst.Version, target) {
						return adoptInstallation(inst, setActive)
					}
				}
				return fmt.Errorf("no external installation matching %q found", args[0])
			}

			// Interactive selection
			fmt.Println("Detected Node.js installations:")
			fmt.Println()
			for i, inst := range external {
				fmt.Printf("  [%d] v%s (%s)\n", i+1, inst.Version, inst.Source)
				fmt.Printf("      %s\n", inst.Path)
			}
			fmt.Println()

			reader := bufio.NewReader(os.Stdin)
			adopted := 0
			for i, inst := range external {
				fmt.Printf("[%d/%d] Adopt v%s (%s)? [y/N] ", i+1, len(external), inst.Version, inst.Source)
				answer, _ := reader.ReadString('\n')
				answer = strings.TrimSpace(strings.ToLower(answer))
				if answer == "y" || answer == "yes" {
					if err := adoptInstallation(inst, setActive); err != nil {
						fmt.Printf("  ✗ Error adopting v%s: %s\n", inst.Version, err)
					} else {
						adopted++
					}
				} else {
					fmt.Printf("  Skipped v%s\n", inst.Version)
				}
			}
			if adopted > 0 {
				fmt.Printf("\nAdopted %d installation(s).\n", adopted)
			} else {
				fmt.Println("\nNo installations adopted.")
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&setActive, "set-active", false, "Set the adopted version as the active version")

	return cmd
}

// adoptInstallation copies an existing Node.js installation into nodeman's versions directory.
func adoptInstallation(inst discover.Installation, setActive bool) error {
	versionsDir, err := platform.VersionsDir()
	if err != nil {
		return err
	}

	destDir := filepath.Join(versionsDir, inst.Version)

	// Check if already managed
	if _, err := os.Stat(destDir); err == nil {
		fmt.Printf("  v%s is already managed by nodeman.\n", inst.Version)
		if setActive {
			return setActiveVersion(inst.Version)
		}
		return nil
	}

	if err := platform.EnsureDirs(); err != nil {
		return err
	}

	fmt.Printf("Copying v%s from %s...\n", inst.Version, inst.RootDir)

	// Copy the entire installation directory tree
	if err := copyDir(inst.RootDir, destDir); err != nil {
		// Clean up partial copy
		os.RemoveAll(destDir)
		return fmt.Errorf("copying installation: %w", err)
	}

	// Verify the copied node binary works
	nodeBin := filepath.Join(platform.BinDir(destDir), "node"+platform.ExeSuffix())
	if _, err := os.Stat(nodeBin); os.IsNotExist(err) {
		os.RemoveAll(destDir)
		return fmt.Errorf("adopted installation missing node binary at expected path")
	}

	fmt.Printf("  Adopted v%s into %s\n", inst.Version, destDir)

	if setActive {
		return setActiveVersion(inst.Version)
	}

	return nil
}

func setActiveVersion(version string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.ActiveVersion = version
	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("  Set v%s as active version.\n", version)
	return nil
}

// copyDir recursively copies a directory tree.
func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		info, err := entry.Info()
		if err != nil {
			return err
		}

		// Handle symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(srcPath)
			if err != nil {
				return err
			}
			// Validate symlink stays within the source tree
			if filepath.IsAbs(linkTarget) {
				// Skip absolute symlinks — they can't be safely relocated
				continue
			}
			resolved := filepath.Join(filepath.Dir(srcPath), linkTarget)
			if !strings.HasPrefix(filepath.Clean(resolved)+string(os.PathSeparator), filepath.Clean(src)+string(os.PathSeparator)) &&
				filepath.Clean(resolved) != filepath.Clean(src) {
				// Skip symlinks that escape the source tree
				continue
			}
			if err := os.Symlink(linkTarget, dstPath); err != nil {
				return err
			}
			continue
		}

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFileAdopt(srcPath, dstPath, info.Mode()); err != nil {
				return err
			}
		}
	}

	return nil
}

func copyFileAdopt(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = bufio.NewReader(in).WriteTo(out)
	return err
}
