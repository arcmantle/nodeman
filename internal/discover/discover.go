package discover

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/roen/nodeman/internal/platform"
)

// Installation represents a discovered Node.js installation on the system.
type Installation struct {
	Path    string // Full path to the node binary
	Version string // Version string without "v" prefix (e.g. "22.14.0")
	Source  string // Detected source (e.g. "Homebrew", "nvm", "system", "unknown")
	BinDir  string // Directory containing the node binary
	RootDir string // Root of the Node.js installation
}

// FindAll scans the system for existing Node.js installations outside of nodeman.
func FindAll() ([]Installation, error) {
	shimsDir, _ := platform.ShimsDir()
	seen := map[string]bool{}
	var installations []Installation

	// Strategy 1: which/where node — find anything on the current PATH
	if paths, err := findOnPath(shimsDir); err == nil {
		for _, inst := range paths {
			key := filepath.Clean(inst.Path)
			if !seen[key] {
				seen[key] = true
				installations = append(installations, inst)
			}
		}
	}

	// Strategy 2: Check well-known install locations
	for _, inst := range checkKnownPaths(shimsDir) {
		key := filepath.Clean(inst.Path)
		if !seen[key] {
			seen[key] = true
			installations = append(installations, inst)
		}
	}

	return installations, nil
}

// findOnPath uses the system PATH to locate node binaries.
func findOnPath(shimsDir string) ([]Installation, error) {
	var results []Installation

	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		return nil, nil
	}

	suffix := platform.ExeSuffix()
	for _, dir := range filepath.SplitList(pathEnv) {
		// Skip nodeman's own shims directory
		if shimsDir != "" && filepath.Clean(dir) == filepath.Clean(shimsDir) {
			continue
		}

		nodePath := filepath.Join(dir, "node"+suffix)
		if info, err := os.Stat(nodePath); err == nil && !info.IsDir() {
			if inst, err := inspectNode(nodePath); err == nil {
				results = append(results, inst)
			}
		}
	}

	return results, nil
}

// checkKnownPaths probes well-known Node.js installation locations.
func checkKnownPaths(shimsDir string) []Installation {
	var candidates []string

	home := os.Getenv("HOME")

	switch runtime.GOOS {
	case "darwin":
		candidates = append(candidates,
			"/usr/local/bin/node",
			"/opt/homebrew/bin/node",
			"/usr/local/opt/node/bin/node",
			"/opt/homebrew/opt/node/bin/node",
		)
		if home != "" {
			candidates = append(candidates,
				filepath.Join(home, ".nvm/current/bin/node"),
				filepath.Join(home, ".volta/bin/node"),
			)
			// Check all nvm versions
			nvmDir := filepath.Join(home, ".nvm/versions/node")
			if entries, err := os.ReadDir(nvmDir); err == nil {
				for _, e := range entries {
					if e.IsDir() {
						candidates = append(candidates, filepath.Join(nvmDir, e.Name(), "bin/node"))
					}
				}
			}
			// Check fnm versions
			fnmDir := filepath.Join(home, ".fnm/node-versions")
			if entries, err := os.ReadDir(fnmDir); err == nil {
				for _, e := range entries {
					if e.IsDir() {
						candidates = append(candidates, filepath.Join(fnmDir, e.Name(), "installation/bin/node"))
					}
				}
			}
		}
	case "linux":
		candidates = append(candidates,
			"/usr/bin/node",
			"/usr/local/bin/node",
			"/snap/bin/node",
		)
		if home != "" {
			candidates = append(candidates,
				filepath.Join(home, ".nvm/current/bin/node"),
				filepath.Join(home, ".volta/bin/node"),
			)
			// Check all nvm versions
			nvmDir := filepath.Join(home, ".nvm/versions/node")
			if entries, err := os.ReadDir(nvmDir); err == nil {
				for _, e := range entries {
					if e.IsDir() {
						candidates = append(candidates, filepath.Join(nvmDir, e.Name(), "bin/node"))
					}
				}
			}
		}
	case "windows":
		programFiles := os.Getenv("ProgramFiles")
		if programFiles == "" {
			programFiles = `C:\Program Files`
		}
		candidates = append(candidates,
			filepath.Join(programFiles, "nodejs", "node.exe"),
		)
		appData := os.Getenv("APPDATA")
		if appData != "" {
			candidates = append(candidates,
				filepath.Join(appData, "nvm", "current", "node.exe"),
			)
		}
	}

	var results []Installation
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			// Skip if it's inside nodeman's shims
			if shimsDir != "" && filepath.Clean(filepath.Dir(candidate)) == filepath.Clean(shimsDir) {
				continue
			}
			if inst, err := inspectNode(candidate); err == nil {
				results = append(results, inst)
			}
		}
	}

	return results
}

