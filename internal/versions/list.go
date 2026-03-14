package versions

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/arcmantle/nodeman/internal/platform"
)

// InstalledVersion holds info about a locally installed Node.js version.
type InstalledVersion struct {
	Version string // e.g., "22.14.0" (without "v" prefix)
	Path    string // full path to the version directory
}

// ListInstalled scans ~/.nodeman/versions/ and returns all installed versions,
// sorted with newest first.
func ListInstalled() ([]InstalledVersion, error) {
	dir, err := platform.VersionsDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading versions directory: %w", err)
	}

	var installed []InstalledVersion
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// Verify that the directory contains a node binary
		binDir := platform.BinDir(filepath.Join(dir, name))
		nodeBin := filepath.Join(binDir, "node"+platform.ExeSuffix())
		if _, err := os.Stat(nodeBin); err != nil {
			continue // skip directories that don't look like Node installations
		}
		installed = append(installed, InstalledVersion{
			Version: name,
			Path:    filepath.Join(dir, name),
		})
	}

	// Sort newest first using a simple string comparison on version segments.
	sort.Slice(installed, func(i, j int) bool {
		return compareVersions(installed[i].Version, installed[j].Version) > 0
	})

	return installed, nil
}

// IsInstalled checks if a given version (without "v" prefix) is installed.
func IsInstalled(version string) (bool, error) {
	dir, err := platform.VersionsDir()
	if err != nil {
		return false, err
	}
	binDir := platform.BinDir(filepath.Join(dir, version))
	nodeBin := filepath.Join(binDir, "node"+platform.ExeSuffix())
	_, err = os.Stat(nodeBin)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// compareVersions compares two version strings (e.g., "22.14.0" vs "20.18.3").
// Returns positive if a > b, negative if a < b, 0 if equal.
// Handles pre-release suffixes: numeric parts are extracted and compared,
// and segments with non-numeric suffixes (e.g. "0-rc1") sort before pure numeric ones.
func compareVersions(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	maxLen := len(aParts)
	if len(bParts) > maxLen {
		maxLen = len(bParts)
	}
	for i := 0; i < maxLen; i++ {
		var aPart, bPart string
		if i < len(aParts) {
			aPart = aParts[i]
		}
		if i < len(bParts) {
			bPart = bParts[i]
		}
		aNum, aExtra := parseVersionPart(aPart)
		bNum, bExtra := parseVersionPart(bPart)
		if aNum != bNum {
			return aNum - bNum
		}
		// Same number: a pre-release suffix (e.g. "-rc1") sorts before no suffix
		if aExtra != bExtra {
			if aExtra && !bExtra {
				return -1
			}
			return 1
		}
	}
	return 0
}

// parseVersionPart extracts the leading integer from a version segment
// and reports whether there is a non-numeric suffix (pre-release indicator).
func parseVersionPart(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	// Find where the digits end
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, true // entirely non-numeric
	}
	num, _ := strconv.Atoi(s[:i])
	hasExtra := i < len(s) // there's a suffix like "-rc1" or "beta2"
	return num, hasExtra
}
