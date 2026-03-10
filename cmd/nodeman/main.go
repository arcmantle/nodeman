package main

import (
	"fmt"
	"os"

	"github.com/roen/nodeman/internal/cli"
	"github.com/roen/nodeman/internal/shim"
)

var version = "dev"

func main() {
	// Check if we're being invoked as a shim (node, npm, npx, corepack)
	shimName := shim.DetectShim(os.Args[0])
	if shimName != "" {
		if err := shim.Exec(shimName, os.Args); err != nil {
			fmt.Fprintf(os.Stderr, "nodeman: %s\n", err)
			os.Exit(1)
		}
		return // unreachable on Unix (syscall.Exec replaces process)
	}

	// Run as the nodeman CLI
	root := cli.NewRootCmd(version)
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
