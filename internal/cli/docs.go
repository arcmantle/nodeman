package cli

import (
	"fmt"
	"strings"

	nodeman "github.com/arcmantle/nodeman"
	"github.com/arcmantle/nodeman/internal/platform"
	"github.com/arcmantle/rembed"
	"github.com/spf13/cobra"
)

func newDocsCmd(version string) *cobra.Command {
	var noOpen bool
	var force bool

	cmd := &cobra.Command{
		Use:   "docs",
		Short: "Render and open local documentation",
		Long: `Render local nodeman documentation from the embedded README and open it
in your default browser.

The generated file is versioned and written to:
  ~/.nodeman/docs/<version>/index.html`,
		RunE: func(cmd *cobra.Command, args []string) error {
			rootDir, err := platform.RootDir()
			if err != nil {
				return err
			}

			ref := "master"
			if strings.HasPrefix(version, "v") {
				ref = version
			}

			docPath, err := rembed.WriteDocsWithOptions(
				rootDir,
				string(nodeman.EmbeddedREADME),
				rembed.WriteOptions{
					Version:    version,
					Title:      "nodeman Documentation",
					SourcePath: "embedded README.md",
					Force:      force,
					InlineAssets: map[string]rembed.InlineAsset{
						"assets/logo.svg": {
							Data:     nodeman.EmbeddedLogoSVG,
							MIMEType: "image/svg+xml",
						},
					},
					LinkBaseURL: rembed.GitHubRawBaseURL("arcmantle", "nodeman", ref),
				},
			)

			if err != nil {
				return err
			}

			fmt.Println("Documentation file:", docPath)
			if noOpen {
				fmt.Println("Open disabled with --no-open")
				return nil
			}

			if err := rembed.OpenInBrowser(docPath); err != nil {
				return fmt.Errorf("opening docs in browser: %w", err)
			}
			fmt.Println("Opened documentation in your default browser.")
			return nil
		},
	}

	cmd.Flags().BoolVar(&noOpen, "no-open", false, "Generate docs but do not open browser")
	cmd.Flags().BoolVar(&force, "force", false, "Regenerate docs even if file already exists")
	return cmd
}
