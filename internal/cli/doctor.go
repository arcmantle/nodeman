package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/arcmantle/nodeman/internal/auth"
	"github.com/arcmantle/nodeman/internal/config"
	"github.com/arcmantle/nodeman/internal/discover"
	"github.com/arcmantle/nodeman/internal/platform"
	"github.com/arcmantle/nodeman/internal/versions"
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check that nodeman is set up correctly",
		Long: `Runs a series of diagnostic checks to verify that nodeman is properly
configured and controlling Node.js/npm on this system.

Checks include:
  - Active version is set
  - Shims exist in ~/.nodeman/shims/
  - Shims directory is in PATH (and in the correct position)
  - 'which node' and 'which npm' point to nodeman's shims
  - The active node and npm versions match expectations
  - No conflicting Node.js installations shadow the shims`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor()
		},
	}
}

type checkResult struct {
	name   string
	ok     bool
	detail string
}

func runDoctor() error {
	var results []checkResult

	// 1. Check active version
	cfg, err := config.Load()
	if err != nil {
		results = append(results, checkResult{"Active version", false, fmt.Sprintf("cannot load config: %s", err)})
	} else if cfg.ActiveVersion == "" {
		results = append(results, checkResult{"Active version", false, "no active version set — run 'nodeman use <version>'"})
	} else {
		results = append(results, checkResult{"Active version", true, cfg.ActiveVersion})
	}

	if cfg != nil {
		ok, detail := auth.DoctorStatus(cfg)
		results = append(results, checkResult{"Package auth", ok, detail})
	}

	// 1b. Check installed versions
	installed, _ := versions.ListInstalled()
	if len(installed) == 0 {
		results = append(results, checkResult{"Installed versions", false, "no versions installed — run 'nodeman install <version>'"})
	} else {
		versionList := make([]string, 0, len(installed))
		for _, v := range installed {
			versionList = append(versionList, v.Version)
		}
		results = append(results, checkResult{"Installed versions", true, fmt.Sprintf("%d: %s", len(installed), strings.Join(versionList, ", "))})
	}

	// 2. Check shims directory exists
	shimsDir, _ := platform.ShimsDir()
	shimsDirExists := false
	if info, err := os.Stat(shimsDir); err == nil && info.IsDir() {
		shimsDirExists = true
		results = append(results, checkResult{"Shims directory", true, shimsDir})
	} else {
		results = append(results, checkResult{"Shims directory", false, fmt.Sprintf("%s does not exist — run 'nodeman setup'", shimsDir)})
	}

	// 3. Check individual shim binaries exist
	if shimsDirExists {
		suffix := platform.ExeSuffix()
		for _, name := range platform.ShimNames() {
			shimPath := filepath.Join(shimsDir, name+suffix)
			if _, err := os.Stat(shimPath); err == nil {
				results = append(results, checkResult{fmt.Sprintf("Shim: %s", name), true, shimPath})
			} else {
				results = append(results, checkResult{fmt.Sprintf("Shim: %s", name), false, "missing — run 'nodeman setup'"})
			}
		}
	}

	// 4. Check PATH ordering
	inPath, isFirst, otherDirs := discover.ShimsInPath()
	if inPath && isFirst {
		results = append(results, checkResult{"PATH priority", true, "nodeman shims appear before other Node installs"})
	} else if inPath && !isFirst {
		results = append(results, checkResult{"PATH priority", false,
			fmt.Sprintf("shims in PATH but shadowed by: %s", strings.Join(otherDirs, ", "))})
	} else {
		detail := "shims directory not in PATH — add it to your shell profile"
		if len(otherDirs) > 0 {
			detail += fmt.Sprintf(" (other Node found at: %s)", strings.Join(otherDirs, ", "))
		}
		results = append(results, checkResult{"PATH priority", false, detail})
	}

	if runtime.GOOS == "windows" {
		results = append(results, checkWindowsUserPath(shimsDir))
	}

	// 5. Check 'which node' points to shim
	if whichNode, err := exec.LookPath("node"); err == nil {
		resolved, _ := filepath.EvalSymlinks(whichNode)
		if shimsDir != "" && filepath.Clean(filepath.Dir(whichNode)) == filepath.Clean(shimsDir) {
			results = append(results, checkResult{"which node", true, whichNode})
		} else {
			results = append(results, checkResult{"which node", false,
				fmt.Sprintf("points to %s (resolved: %s) — not the nodeman shim", whichNode, resolved)})
		}
	} else {
		results = append(results, checkResult{"which node", false, "node not found in PATH"})
	}

	// 6. Check 'which npm' points to shim
	if whichNpm, err := exec.LookPath("npm"); err == nil {
		if shimsDir != "" && filepath.Clean(filepath.Dir(whichNpm)) == filepath.Clean(shimsDir) {
			results = append(results, checkResult{"which npm", true, whichNpm})
		} else {
			results = append(results, checkResult{"which npm", false,
				fmt.Sprintf("points to %s — not the nodeman shim", whichNpm)})
		}
	} else {
		results = append(results, checkResult{"which npm", false, "npm not found in PATH"})
	}

	// 7. Check active version's node binary exists and reports correct version
	if cfg != nil && cfg.ActiveVersion != "" {
		versionsDir, _ := platform.VersionsDir()
		binDir := platform.BinDir(filepath.Join(versionsDir, cfg.ActiveVersion))

		nodeBin, nodeErr := platform.ResolveBinCommand(binDir, "node")
		if nodeErr == nil {
			out, err := platform.CommandForBinary(nodeBin, "--version").Output()
			if err == nil {
				ver := strings.TrimSpace(string(out))
				expected := "v" + cfg.ActiveVersion
				if ver == expected {
					results = append(results, checkResult{"node --version", true, ver})
				} else {
					results = append(results, checkResult{"node --version", false,
						fmt.Sprintf("expected %s, got %s", expected, ver)})
				}
			} else {
				results = append(results, checkResult{"node --version", false,
					fmt.Sprintf("cannot execute %s: %s", nodeBin, err)})
			}
		} else {
			results = append(results, checkResult{"node --version", false, nodeErr.Error()})
		}

		npmBin, npmErr := platform.ResolveBinCommand(binDir, "npm")
		if npmErr == nil {
			out, err := platform.CommandForBinary(npmBin, "--version").Output()
			if err == nil {
				ver := strings.TrimSpace(string(out))
				results = append(results, checkResult{"npm --version", true, fmt.Sprintf("%s (bundled with node %s)", ver, cfg.ActiveVersion)})
			} else {
				results = append(results, checkResult{"npm --version", false,
					fmt.Sprintf("cannot execute %s: %s", npmBin, err)})
			}
		} else {
			results = append(results, checkResult{"npm --version", false, npmErr.Error()})
		}
	}

	// 8. Check shell completions
	results = append(results, checkCompletions()...)

	// Print results
	fmt.Println("nodeman doctor")
	fmt.Println(strings.Repeat("─", 60))

	allOk := true
	for _, r := range results {
		icon := "✓"
		if !r.ok {
			icon = "✗"
			allOk = false
		}
		fmt.Printf("  %s  %-16s %s\n", icon, r.name, r.detail)
	}

	fmt.Println(strings.Repeat("─", 60))
	if allOk {
		fmt.Println("All checks passed. nodeman is fully in control.")
	} else {
		fmt.Println("Some checks failed. See above for details.")
	}

	return nil
}

