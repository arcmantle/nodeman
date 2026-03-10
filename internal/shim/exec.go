package shim

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/roen/nodeman/internal/config"
	"github.com/roen/nodeman/internal/platform"
	"github.com/roen/nodeman/internal/versions"
)

// Exec replaces the current process with the real binary for the given shim name.
// shimName is one of: "node", "npm", "npx", "corepack".
func Exec(shimName string, args []string) error {
	// Check for NODEMAN_VERSION env override
	activeVersion := os.Getenv("NODEMAN_VERSION")
	if activeVersion != "" {
		// Resolve partial version (e.g. "22") against installed versions
		installed, err := versions.ListInstalled()
		if err != nil {
			return fmt.Errorf("listing installed versions: %w", err)
		}
		v := strings.TrimPrefix(activeVersion, "v")
		var matched string
		for _, iv := range installed {
			if iv.Version == v || strings.HasPrefix(iv.Version, v+".") {
				matched = iv.Version
				break
			}
		}
		if matched == "" {
			return fmt.Errorf("NODEMAN_VERSION=%s: no matching installed version found", activeVersion)
		}
		activeVersion = matched
	} else {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		if cfg.ActiveVersion == "" {
			return fmt.Errorf("no active Node.js version set. Run: nodeman use <version>")
		}
		activeVersion = cfg.ActiveVersion
	}

	versionsDir, err := platform.VersionsDir()
	if err != nil {
		return err
	}

	binDir := platform.BinDir(filepath.Join(versionsDir, activeVersion))
	binaryPath := filepath.Join(binDir, shimName+platform.ExeSuffix())

	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return fmt.Errorf("%s not found for Node.js %s at %s", shimName, activeVersion, binaryPath)
	}

	if runtime.GOOS == "windows" {
		return execWindows(binaryPath, args)
	}
	return execUnix(binaryPath, args)
}

func execUnix(binaryPath string, args []string) error {
	argv := append([]string{binaryPath}, args[1:]...)
	return syscall.Exec(binaryPath, argv, os.Environ())
}

func execWindows(binaryPath string, args []string) error {
	cmd := exec.Command(binaryPath, args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "nodeman: %s\n", err)
		os.Exit(1)
	}
	os.Exit(0)
	return nil // unreachable
}
