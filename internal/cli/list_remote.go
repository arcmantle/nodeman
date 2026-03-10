package cli

import (
	"fmt"
	"strings"

	"github.com/roen/nodeman/internal/versions"
	"github.com/spf13/cobra"
)

func newListRemoteCmd() *cobra.Command {
	var ltsOnly bool
	var limit int
	var noCache bool

	cmd := &cobra.Command{
		Use:     "ls-remote",
		Aliases: []string{"list-remote"},
		Short:   "List available Node.js versions from nodejs.org",
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

			// Apply filters
			var filtered []versions.RemoteVersion
			for _, v := range remote {
				if ltsOnly && !v.IsLTS() {
					continue
				}
				// If positional arg given, filter by major version prefix
				if len(args) > 0 {
					prefix := strings.TrimPrefix(args[0], "v")
					num := v.VersionNumber()
					if num != prefix && !strings.HasPrefix(num, prefix+".") {
						continue
					}
				}
				filtered = append(filtered, v)
			}

			if len(filtered) == 0 {
				fmt.Println("No matching versions found.")
				return nil
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
	cmd.Flags().IntVarP(&limit, "limit", "n", 20, "Maximum number of versions to display")
	cmd.Flags().BoolVar(&noCache, "no-cache", false, "Bypass the version cache and fetch fresh data")
	return cmd
}