// checkCompletions verifies that shell completions are configured.
func checkCompletions() []checkResult {
	shell := os.Getenv("SHELL")
	home, err := os.UserHomeDir()
	if err != nil {
		return []checkResult{{"Completions", false, "cannot determine home directory"}}
	}

	if runtime.GOOS == "windows" {
		return checkPowerShellCompletions(home)
	}

	switch {
	case strings.HasSuffix(shell, "/zsh"):
		return checkProfileContains(home, ".zshrc", "nodeman completion zsh")
	case strings.HasSuffix(shell, "/bash"):
		if runtime.GOOS == "darwin" {
			return checkProfileContains(home, ".bash_profile", "nodeman completion bash")
		}
		return checkProfileContains(home, ".bashrc", "nodeman completion bash")
	case strings.HasSuffix(shell, "/fish"):
		fishFile := filepath.Join(home, ".config", "fish", "completions", "nodeman.fish")
		if _, err := os.Stat(fishFile); err == nil {
			return []checkResult{{"Completions", true, fishFile}}
		}
		return []checkResult{{"Completions", false, "missing — run 'nodeman setup'"}}
	default:
		return []checkResult{{"Completions", false, fmt.Sprintf("unknown shell %q — configure manually", shell)}}
	}
}

func checkPowerShellCompletions(home string) []checkResult {
	profiles := []string{
		filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1"),
		filepath.Join(home, "Documents", "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1"),
	}

	for _, profilePath := range profiles {
		content, err := os.ReadFile(profilePath)
		if err != nil {
			continue
		}

		if strings.Contains(string(content), "nodeman completion powershell") {
			return []checkResult{{"Completions", true, fmt.Sprintf("configured in %s", profilePath)}}
		}
	}

	for _, profilePath := range profiles {
		if _, err := os.Stat(profilePath); err == nil {
			return []checkResult{{"Completions", false, fmt.Sprintf("not found in %s — run 'nodeman setup'", profilePath)}}
		}
	}

	return []checkResult{{"Completions", false, "not found in PowerShell profile — run 'nodeman setup'"}}
}

func checkProfileContains(home, profileName, needle string) []checkResult {
	profilePath := filepath.Join(home, profileName)
	content, err := os.ReadFile(profilePath)
	if err != nil {
		return []checkResult{{"Completions", false, fmt.Sprintf("%s not found — run 'nodeman setup'", profileName)}}
	}
	if strings.Contains(string(content), needle) {
		return []checkResult{{"Completions", true, fmt.Sprintf("configured in ~/%s", profileName)}}
	}
	return []checkResult{{"Completions", false, fmt.Sprintf("not found in ~/%s — run 'nodeman setup'", profileName)}}
}

func checkWindowsUserPath(shimsDir string) checkResult {
	out, err := exec.Command("powershell", "-NoProfile", "-Command", `[Environment]::GetEnvironmentVariable('Path', 'User')`).Output()
	if err != nil {
		return checkResult{"User PATH", false, "cannot read user PATH — run 'nodeman setup'"}
	}

	userPath := strings.TrimSpace(string(out))
	for _, entry := range strings.Split(userPath, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.EqualFold(filepath.Clean(entry), filepath.Clean(shimsDir)) {
			return checkResult{"User PATH", true, "nodeman shims persisted for GUI apps (including VS Code)"}
		}
	}

	return checkResult{"User PATH", false, "nodeman shims missing from user PATH — run 'nodeman setup' then restart VS Code"}
}
