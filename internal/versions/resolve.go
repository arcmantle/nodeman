package versions

import (
	"fmt"
	"strings"
)

// ResolveVersion resolves a user-provided version string to a full version tag (e.g., "v22.14.0").
// Accepts: "lts", "latest", "22", "22.14", "22.14.0", "v22.14.0"
func ResolveVersion(input string, remoteVersions []RemoteVersion) (string, error) {
	input = strings.TrimSpace(input)

	switch strings.ToLower(input) {
	case "lts":
		return resolveLatestLTS(remoteVersions)
	case "latest":
		if len(remoteVersions) == 0 {
			return "", fmt.Errorf("no remote versions available")
		}
		return remoteVersions[0].Version, nil
	}

	// Strip leading "v"
	query := strings.TrimPrefix(input, "v")

	// Try exact match first
	for _, v := range remoteVersions {
		if v.VersionNumber() == query {
			return v.Version, nil
		}
	}

	// Try prefix match (e.g., "22" matches "22.14.0", "22.14" matches "22.14.0")
	// The remote list is sorted newest first, so the first prefix match is the latest.
	prefix := query + "."
	for _, v := range remoteVersions {
		num := v.VersionNumber()
		if num == query || strings.HasPrefix(num, prefix) {
			return v.Version, nil
		}
	}

	return "", fmt.Errorf("no version matching %q found", input)
}

func resolveLatestLTS(versions []RemoteVersion) (string, error) {
	for _, v := range versions {
		if v.IsLTS() {
			return v.Version, nil
		}
	}
	return "", fmt.Errorf("no LTS version found")
}
