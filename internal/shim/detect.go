package shim

import (
	"path/filepath"

	"github.com/arcmantle/nodeman/internal/config"
	"github.com/arcmantle/nodeman/internal/platform"
)

// shimBinaries is the set of binary names that nodeman always provides shims for.
var shimBinaries = map[string]bool{
	"node":     true,
	"npm":      true,
	"npx":      true,
	"corepack": true,
}

// DetectShim checks whether the process was invoked as one of the shimmed
// binary names. Returns the shim name (e.g. "node", "pnpm") or "" if not a shim.
// It matches the core shims (node, npm, npx, corepack) plus any binary
// that exists in the active Node.js version's bin directory.
func DetectShim(argv0 string) string {
	base := normalizeShimName(argv0)
	if base == "" {
		return ""
	}

	// Always match core shims
	if shimBinaries[base] {
		return base
	}

	// Don't intercept "nodeman" itself
	if base == "nodeman" {
		return ""
	}

	// Check if this name exists as a binary in the active version's bin dir
	cfg, err := config.Load()
	if err != nil || cfg.ActiveVersion == "" {
		return ""
	}

	versionsDir, err := platform.VersionsDir()
	if err != nil {
		return ""
	}

	binDir := platform.BinDir(filepath.Join(versionsDir, cfg.ActiveVersion))
	if _, err := resolveShimTarget(binDir, base); err == nil {
		return base
	}

	return ""
}
