package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/roen/nodeman/internal/config"
	"github.com/roen/nodeman/internal/discover"
	"github.com/roen/nodeman/internal/platform"
	"github.com/roen/nodeman/internal/versions"
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
		suffix := platform.ExeSuffix()

		nodeBin := filepath.Join(binDir, "node"+suffix)
		if out, err := exec.Command(nodeBin, "--version").Output(); err == nil {
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

		npmBin := filepath.Join(binDir, "npm"+suffix)
		if out, err := exec.Command(npmBin, "--version").Output(); err == nil {
			ver := strings.TrimSpace(string(out))
			results = append(results, checkResult{"npm --version", true, fmt.Sprintf("%s (bundled with node %s)", ver, cfg.ActiveVersion)})
		} else {
			results = append(results, checkResult{"npm --version", false,
				fmt.Sprintf("cannot execute %s: %s", npmBin, err)})
		}
	}

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
