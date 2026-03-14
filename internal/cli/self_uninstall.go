package cli

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/arcmantle/nodeman/internal/platform"
	"github.com/spf13/cobra"
)

func newSelfUninstallCmd() *cobra.Command {
	var yes bool
	var killProcesses bool

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
			return runSelfUninstall(yes, killProcesses)
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt")
	cmd.Flags().BoolVar(&killProcesses, "kill-processes", false, "On Windows, stop node/nodeman processes running from ~/.nodeman before removal")

	return cmd
}

func runSelfUninstall(yes bool, killProcesses bool) error {
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

	// 2. On Windows, also clean the user-level PATH.
	// Do this before directory removal so PATH is cleaned even when self-delete is deferred.
	if runtime.GOOS == "windows" {
		removeFromWindowsPath(shimsDir)
	}

	// 3. Remove the entire ~/.nodeman directory
	deferred, err := removeNodemanRoot(rootDir, killProcesses)
	if err != nil {
		return fmt.Errorf("failed to remove %s: %w", rootDir, err)
	}
	if deferred {
		fmt.Printf("Scheduled removal of %s after this process exits\n", rootDir)
	} else {
		fmt.Printf("Removed %s\n", rootDir)
	}

	fmt.Println()
	fmt.Println("nodeman has been uninstalled.")
	fmt.Println("Restart your terminal for PATH changes to take effect.")

	return nil
}

func removeNodemanRoot(rootDir string, killProcesses bool) (bool, error) {
	if err := os.RemoveAll(rootDir); err == nil {
		return false, nil
	} else if runtime.GOOS != "windows" {
		return false, err
	} else {
		// Best-effort: clear read-only attributes and try once more.
		_ = filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			mode := os.FileMode(0o666)
			if d.IsDir() {
				mode = 0o777
			}
			_ = os.Chmod(path, mode)
			return nil
		})

		if killProcesses {
			if killed, killErr := killWindowsNodemanProcesses(rootDir, os.Getpid()); killErr != nil {
				fmt.Printf("  Warning: could not stop nodeman-managed processes: %v\n", killErr)
			} else if killed > 0 {
				fmt.Printf("Stopped %d process(es) running from %s\n", killed, rootDir)
			}
		}

		if retryErr := os.RemoveAll(rootDir); retryErr == nil {
			return false, nil
		} else {
			if !runningFromNodemanRoot(rootDir) && !isLikelyWindowsFileLock(retryErr) {
				return false, retryErr
			}

			if scheduleErr := scheduleWindowsRemove(rootDir, killProcesses); scheduleErr != nil {
				return false, fmt.Errorf("%w (and deferred cleanup failed: %v)", retryErr, scheduleErr)
			}

			return true, nil
		}
	}
}


func scheduleWindowsRemove(rootDir string, killProcesses bool) error {
	escapedRoot := strings.ReplaceAll(rootDir, "'", "''")
	killBlock := ""
	if killProcesses {
		killBlock = `$targets = Get-Process node,nodeman -ErrorAction SilentlyContinue | Where-Object { $_.Path -and $_.Path.StartsWith($p, [System.StringComparison]::OrdinalIgnoreCase) -and $_.Id -ne $PID }; if($targets){ $targets | Stop-Process -Force -ErrorAction SilentlyContinue }; `
	}

	script := fmt.Sprintf(
		`$p='%s'; for($i=0; $i -lt 120; $i++){ try { %sif(-not (Test-Path -LiteralPath $p)) { exit 0 }; Remove-Item -LiteralPath $p -Recurse -Force -ErrorAction Stop; exit 0 } catch { Start-Sleep -Seconds 1 } }; exit 1`,
		escapedRoot,
		killBlock,
	)
	cmd := exec.Command("powershell", "-NoProfile", "-WindowStyle", "Hidden", "-Command", script)
	return cmd.Start()
}

func killWindowsNodemanProcesses(rootDir string, excludePID int) (int, error) {
	if runtime.GOOS != "windows" {
		return 0, nil
	}

	script := fmt.Sprintf(
		`$p='%s'; $exclude=%d; $targets = Get-Process node,nodeman -ErrorAction SilentlyContinue | Where-Object { $_.Path -and $_.Path.StartsWith($p, [System.StringComparison]::OrdinalIgnoreCase) -and $_.Id -ne $exclude }; $count=@($targets).Count; if($count -gt 0){ $targets | Stop-Process -Force -ErrorAction SilentlyContinue }; Write-Output $count`,
		strings.ReplaceAll(rootDir, "'", "''"),
		excludePID,
	)
	cmd := exec.Command("powershell", "-NoProfile", "-Command", script)
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	countStr := strings.TrimSpace(string(out))
	if countStr == "" {
		return 0, nil
	}

	killed, err := strconv.Atoi(countStr)
	if err != nil {
		return 0, fmt.Errorf("parsing stopped process count %q: %w", countStr, err)
	}

	return killed, nil
}

func runningFromNodemanRoot(rootDir string) bool {
	execPath, err := os.Executable()
	if err != nil {
		return false
	}

	if resolved, resolveErr := filepath.EvalSymlinks(execPath); resolveErr == nil {
		execPath = resolved
	}

	return pathInside(execPath, rootDir)
}

func isLikelyWindowsFileLock(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "access is denied") ||
		strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "used by another process")
}

func pathInside(path, root string) bool {
	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(root)

	if runtime.GOOS == "windows" {
		cleanPath = strings.ToLower(cleanPath)
		cleanRoot = strings.ToLower(cleanRoot)
	}

	if cleanPath == cleanRoot {
		return true
	}

	return strings.HasPrefix(cleanPath, cleanRoot+string(os.PathSeparator))
}

// cleanShellProfiles removes nodeman PATH and completion entries from shell profiles.
func cleanShellProfiles(shimsDir string) {
	if runtime.GOOS == "windows" {
		cleanPowerShellProfiles(shimsDir)
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

func cleanPowerShellProfiles(shimsDir string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	profiles := []string{
		filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1"),
		filepath.Join(home, "Documents", "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1"),
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
