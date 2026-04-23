package shim

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/arcmantle/nodeman/internal/config"
	"github.com/arcmantle/nodeman/internal/discover"
	"github.com/arcmantle/nodeman/internal/platform"
)

// Setup creates shim binaries in ~/.nodeman/shims/.
// It copies (or hardlinks) the currently running nodeman binary as node, npm, npx, corepack.
// It also scans for existing Node.js installations and validates PATH ordering.
func Setup() error {
	if err := platform.EnsureDirs(); err != nil {
		return err
	}

	shimsDir, err := platform.ShimsDir()
	if err != nil {
		return err
	}

	// Find the currently running binary
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot find own executable: %w", err)
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}

	suffix := platform.ExeSuffix()
	for _, name := range platform.ShimNames() {
		shimPath := filepath.Join(shimsDir, name+suffix)

		// Remove existing shim
		os.Remove(shimPath)

		// Try hardlink first (most efficient), fall back to copy
		if err := os.Link(self, shimPath); err != nil {
			if err := copyFile(self, shimPath); err != nil {
				return fmt.Errorf("creating shim %s: %w", name, err)
			}
		}

		// On Windows, also create a .cmd wrapper so callers using
		// "npm.cmd", "npx.cmd", etc. are intercepted by nodeman.
		if runtime.GOOS == "windows" {
			if err := writeCmdShim(shimsDir, name); err != nil {
				return fmt.Errorf("creating .cmd shim for %s: %w", name, err)
			}
		}
	}

	// Also create a nodeman symlink/copy in the shims dir for convenience
	nodeman := filepath.Join(shimsDir, "nodeman"+suffix)
	if !samePath(self, nodeman) {
		os.Remove(nodeman)
		if err := os.Link(self, nodeman); err != nil {
			if err := copyFile(self, nodeman); err != nil {
				return fmt.Errorf("creating nodeman shim: %w", err)
			}
		}
		if runtime.GOOS == "windows" {
			if err := writeCmdShim(shimsDir, "nodeman"); err != nil {
				return fmt.Errorf("creating .cmd shim for nodeman: %w", err)
			}
		}
	}

	fmt.Println("Shims created in", shimsDir)

	// Detect existing Node.js installations
	reportExistingInstalls()

	// Validate PATH
	reportPathStatus(shimsDir)

	printPathInstructions(shimsDir)

	// Configure shell completions
	configureCompletions(shimsDir)

	// Sync shims for globally installed packages
	if synced, pruned, err := SyncShims(); err == nil && (synced > 0 || pruned > 0) {
		fmt.Printf("Shims synced: %d created/updated, %d pruned.\n", synced, pruned)
	}

	return nil
}

// SyncShims scans the active Node.js version's bin directory and creates
// shim hardlinks/copies in the shims directory for any binaries that don't
// already have one. It also prunes stale shims that no longer exist in the
// active version's bin directory.
// Returns: createdOrUpdated, pruned, error.
func SyncShims() (int, int, error) {
	cfg, err := config.Load()
	if err != nil || cfg.ActiveVersion == "" {
		return 0, 0, nil
	}

	versionsDir, err := platform.VersionsDir()
	if err != nil {
		return 0, 0, err
	}

	shimsDir, err := platform.ShimsDir()
	if err != nil {
		return 0, 0, err
	}

	binDir := platform.BinDir(filepath.Join(versionsDir, cfg.ActiveVersion))
	entries, err := os.ReadDir(binDir)
	if err != nil {
		return 0, 0, nil
	}

	// Find the nodeman binary in shimsDir to use as the shim source
	suffix := platform.ExeSuffix()
	shimSource := filepath.Join(shimsDir, "nodeman"+suffix)
	if _, err := os.Stat(shimSource); os.IsNotExist(err) {
		return 0, 0, fmt.Errorf("nodeman shim not found in %s — run 'nodeman setup' first", shimsDir)
	}
	shimSource, _ = filepath.EvalSymlinks(shimSource)

	sourceInfo, err := os.Stat(shimSource)
	if err != nil {
		return 0, 0, err
	}

	// Names we should never create shims for from bin entries.
	skipCreate := map[string]bool{
		"nodeman": true,
	}

	// Desired shims after sync (core + discovered binaries + nodeman helper).
	desired := make(map[string]bool)
	desired["nodeman"] = true
	for _, core := range platform.ShimNames() {
		desired[core] = true
	}

	synced := 0
	seen := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name, ok := shimNameFromBinEntry(entry.Name())
		if !ok {
			continue
		}

		if skipCreate[name] || seen[name] {
			continue
		}
		seen[name] = true
		desired[name] = true

		shimPath := filepath.Join(shimsDir, name+suffix)

		// If shim exists, check if it's up to date (same file as nodeman)
		if existingInfo, err := os.Stat(shimPath); err == nil {
			if os.SameFile(sourceInfo, existingInfo) {
				// .exe is current, but ensure .cmd wrapper exists on Windows
				if runtime.GOOS == "windows" {
					_ = writeCmdShim(shimsDir, name)
				}
				continue // already points to the current nodeman binary
			}
			// Stale shim — remove and recreate
			os.Remove(shimPath)
		}

		// Create a new shim
		if err := os.Link(shimSource, shimPath); err != nil {
			if err := copyFile(shimSource, shimPath); err != nil {
				continue
			}
		}
		// On Windows, also create a .cmd wrapper
		if runtime.GOOS == "windows" {
			_ = writeCmdShim(shimsDir, name)
		}
		synced++
	}

	pruned := 0
	if shimEntries, err := os.ReadDir(shimsDir); err == nil {
		for _, entry := range shimEntries {
			if entry.IsDir() {
				continue
			}

			entryName := entry.Name()
			lowerName := strings.ToLower(entryName)
			if runtime.GOOS == "windows" &&
				!strings.HasSuffix(lowerName, ".exe") &&
				!strings.HasSuffix(lowerName, ".cmd") {
				continue
			}

			name := normalizeShimName(entryName)
			if name == "" || desired[name] {
				continue
			}

			if err := os.Remove(filepath.Join(shimsDir, entryName)); err == nil {
				pruned++
			}
		}
	}

	return synced, pruned, nil
}

