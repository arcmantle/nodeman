package platform

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// RootDir returns the nodeman data directory (~/.nodeman).
func RootDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".nodeman"), nil
}

// VersionsDir returns the path to ~/.nodeman/versions.
func VersionsDir() (string, error) {
	root, err := RootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "versions"), nil
}

// ShimsDir returns the path to ~/.nodeman/shims.
func ShimsDir() (string, error) {
	root, err := RootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "shims"), nil
}

// EnsureDirs creates all required nodeman directories if they don't exist.
func EnsureDirs() error {
	root, err := RootDir()
	if err != nil {
		return err
	}
	dirs := []string{
		root,
		filepath.Join(root, "versions"),
		filepath.Join(root, "shims"),
		filepath.Join(root, "cache"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("cannot create directory %s: %w", d, err)
		}
	}
	return nil
}

// NodeOS returns the Node.js dist OS name for the current platform.
func NodeOS() string {
	switch runtime.GOOS {
	case "darwin":
		return "darwin"
	case "linux":
		return "linux"
	case "windows":
		return "win"
	default:
		return runtime.GOOS
	}
}

// NodeArch returns the Node.js dist architecture name for the current platform.
func NodeArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "arm64":
		return "arm64"
	default:
		return runtime.GOARCH
	}
}

// ArchiveExt returns the expected archive extension for the current OS.
func ArchiveExt() string {
	if runtime.GOOS == "windows" {
		return "zip"
	}
	if runtime.GOOS == "linux" {
		return "tar.xz"
	}
	return "tar.gz"
}

// DownloadURL returns the full URL for a given Node.js version string (e.g. "v22.14.0").
func DownloadURL(version string) string {
	return fmt.Sprintf("https://nodejs.org/dist/%s/node-%s-%s-%s.%s",
		version, version, NodeOS(), NodeArch(), ArchiveExt())
}

// ChecksumsURL returns the SHASUMS256.txt URL for a given Node.js version.
func ChecksumsURL(version string) string {
	return fmt.Sprintf("https://nodejs.org/dist/%s/SHASUMS256.txt", version)
}

// ArchiveFilename returns just the filename portion of the download.
func ArchiveFilename(version string) string {
	return fmt.Sprintf("node-%s-%s-%s.%s", version, NodeOS(), NodeArch(), ArchiveExt())
}

// BinDir returns the path to the bin directory inside an installed Node.js version.
// On Windows, binaries are at the root of the extracted folder; on Unix, they're in bin/.
func BinDir(versionDir string) string {
	if runtime.GOOS == "windows" {
		return versionDir
	}
	return filepath.Join(versionDir, "bin")
}

// ExeSuffix returns ".exe" on Windows, "" otherwise.
func ExeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// ResolveBinCommand resolves a command inside a Node bin directory.
// On Windows it supports both .exe and .cmd launchers.
func ResolveBinCommand(binDir, name string) (string, error) {
	if runtime.GOOS == "windows" {
		for _, ext := range []string{".exe", ".cmd"} {
			candidate := filepath.Join(binDir, name+ext)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
		}
		return "", fmt.Errorf("%s not found in %s", name, binDir)
	}

	candidate := filepath.Join(binDir, name+ExeSuffix())
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		return candidate, nil
	}
	return "", fmt.Errorf("%s not found in %s", name, binDir)
}

// CommandForBinary creates an exec.Cmd for the given binary path.
// On Windows, .cmd targets are run via cmd.exe /c.
func CommandForBinary(binaryPath string, args ...string) *exec.Cmd {
	if runtime.GOOS == "windows" && strings.EqualFold(filepath.Ext(binaryPath), ".cmd") {
		return exec.Command("cmd.exe", append([]string{"/c", binaryPath}, args...)...)
	}
	return exec.Command(binaryPath, args...)
}

// ShimNames returns the names of binaries that should be shimmed.
func ShimNames() []string {
	return []string{"node", "npm", "npx", "corepack"}
}

// ExtractTarGz extracts a .tar.gz archive to destDir.
func ExtractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip open: %w", err)
	}
	defer gr.Close()

	return extractTar(gr, destDir)
}

// ExtractTarXz extracts a .tar.xz archive to destDir using the system xz command.
func ExtractTarXz(archivePath, destDir string) error {
	if _, err := exec.LookPath("xz"); err != nil {
		return fmt.Errorf("xz command not found: install xz-utils (apt) or xz (brew/dnf) to extract .tar.xz archives")
	}
	cmd := exec.Command("xz", "-dc", archivePath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("xz pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("xz start: %w", err)
	}
	if err := extractTar(stdout, destDir); err != nil {
		return err
	}
	return cmd.Wait()
}

// ExtractZip extracts a .zip archive to destDir.
func ExtractZip(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		target := filepath.Join(destDir, filepath.FromSlash(f.Name))
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("zip entry attempts path traversal: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return err
		}
		out.Close()
		rc.Close()
	}
	return nil
}

func extractTar(r io.Reader, destDir string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}

		target := filepath.Join(destDir, filepath.FromSlash(hdr.Name))
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("tar entry attempts path traversal: %s", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			linkTarget := hdr.Linkname
			if filepath.IsAbs(linkTarget) {
				return fmt.Errorf("tar symlink has absolute target: %s -> %s", hdr.Name, linkTarget)
			}
			resolved := filepath.Join(filepath.Dir(target), linkTarget)
			if !strings.HasPrefix(filepath.Clean(resolved), filepath.Clean(destDir)+string(os.PathSeparator)) {
				return fmt.Errorf("tar symlink escapes destination: %s -> %s", hdr.Name, linkTarget)
			}
			os.Remove(target)
			if err := os.Symlink(linkTarget, target); err != nil {
				return err
			}
		}
	}
	return nil
}
