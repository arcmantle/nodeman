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
// shimName is one of: "node", "npm", "npx", "corepack", or any globally installed binary.
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
	binaryPath, err := resolveShimTarget(binDir, shimName)
	if err != nil {
		return err
	}

	// If this is npm/npx with a global install/uninstall, run as a child process
	// so we can sync shims afterward.
	if isGlobalNpmCommand(shimName, args) {
		return execAndSync(binaryPath, args)
	}

	if runtime.GOOS == "windows" {
		return execWindows(binaryPath, args)
	}
	return execUnix(binaryPath, args)
}

// isGlobalNpmCommand checks if this is an npm/npx command that modifies global packages.
func isGlobalNpmCommand(shimName string, args []string) bool {
	if shimName != "npm" && shimName != "npx" {
		return false
	}
	hasGlobalFlag := false
	hasInstallCmd := false
	for _, arg := range args[1:] {
		if arg == "-g" || arg == "--global" {
			hasGlobalFlag = true
		}
		if arg == "install" || arg == "i" || arg == "add" ||
			arg == "uninstall" || arg == "remove" || arg == "rm" || arg == "un" {
			hasInstallCmd = true
		}
		// Stop scanning at -- separator
		if arg == "--" {
			break
		}
	}
	return hasGlobalFlag && hasInstallCmd
}

// execAndSync runs the binary as a child process and syncs shims after it completes.
func execAndSync(binaryPath string, args []string) error {
	cmd := commandForBinary(binaryPath, args[1:])
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()

	// Sync shims regardless of exit code — a partial install may have added binaries
	_, _, _ = SyncShims()

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

func execUnix(binaryPath string, args []string) error {
	argv := append([]string{binaryPath}, args[1:]...)
	return syscall.Exec(binaryPath, argv, os.Environ())
}

func execWindows(binaryPath string, args []string) error {
	cmd := commandForBinary(binaryPath, args[1:])
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

func commandForBinary(binaryPath string, args []string) *exec.Cmd {
	if runtime.GOOS == "windows" && strings.EqualFold(filepath.Ext(binaryPath), ".cmd") {
		return exec.Command("cmd.exe", append([]string{"/c", binaryPath}, args...)...)
	}
	return exec.Command(binaryPath, args...)
}
