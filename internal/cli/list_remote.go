package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/roen/nodeman/internal/versions"
	"github.com/spf13/cobra"
)

const defaultMinMajor = 18

func newListRemoteCmd() *cobra.Command {
	var ltsOnly bool
	var limit int
	var noCache bool
	var full bool

	cmd := &cobra.Command{
		Use:     "ls-remote",
		Aliases: []string{"list-remote"},
		Short:   "List available Node.js versions from nodejs.org",
		Long: `List available Node.js versions from nodejs.org.

By default, shows the latest version in each major release (18+).
Use --full to show all individual versions, or a positional argument
to filter by major version.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var remote []versions.RemoteVersion
			var err error
			if noCache {
				remote, err = versions.FetchRemoteVersionsNoCache()
			} else {
				remote, err = versions.FetchRemoteVersions()
			}
			if err != nil {
				return err
			}

			// When a specific version prefix is given, show all matches (like --full)
			hasVersionArg := len(args) > 0

			// Apply filters
			var filtered []versions.RemoteVersion
			for _, v := range remote {
				if ltsOnly && !v.IsLTS() {
					continue
				}
				num := v.VersionNumber()

				// If positional arg given, filter by version prefix
				if hasVersionArg {
					prefix := strings.TrimPrefix(args[0], "v")
					if num != prefix && !strings.HasPrefix(num, prefix+".") {
						continue
					}
				} else if !full {
					// Default mode: only show major >= 18
					major := parseMajor(num)
					if major < defaultMinMajor {
						continue
					}
				}

				filtered = append(filtered, v)
			}

			if len(filtered) == 0 {
				fmt.Println("No matching versions found.")
				return nil
			}

			// Default mode: collapse to latest per major version
			if !full && !hasVersionArg {
				filtered = latestPerMajor(filtered)
			}

			// Apply limit
			if limit > 0 && limit < len(filtered) {
				filtered = filtered[:limit]
			}

			for _, v := range filtered {
				ltsInfo := ""
				if v.IsLTS() {
					ltsInfo = fmt.Sprintf(" (LTS: %s)", v.LTSName())
				}
				fmt.Printf("  %s%s\n", v.VersionNumber(), ltsInfo)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&ltsOnly, "lts", false, "Show only LTS versions")
	cmd.Flags().BoolVar(&full, "full", false, "Show all versions instead of latest per major")
	cmd.Flags().IntVarP(&limit, "limit", "n", 0, "Maximum number of versions to display (0 = no limit)")
	cmd.Flags().BoolVar(&noCache, "no-cache", false, "Bypass the version cache and fetch fresh data")
	return cmd
}

// parseMajor extracts the major version number from a version string like "22.14.0".
func parseMajor(version string) int {
	parts := strings.SplitN(version, ".", 2)
	n, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}
	return n
}

// latestPerMajor returns only the first (latest) version for each major version.
// Input must be sorted newest-first (as returned by nodejs.org).
func latestPerMajor(all []versions.RemoteVersion) []versions.RemoteVersion {
	seen := map[int]bool{}
	var result []versions.RemoteVersion
	for _, v := range all {
		major := parseMajor(v.VersionNumber())
		if !seen[major] {
			seen[major] = true
			result = append(result, v)
		}
	}
	return result
}
