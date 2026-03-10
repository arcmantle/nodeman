package shim

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/roen/nodeman/internal/discover"
	"github.com/roen/nodeman/internal/platform"
)

// Setup creates shim binaries in ~/.nodeman/shims/.
// It copies (or hardlinks) the currently running nodeman binary as node, npm, npx, corepack.
// It also scans for existing Node.js installations and validates PATH ordering.
func Setup() error {
	if err := platform.EnsureDirs(); err != nil {
		return err
	}

	shimsDir, err := platform.ShimsDir()
	if err != nil {
		return err
	}

	// Find the currently running binary
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot find own executable: %w", err)
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}

	suffix := platform.ExeSuffix()
	for _, name := range platform.ShimNames() {
		shimPath := filepath.Join(shimsDir, name+suffix)

		// Remove existing shim
		os.Remove(shimPath)

		// Try hardlink first (most efficient), fall back to copy
		if err := os.Link(self, shimPath); err != nil {
			if err := copyFile(self, shimPath); err != nil {
				return fmt.Errorf("creating shim %s: %w", name, err)
			}
		}
	}

	// Also create a nodeman symlink/copy in the shims dir for convenience
	nodeman := filepath.Join(shimsDir, "nodeman"+suffix)
	os.Remove(nodeman)
	if err := os.Link(self, nodeman); err != nil {
		if err := copyFile(self, nodeman); err != nil {
			return fmt.Errorf("creating nodeman shim: %w", err)
		}
	}

	fmt.Println("Shims created in", shimsDir)

	// Detect existing Node.js installations
	reportExistingInstalls()

	// Validate PATH
	reportPathStatus(shimsDir)

	printPathInstructions(shimsDir)
	return nil
}

// reportExistingInstalls scans for Node.js installations outside nodeman and prints them.
func reportExistingInstalls() {
	installations, err := discover.FindAll()
	if err != nil {
		return
	}

	var external []discover.Installation
	for _, inst := range installations {
		if inst.Source != "nodeman" {
			external = append(external, inst)
		}
	}

	if len(external) == 0 {
		return
	}

	fmt.Println()
	fmt.Println("Detected existing Node.js installations:")
	for _, inst := range external {
		fmt.Printf("  - v%s (%s) at %s\n", inst.Version, inst.Source, inst.Path)
	}
	fmt.Println()
	fmt.Println("Run 'nodeman adopt' to import them into nodeman.")
	fmt.Println("After adopting, you can uninstall the originals (e.g. 'brew uninstall node').")
}

// reportPathStatus checks if the shims directory is in PATH and in the correct position.
func reportPathStatus(shimsDir string) {
	inPath, isFirst, otherDirs := discover.ShimsInPath()

	fmt.Println()
	if inPath && isFirst {
		fmt.Println("PATH: OK — nodeman shims take priority.")
	} else if inPath && !isFirst {
		fmt.Println("WARNING: nodeman shims are in PATH but another Node.js appears first:")
		for _, d := range otherDirs {
			fmt.Printf("  - %s\n", d)
		}
		fmt.Println()
		fmt.Printf("Make sure %s appears BEFORE these directories in your PATH.\n", shimsDir)
	} else {
		fmt.Println("NOTE: nodeman shims are not yet in your PATH.")
		if len(otherDirs) > 0 {
			fmt.Println("Other Node.js directories found in PATH:")
			for _, d := range otherDirs {
				fmt.Printf("  - %s\n", d)
			}
		}
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func printPathInstructions(shimsDir string) {
	fmt.Println()
	fmt.Println("Add the shims directory to your PATH (if not already done):")
	fmt.Println()

	switch runtime.GOOS {
	case "windows":
		fmt.Printf("  setx PATH \"%s;%%PATH%%\"\n", shimsDir)
	default:
		fmt.Printf("  # Add to your shell profile (~/.bashrc, ~/.zshrc, etc.):\n")
		fmt.Printf("  export PATH=\"%s:$PATH\"\n", shimsDir)
	}
	fmt.Println()
}
