package cli

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/roen/nodeman/internal/discover"
	"github.com/spf13/cobra"
)

func newCleanCmd() *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove external Node.js installations from the system",
		Long: `Detects Node.js installations not managed by nodeman and helps remove them.

For package-manager installations (Homebrew, Snap, etc.), runs the appropriate
uninstall command. For version managers (nvm, fnm, Volta), removes their
Node.js directories.

Use --yes to skip confirmation prompts.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			installations, err := discover.FindAll()
			if err != nil {
				return fmt.Errorf("scanning for Node.js installations: %w", err)
			}

			var external []discover.Installation
			for _, inst := range installations {
				if inst.Source != "nodeman" {
					external = append(external, inst)
				}
			}

			if len(external) == 0 {
				fmt.Println("No external Node.js installations found. You're all clean!")
				return nil
			}

			fmt.Println("Found external Node.js installations:")
			fmt.Println()
			for i, inst := range external {
				fmt.Printf("  [%d] v%s (%s)\n", i+1, inst.Version, inst.Source)
				fmt.Printf("      %s\n", inst.Path)
			}
			fmt.Println()

			reader := bufio.NewReader(os.Stdin)
			removed := 0

			for i, inst := range external {
				action := describeRemoval(inst)
				fmt.Printf("[%d/%d] v%s (%s)\n", i+1, len(external), inst.Version, inst.Source)
				fmt.Printf("      Action: %s\n", action.Description)

				if !action.Supported {
					fmt.Printf("      Skipped — manual removal required.\n\n")
					continue
				}

				if !yes {
					fmt.Printf("      Proceed? [y/N] ")
					answer, _ := reader.ReadString('\n')
					answer = strings.TrimSpace(strings.ToLower(answer))
					if answer != "y" && answer != "yes" {
						fmt.Printf("      Skipped.\n\n")
						continue
					}
				}

				if err := action.Execute(); err != nil {
					fmt.Printf("      ✗ Failed: %s\n\n", err)
				} else {
					fmt.Printf("      ✓ Removed.\n\n")
					removed++
				}
			}

			if removed > 0 {
				fmt.Printf("Removed %d installation(s).\n", removed)
			} else {
				fmt.Println("No installations were removed.")
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompts")

	return cmd
}

type removalAction struct {
	Description string
	Supported   bool
	Execute     func() error
}

func describeRemoval(inst discover.Installation) removalAction {
	switch inst.Source {
	case "Homebrew":
		return removalAction{
			Description: "brew uninstall node",
			Supported:   true,
			Execute: func() error {
				cmd := exec.Command("brew", "uninstall", "--ignore-dependencies", "node")
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				return cmd.Run()
			},
		}

	case "nvm":
		desc := fmt.Sprintf("remove %s", inst.RootDir)
		if runtime.GOOS == "windows" {
			desc = fmt.Sprintf("nvm uninstall %s (fallback: remove %s)", inst.Version, inst.RootDir)
		}
		return removalAction{
			Description: desc,
			Supported:   true,
			Execute: func() error {
				return removeNvmInstall(inst)
			},
		}

	case "fnm":
		return removalAction{
			Description: fmt.Sprintf("remove %s", inst.RootDir),
			Supported:   true,
			Execute: func() error {
				return removeManagedDir(inst.RootDir)
			},
		}

	case "Volta":
		return removalAction{
			Description: fmt.Sprintf("remove %s", inst.RootDir),
			Supported:   true,
			Execute: func() error {
				return removeManagedDir(inst.RootDir)
			},
		}

	case "Snap":
		return removalAction{
			Description: "sudo snap remove node",
			Supported:   true,
			Execute: func() error {
				cmd := exec.Command("sudo", "snap", "remove", "node")
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				return cmd.Run()
			},
		}

	case "official installer":
		if runtime.GOOS == "windows" {
			return removalAction{
				Description: "launch the Windows Node.js uninstaller",
				Supported:   true,
				Execute:     removeWindowsInstall,
			}
		}
		return removalAction{
			Description: "remove Node.js files from /usr/local (requires sudo)",
			Supported:   true,
			Execute:     removeOfficialInstall,
		}

	case "system":
		// Check if it's at /usr/local (official tarball install, not a distro package)
		if strings.HasPrefix(inst.Path, "/usr/local/") {
			return removalAction{
				Description: "remove Node.js files from /usr/local (requires sudo)",
				Supported:   true,
				Execute:     removeOfficialInstall,
			}
		}
		return removalAction{
			Description: "System package — remove via your package manager (apt, yum, etc.)",
			Supported:   false,
		}

	default:
		return removalAction{
			Description: fmt.Sprintf("Unknown source — manually remove %s", inst.Path),
			Supported:   false,
		}
	}
}

// removeOfficialInstall removes Node.js files installed by the official
// macOS .pkg installer or a manual tarball extraction into /usr/local.
func removeOfficialInstall() error {
	// Files and directories placed by the official Node.js installer
	targets := []string{
		"/usr/local/bin/node",
		"/usr/local/bin/npm",
		"/usr/local/bin/npx",
		"/usr/local/bin/corepack",
		"/usr/local/lib/node_modules",
		"/usr/local/include/node",
		"/usr/local/share/doc/node",
		"/usr/local/share/man/man1/node.1",
		"/usr/local/share/systemtap/tapset/node.stp",
	}

	var toRemove []string
	for _, t := range targets {
		if _, err := os.Stat(t); err == nil {
			toRemove = append(toRemove, t)
		}
	}

	if len(toRemove) == 0 {
		fmt.Println("      No Node.js files found in /usr/local.")
		return nil
	}

	fmt.Println("      Removing:")
	for _, t := range toRemove {
		fmt.Printf("        %s\n", t)
	}

	// Build a single sudo rm command for all targets
	args := []string{"rm", "-rf"}
	args = append(args, toRemove...)

	cmd := exec.Command("sudo", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sudo rm failed: %w", err)
	}

	// Also clean up any leftover node-related receipts on macOS
	if runtime.GOOS == "darwin" {
		receipts, _ := filepath.Glob("/var/db/receipts/org.nodejs.*")
		if len(receipts) > 0 {
			fmt.Println("      Removing installer receipts...")
			rArgs := []string{"rm", "-f"}
			rArgs = append(rArgs, receipts...)
			rCmd := exec.Command("sudo", rArgs...)
			rCmd.Stdout = os.Stdout
			rCmd.Stderr = os.Stderr
			rCmd.Stdin = os.Stdin
			_ = rCmd.Run() // best effort
		}
	}

	return nil
}

// removeWindowsInstall finds the Node.js MSI uninstaller from the registry and runs it.
func removeWindowsInstall() error {
	// Use PowerShell to find the Node.js uninstall string from the registry
	psScript := `
