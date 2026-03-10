package cli

import (
	"fmt"

	"github.com/roen/nodeman/internal/config"
	"github.com/spf13/cobra"
)

func newCurrentCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "current",
		Short: "Show the active Node.js version",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			if cfg.ActiveVersion == "" {
				fmt.Println("No active version set. Run 'nodeman use <version>'.")
				return nil
			}

			fmt.Println(cfg.ActiveVersion)
			return nil
		},
	}
}
