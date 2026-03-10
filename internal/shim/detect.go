package shim

import (
	"path/filepath"
	"strings"
)

// shimBinaries is the set of binary names that nodeman provides shims for.
var shimBinaries = map[string]bool{
	"node":     true,
	"npm":      true,
	"npx":      true,
	"corepack": true,
}

// DetectShim checks whether the process was invoked as one of the shimmed
// binary names. Returns the shim name (e.g. "node") or "" if not a shim.
func DetectShim(argv0 string) string {
	base := filepath.Base(argv0)
	base = strings.TrimSuffix(base, ".exe")
	if shimBinaries[base] {
		return base
	}
	return ""
}