$paths = @(
    'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*',
    'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*',
    'HKCU:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*'
)
foreach ($path in $paths) {
    $entries = Get-ItemProperty $path -ErrorAction SilentlyContinue | Where-Object { $_.DisplayName -like 'Node.js*' }
    foreach ($entry in $entries) {
        if ($entry.UninstallString) {
            Write-Output $entry.UninstallString
            exit 0
        }
    }
}
exit 1
`
	findCmd := exec.Command("powershell", "-NoProfile", "-Command", psScript)
	output, err := findCmd.Output()
	if err != nil {
		fmt.Println("      Could not find Node.js in the Windows uninstall registry.")
		fmt.Println("      Try removing it manually: Settings > Apps > Installed apps > Node.js > Uninstall")
		return fmt.Errorf("Node.js uninstaller not found in registry")
	}

	uninstallStr := strings.TrimSpace(string(output))
	fmt.Printf("      Found uninstaller: %s\n", uninstallStr)

	// The uninstall string is typically: MsiExec.exe /I{GUID} or MsiExec.exe /X{GUID}
	// Normalize to /X (uninstall) and add /passive for a non-silent but auto-progressing UI
	uninstallStr = strings.Replace(uninstallStr, "/I", "/X", 1)

	// Extract the GUID from the msiexec command
	parts := strings.Fields(uninstallStr)
	if len(parts) < 2 {
		return fmt.Errorf("unexpected uninstall string format: %s", uninstallStr)
	}

	// Run msiexec with /passive for a progress bar (not fully silent, but no user clicks needed)
	args := append(parts[1:], "/passive")
	cmd := exec.Command("msiexec.exe", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("uninstaller failed: %w", err)
	}

	return nil
}

func removeNvmInstall(inst discover.Installation) error {
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("nvm"); err == nil {
			cmd := exec.Command("nvm", "uninstall", inst.Version)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err == nil {
				return nil
			}
			fmt.Println("      nvm uninstall failed; falling back to direct directory removal.")
		}
	}

	return removeManagedDir(inst.RootDir)
}

func removeManagedDir(dir string) error {
	if runtime.GOOS == "windows" {
		// Best-effort: clear read-only attributes before deletion.
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			mode := os.FileMode(0o666)
			if d.IsDir() {
				mode = 0o777
			}
			_ = os.Chmod(path, mode)
			return nil
		})
	}

	if err := os.RemoveAll(dir); err != nil {
		if runtime.GOOS == "windows" {
			return fmt.Errorf("%w (directory may be in use; close running node/npm processes and retry)", err)
		}
		return err
	}

	return nil
}