// inspectNode runs a node binary to determine its version and source.
func inspectNode(nodePath string) (Installation, error) {
	// Resolve symlinks to find the real binary
	resolved, err := filepath.EvalSymlinks(nodePath)
	if err != nil {
		resolved = nodePath
	}

	// Get version
	out, err := exec.Command(nodePath, "--version").Output()
	if err != nil {
		return Installation{}, fmt.Errorf("cannot execute %s: %w", nodePath, err)
	}
	version := strings.TrimSpace(string(out))
	version = strings.TrimPrefix(version, "v")

	binDir := filepath.Dir(resolved)
	rootDir := installationRoot(binDir)

	source := identifySource(resolved)

	return Installation{
		Path:    resolved,
		Version: version,
		Source:  source,
		BinDir:  binDir,
		RootDir: rootDir,
	}, nil
}

// identifySource guesses where a Node installation came from based on its path.
func identifySource(resolvedPath string) string {
	p := strings.ToLower(resolvedPath)
	// Normalize separators so source checks work on Windows and Unix paths.
	p = strings.ReplaceAll(p, "\\", "/")

	switch {
	case strings.Contains(p, "homebrew") || strings.Contains(p, "/opt/homebrew/") || strings.Contains(p, "/usr/local/cellar/"):
		return "Homebrew"
	case strings.Contains(p, "/.nvm/") || strings.Contains(p, "/nvm/"):
		return "nvm"
	case strings.Contains(p, "/.fnm/") || strings.Contains(p, "/fnm/"):
		return "fnm"
	case strings.Contains(p, "/.volta/") || strings.Contains(p, "/volta/"):
		return "Volta"
	case strings.Contains(p, ".nodeman/"):
		return "nodeman"
	case strings.Contains(p, "/snap/"):
		return "Snap"
	case strings.Contains(p, "program files") || strings.Contains(p, "programfiles"):
		return "official installer"
	case strings.Contains(p, "/usr/bin/") || strings.Contains(p, "/usr/local/bin/"):
		return "system"
	default:
		return "unknown"
	}
}

func installationRoot(binDir string) string {
	// On Windows, node.exe is usually at the installation root (no bin/ folder).
	if runtime.GOOS == "windows" {
		base := strings.ToLower(filepath.Base(binDir))
		if base != "bin" {
			return binDir
		}
	}

	return filepath.Dir(binDir)
}

// ShimsInPath checks whether the nodeman shims directory is in PATH,
// and whether it appears before any detected Node installations.
// Returns: inPath, isFirstNode, existingNodeDirs
func ShimsInPath() (inPath bool, isFirstNode bool, existingNodeDirs []string) {
	shimsDir, err := platform.ShimsDir()
	if err != nil {
		return false, false, nil
	}

	suffix := platform.ExeSuffix()
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		return false, false, nil
	}

	dirs := filepath.SplitList(pathEnv)
	shimsClean := filepath.Clean(shimsDir)

	foundShims := false
	foundOtherNodeFirst := false

	for _, dir := range dirs {
		clean := filepath.Clean(dir)
		if clean == shimsClean {
			foundShims = true
			continue
		}
		// Check if this dir contains a node binary
		if info, err := os.Stat(filepath.Join(dir, "node"+suffix)); err == nil && !info.IsDir() {
			existingNodeDirs = append(existingNodeDirs, dir)
			if !foundShims {
				foundOtherNodeFirst = true
			}
		}
	}

	return foundShims, foundShims && !foundOtherNodeFirst, existingNodeDirs
}