// reportExistingInstalls scans for Node.js installations outside nodeman and prints them.
func reportExistingInstalls() {
	installations, err := discover.FindAll()
	if err != nil {
		return
	}

	var external []discover.Installation
	for _, inst := range installations {
		if inst.Source != "nodeman" {
			external = append(external, inst)
		}
	}

	if len(external) == 0 {
		return
	}

	fmt.Println()
	fmt.Println("Detected existing Node.js installations:")
	for _, inst := range external {
		fmt.Printf("  - v%s (%s) at %s\n", inst.Version, inst.Source, inst.Path)
	}
	fmt.Println()
	fmt.Println("Run 'nodeman adopt' to import them into nodeman.")
	fmt.Println("After adopting, you can uninstall the originals (e.g. 'brew uninstall node').")
}

// reportPathStatus checks if the shims directory is in PATH and in the correct position.
func reportPathStatus(shimsDir string) {
	inPath, isFirst, otherDirs := discover.ShimsInPath()

	fmt.Println()
	if inPath && isFirst {
		fmt.Println("PATH: OK — nodeman shims take priority.")
	} else if inPath && !isFirst {
		fmt.Println("WARNING: nodeman shims are in PATH but another Node.js appears first:")
		for _, d := range otherDirs {
			fmt.Printf("  - %s\n", d)
		}
		fmt.Println()
		fmt.Printf("Make sure %s appears BEFORE these directories in your PATH.\n", shimsDir)
	} else {
		fmt.Println("NOTE: nodeman shims are not yet in your PATH.")
		if len(otherDirs) > 0 {
			fmt.Println("Other Node.js directories found in PATH:")
			for _, d := range otherDirs {
				fmt.Printf("  - %s\n", d)
			}
		}
	}
}

