package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/roen/nodeman/internal/platform"
	"github.com/spf13/cobra"
)

func newSelfUninstallCmd() *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "self-uninstall",
		Short: "Remove nodeman and all its data from your system",
		Long: `Completely removes nodeman from your system:

  1. Removes the ~/.nodeman directory (shims, installed versions, config)
  2. Removes the PATH entry from your shell profile
  3. Removes shell completion configuration

After running this command, nodeman will no longer be available.
You may need to restart your terminal for PATH changes to take effect.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSelfUninstall(yes)
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt")

	return cmd
}

func runSelfUninstall(yes bool) error {
	rootDir, err := platform.RootDir()
	if err != nil {
		return fmt.Errorf("cannot determine nodeman directory: %w", err)
	}

	shimsDir, err := platform.ShimsDir()
	if err != nil {
		return fmt.Errorf("cannot determine shims directory: %w", err)
	}

	fmt.Println("This will remove nodeman and all its data:")
	fmt.Printf("  - %s (shims, installed Node.js versions, config)\n", rootDir)
	fmt.Println("  - PATH entry from shell profile")
	fmt.Println("  - Shell completion configuration")
	fmt.Println()

	if !yes {
		fmt.Print("Are you sure? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// 1. Clean shell profile entries (PATH and completions)
	cleanShellProfiles(shimsDir)

	// 2. Remove the entire ~/.nodeman directory
	if err := os.RemoveAll(rootDir); err != nil {
		return fmt.Errorf("failed to remove %s: %w", rootDir, err)
	}
	fmt.Printf("Removed %s\n", rootDir)

	// 3. On Windows, also clean the user-level PATH
	if runtime.GOOS == "windows" {
		removeFromWindowsPath(shimsDir)
	}

	fmt.Println()
	fmt.Println("nodeman has been uninstalled.")
	fmt.Println("Restart your terminal for PATH changes to take effect.")

	return nil
}

// cleanShellProfiles removes nodeman PATH and completion entries from shell profiles.
func cleanShellProfiles(shimsDir string) {
	if runtime.GOOS == "windows" {
		return
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	// Check all common shell profiles
	profiles := []string{
		filepath.Join(home, ".zshrc"),
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".bash_profile"),
		filepath.Join(home, ".profile"),
		filepath.Join(home, ".config", "fish", "config.fish"),
	}

	for _, profile := range profiles {
		content, err := os.ReadFile(profile)
		if err != nil {
			continue
		}

		original := string(content)
		cleaned := removeNodemanLines(original, shimsDir)

		if cleaned != original {
			if err := os.WriteFile(profile, []byte(cleaned), 0o644); err != nil {
				fmt.Printf("  Warning: could not clean %s: %v\n", profile, err)
			} else {
				fmt.Printf("Cleaned %s\n", profile)
			}
		}
	}

	// Remove fish completions file
	fishCompletions := filepath.Join(home, ".config", "fish", "completions", "nodeman.fish")
	if err := os.Remove(fishCompletions); err == nil {
		fmt.Println("Removed fish completions file")
	}
}

// removeNodemanLines removes nodeman-related blocks from a shell profile.
func removeNodemanLines(content, shimsDir string) string {
	lines := strings.Split(content, "\n")
	var result []string
	skip := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip comment lines that introduce nodeman blocks
		if trimmed == "# nodeman - Node.js version manager" ||
			trimmed == "# nodeman shell completions" {
			skip = true
			continue
		}

		// Skip the actual nodeman config lines following a comment
		if skip {
			if strings.Contains(line, shimsDir) ||
				strings.Contains(line, "nodeman completion") {
				skip = false
				continue
			}
			skip = false
		}

		// Also catch standalone nodeman lines without the comment header
		if strings.Contains(line, shimsDir) && strings.Contains(line, "export PATH") {
			continue
		}
		if strings.Contains(line, "nodeman completion") {
			continue
		}

		result = append(result, line)
	}

	// Remove trailing blank lines that were left behind
	for len(result) > 0 && strings.TrimSpace(result[len(result)-1]) == "" {
		result = result[:len(result)-1]
	}

	return strings.Join(result, "\n") + "\n"
}

// removeFromWindowsPath removes shimsDir from the user-level PATH on Windows.
func removeFromWindowsPath(shimsDir string) {
	checkCmd := exec.Command("powershell", "-NoProfile", "-Command",
		`[Environment]::GetEnvironmentVariable('Path', 'User')`)
	userPathBytes, err := checkCmd.Output()
	if err != nil {
		return
	}

	userPath := strings.TrimSpace(string(userPathBytes))
	lowerShims := strings.ToLower(shimsDir)

	parts := strings.Split(userPath, ";")
	var cleaned []string
	for _, p := range parts {
		if strings.ToLower(strings.TrimSpace(p)) != lowerShims {
			cleaned = append(cleaned, p)
		}
	}

	newPath := strings.Join(cleaned, ";")
	if newPath == userPath {
		return
	}

	setCmd := exec.Command("powershell", "-NoProfile", "-Command",
		fmt.Sprintf(`[Environment]::SetEnvironmentVariable('Path', '%s', 'User')`,
			strings.ReplaceAll(newPath, "'", "''")))
	if err := setCmd.Run(); err != nil {
		fmt.Println("  Warning: could not update Windows PATH")
	} else {
		fmt.Println("Removed nodeman from Windows PATH")
	}
}
