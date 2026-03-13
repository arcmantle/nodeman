package shim

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/roen/nodeman/internal/platform"
)

// windowsShimExts are file extensions that nodeman treats as runnable shim targets.
var windowsShimExts = []string{".exe", ".cmd"}

// normalizeShimName extracts a canonical shim name from argv0 or a filename.
// On Windows it strips known executable suffixes repeatedly so polluted names
// like "npm.cmd.exe" normalize to "npm".
func normalizeShimName(name string) string {
	base := filepath.Base(name)
	if base == "" {
		return ""
	}

	if runtime.GOOS != "windows" {
		return base
	}

	base = strings.ToLower(base)
	for {
		trimmed := false
		for _, ext := range windowsShimExts {
			if strings.HasSuffix(base, ext) {
				base = strings.TrimSuffix(base, ext)
				trimmed = true
				break
			}
		}
		if !trimmed {
			break
		}
	}

	return base
}

// shimNameFromBinEntry returns the logical command name from a bin entry.
// On Windows we only accept .exe/.cmd entries to avoid shimming docs and
// helper scripts from Node distributions (e.g. README.md, install_tools.bat).
func shimNameFromBinEntry(entryName string) (string, bool) {
	if runtime.GOOS != "windows" {
		if entryName == "" {
			return "", false
		}
		return entryName, true
	}

	name := strings.ToLower(entryName)
	for _, ext := range windowsShimExts {
		if strings.HasSuffix(name, ext) {
			base := strings.TrimSuffix(name, ext)
			if base == "" {
				return "", false
			}
			return base, true
		}
	}

	return "", false
}

// resolveShimTarget returns the real executable/script path for a shim name.
func resolveShimTarget(binDir, shimName string) (string, error) {
	if runtime.GOOS == "windows" {
		for _, ext := range windowsShimExts {
			candidate := filepath.Join(binDir, shimName+ext)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
		}
		return "", fmt.Errorf("%s not found for active Node.js version in %s", shimName, binDir)
	}

	candidate := filepath.Join(binDir, shimName+platform.ExeSuffix())
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		return candidate, nil
	}
	return "", fmt.Errorf("%s not found for active Node.js version in %s", shimName, binDir)
}
