package cli

import (
	"fmt"
	"strings"

	nodeman "github.com/arcmantle/nodeman"
	"github.com/arcmantle/nodeman/embeddedreadme"
	"github.com/arcmantle/nodeman/internal/platform"
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

			markdown := rewriteDocsMarkdown(nodeman.EmbeddedREADME, version)
			docPath, err := embeddedreadme.WriteVersionedDocs(
				rootDir,
				version,
				markdown,
				"nodeman Documentation",
				"embedded README.md",
				force,
			)
			if err != nil {
				return err
			}

			fmt.Println("Documentation file:", docPath)
			if noOpen {
				fmt.Println("Open disabled with --no-open")
				return nil
			}

			if err := embeddedreadme.OpenInBrowser(docPath); err != nil {
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

func rewriteDocsMarkdown(markdown []byte, version string) []byte {
	ref := "master"
	if strings.HasPrefix(version, "v") {
		ref = version
	}

	logoURL := fmt.Sprintf("https://raw.githubusercontent.com/arcmantle/nodeman/%s/assets/logo.svg", ref)
	content := strings.ReplaceAll(string(markdown), `src="assets/logo.svg"`, fmt.Sprintf(`src="%s"`, logoURL))
	return []byte(content)
}