// writeCmdShim creates a .cmd wrapper script that forwards to the .exe shim.
// This ensures Windows callers that invoke e.g. "npm.cmd" are still intercepted
// by nodeman. The wrapper uses the absolute path to the co-located .exe so that
// it works correctly even when invoked by name only (via PATH) from a different
// working directory. Using %~dp0 is unreliable in that scenario because it can
// resolve to the caller's working directory instead of the shims directory.
func writeCmdShim(shimsDir, name string) error {
	cmdPath := filepath.Join(shimsDir, name+".cmd")
	exePath := filepath.Join(shimsDir, name+".exe")
	content := fmt.Sprintf("@\"%s\" %%*\r\n", exePath)
	return os.WriteFile(cmdPath, []byte(content), 0o755)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func samePath(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func printPathInstructions(shimsDir string) {
	if runtime.GOOS == "windows" {
		addToWindowsPath(shimsDir)
		return
	}

	inPath, isFirst, _ := discover.ShimsInPath()
	if inPath && isFirst {
		return
	}

	// Auto-configure shell profile on Unix systems
	profilePath := detectShellProfile()
	if profilePath == "" {
		fmt.Println()
		fmt.Println("Could not detect shell profile. Add this to your shell config manually:")
		fmt.Printf("  export PATH=\"%s:$PATH\"\n", shimsDir)
		fmt.Println()
		return
	}

	exportLine := fmt.Sprintf("export PATH=\"%s:$PATH\"", shimsDir)

	// Check if already present in profile
	if content, err := os.ReadFile(profilePath); err == nil {
		if strings.Contains(string(content), shimsDir) {
			fmt.Printf("\nPATH entry already exists in %s\n", profilePath)
			fmt.Println("Restart your terminal or run: source", profilePath)
			return
		}
	}

	// Append to profile
	f, err := os.OpenFile(profilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Printf("\nCould not write to %s: %v\n", profilePath, err)
		fmt.Println("Add this manually:")
		fmt.Printf("  %s\n", exportLine)
		return
	}
	defer f.Close()

	if _, err := fmt.Fprintf(f, "\n# nodeman - Node.js version manager\n%s\n", exportLine); err != nil {
		fmt.Printf("\nFailed to write to %s: %v\n", profilePath, err)
		return
	}

	fmt.Printf("\nAdded nodeman to PATH in %s\n", profilePath)
	fmt.Println("Restart your terminal or run: source", profilePath)
}

// addToWindowsPath adds shimsDir to the user-level PATH environment variable on Windows.
func addToWindowsPath(shimsDir string) {
	checkCmd := exec.Command("powershell", "-NoProfile", "-Command",
		`[Environment]::GetEnvironmentVariable('Path', 'User')`)
	userPathBytes, err := checkCmd.Output()
	if err != nil {
		fmt.Println("\nCould not read user PATH. Add this directory to PATH manually:")
		fmt.Println(" ", shimsDir)
		return
	}

	userPath := strings.TrimSpace(string(userPathBytes))
	entries := splitPathEntries(userPath)
	entries = removePathEntry(entries, shimsDir)
	newEntries := append([]string{shimsDir}, entries...)
	newPath := strings.Join(newEntries, ";")

	if strings.EqualFold(strings.TrimSpace(newPath), strings.TrimSpace(userPath)) {
		ensureProcessPathFirst(shimsDir)
		fmt.Println("\nnodeman shims already prioritized in user PATH.")
		fmt.Println("If VS Code is open, restart it so tsserver picks up PATH changes.")
		return
	}

	setCmd := exec.Command("powershell", "-NoProfile", "-Command",
		fmt.Sprintf(`[Environment]::SetEnvironmentVariable('Path', '%s', 'User')`,
			strings.ReplaceAll(newPath, "'", "''")))
	if err := setCmd.Run(); err != nil {
		fmt.Println("\nCould not update user PATH. Add this directory to PATH manually:")
		fmt.Println(" ", shimsDir)
		return
	}

	ensureProcessPathFirst(shimsDir)
	broadcastWindowsEnvironmentChange()

	fmt.Println("\nAdded nodeman to user PATH and moved it to highest user priority.")
	fmt.Println("Restart VS Code and terminal sessions for tsserver to see the updated PATH.")
}

func splitPathEntries(pathValue string) []string {
	if pathValue == "" {
		return nil
	}

	parts := strings.Split(pathValue, ";")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func removePathEntry(entries []string, target string) []string {
	targetClean := filepath.Clean(target)
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		clean := filepath.Clean(entry)
		if strings.EqualFold(clean, targetClean) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func ensureProcessPathFirst(shimsDir string) {
	processEntries := splitPathEntries(os.Getenv("PATH"))
	processEntries = removePathEntry(processEntries, shimsDir)
	processEntries = append([]string{shimsDir}, processEntries...)
	_ = os.Setenv("PATH", strings.Join(processEntries, ";"))
}

func broadcastWindowsEnvironmentChange() {
	cmd := exec.Command("powershell", "-NoProfile", "-Command", `$sig = @"
using System;
using System.Runtime.InteropServices;
public static class EnvNotify {
	[DllImport("user32.dll", CharSet = CharSet.Auto, SetLastError = true)]
	public static extern IntPtr SendMessageTimeout(IntPtr hWnd, uint Msg, UIntPtr wParam, string lParam, uint flags, uint timeout, out UIntPtr result);
}
"@; Add-Type -TypeDefinition $sig -ErrorAction SilentlyContinue | Out-Null; $r = [uintptr]::Zero; [void][EnvNotify]::SendMessageTimeout([intptr]0xffff, 0x1A, [uintptr]::Zero, 'Environment', 0x0002, 5000, [ref]$r)`)
	_ = cmd.Run()
}

// detectShellProfile returns the path to the user's shell profile file.
func detectShellProfile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Check SHELL env var first
	shell := os.Getenv("SHELL")
	switch {
	case strings.HasSuffix(shell, "/zsh"):
		return filepath.Join(home, ".zshrc")
	case strings.HasSuffix(shell, "/bash"):
		// Prefer .bashrc on Linux, .bash_profile on macOS
		if runtime.GOOS == "darwin" {
			return filepath.Join(home, ".bash_profile")
		}
		return filepath.Join(home, ".bashrc")
	case strings.HasSuffix(shell, "/fish"):
		return filepath.Join(home, ".config", "fish", "config.fish")
	}

	// Fallback: check which files exist
	for _, name := range []string{".zshrc", ".bashrc", ".bash_profile", ".profile"} {
		p := filepath.Join(home, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}

// configureCompletions sets up shell completions for nodeman.
func configureCompletions(shimsDir string) {
	if runtime.GOOS == "windows" {
		configurePowerShellCompletions()
		return
	}

	shell := os.Getenv("SHELL")
	nodeman := filepath.Join(shimsDir, "nodeman"+platform.ExeSuffix())

	switch {
	case strings.HasSuffix(shell, "/zsh"):
		configureZshCompletions(nodeman)
	case strings.HasSuffix(shell, "/bash"):
		configureBashCompletions(nodeman)
	case strings.HasSuffix(shell, "/fish"):
		configureFishCompletions(nodeman)
	}
}

func configurePowerShellCompletions() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	profiles := []string{
		filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1"),
		filepath.Join(home, "Documents", "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1"),
	}

	completionLine := `nodeman completion powershell | Out-String | Invoke-Expression`
	configured := false
	alreadyConfigured := false

	for _, profilePath := range profiles {
		if content, err := os.ReadFile(profilePath); err == nil {
			if strings.Contains(string(content), "nodeman completion powershell") {
				alreadyConfigured = true
				continue
			}
		}

		if err := os.MkdirAll(filepath.Dir(profilePath), 0o755); err != nil {
			continue
		}

		f, err := os.OpenFile(profilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			continue
		}

		if _, err := fmt.Fprintf(f, "\n# nodeman shell completions\n%s\n", completionLine); err == nil {
			configured = true
			fmt.Println("Shell completions configured in", profilePath)
		}
		_ = f.Close()
	}

	if !configured && alreadyConfigured {
		fmt.Println("PowerShell completions already configured.")
		return
	}

	if !configured {
		fmt.Println("Could not configure PowerShell completions automatically.")
		fmt.Println("Add this line to your $PROFILE:")
		fmt.Println(" ", completionLine)
	}
}

func configureZshCompletions(nodeman string) {
	profilePath := detectShellProfile()
	if profilePath == "" {
		return
	}

	completionLine := `eval "$(nodeman completion zsh)"`

	if content, err := os.ReadFile(profilePath); err == nil {
		if strings.Contains(string(content), "nodeman completion zsh") {
			return // already configured
		}
	}

	f, err := os.OpenFile(profilePath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	fmt.Fprintf(f, "\n# nodeman shell completions\n%s\n", completionLine)
	fmt.Println("Shell completions configured in", profilePath)
}

func configureBashCompletions(nodeman string) {
	profilePath := detectShellProfile()
	if profilePath == "" {
		return
	}

	completionLine := `eval "$(nodeman completion bash)"`

	if content, err := os.ReadFile(profilePath); err == nil {
		if strings.Contains(string(content), "nodeman completion bash") {
			return // already configured
		}
	}

	f, err := os.OpenFile(profilePath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	fmt.Fprintf(f, "\n# nodeman shell completions\n%s\n", completionLine)
	fmt.Println("Shell completions configured in", profilePath)
}

func configureFishCompletions(nodeman string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	completionsDir := filepath.Join(home, ".config", "fish", "completions")
	completionsFile := filepath.Join(completionsDir, "nodeman.fish")

	// Check if already configured
	if _, err := os.Stat(completionsFile); err == nil {
		return
	}

	if err := os.MkdirAll(completionsDir, 0o755); err != nil {
		return
	}

	// Generate completions and write to file
	cmd := exec.Command(nodeman, "completion", "fish")
	output, err := cmd.Output()
	if err != nil {
		return
	}

	if err := os.WriteFile(completionsFile, output, 0o644); err != nil {
		return
	}

	fmt.Println("Shell completions configured in", completionsFile)
}
