package versions

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/arcmantle/nodeman/internal/httputil"
	"github.com/arcmantle/nodeman/internal/platform"
)

// Install downloads, verifies, and extracts a Node.js version.
// version should include the "v" prefix, e.g., "v22.14.0".
func Install(version string) error {
	versionNum := strings.TrimPrefix(version, "v")

	versionsDir, err := platform.VersionsDir()
	if err != nil {
		return err
	}
	destDir := filepath.Join(versionsDir, versionNum)

	// Check if already installed
	if _, err := os.Stat(destDir); err == nil {
		return fmt.Errorf("version %s is already installed", versionNum)
	}

	if err := platform.EnsureDirs(); err != nil {
		return err
	}

	// Download archive
	url := platform.DownloadURL(version)
	filename := platform.ArchiveFilename(version)
	tmpDir, err := os.MkdirTemp("", "nodeman-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, filename)
	fmt.Printf("Downloading %s...\n", url)
	if err := downloadFile(url, archivePath); err != nil {
		return fmt.Errorf("downloading Node.js %s: %w", versionNum, err)
	}

	// Verify checksum
	fmt.Println("Verifying checksum...")
	if err := verifyChecksum(version, archivePath, filename); err != nil {
		return fmt.Errorf("checksum verification failed: %w", err)
	}

	// Extract
	fmt.Print("Extracting...")
	extractionStart := time.Now()
	extractDir := filepath.Join(tmpDir, "extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return err
	}

	ext := platform.ArchiveExt()
	switch ext {
	case "tar.gz":
		if err := platform.ExtractTarGz(archivePath, extractDir); err != nil {
			return fmt.Errorf("extracting: %w", err)
		}
	case "tar.xz":
		if err := platform.ExtractTarXz(archivePath, extractDir); err != nil {
			return fmt.Errorf("extracting: %w", err)
		}
	case "zip":
		if err := platform.ExtractZip(archivePath, extractDir); err != nil {
			return fmt.Errorf("extracting: %w", err)
		}
	default:
		return fmt.Errorf("unsupported archive format: %s", ext)
	}
	fmt.Printf(" done (%.1fs)\n", time.Since(extractionStart).Seconds())

	// The archive extracts to a directory like node-v22.14.0-darwin-arm64/
	// We need to find it and rename it to just the version number.
	archiveDir := fmt.Sprintf("node-%s-%s-%s", version, platform.NodeOS(), platform.NodeArch())
	extractedPath := filepath.Join(extractDir, archiveDir)
	if _, err := os.Stat(extractedPath); err != nil {
		return fmt.Errorf("expected extracted directory %s not found: %w", archiveDir, err)
	}

	if err := os.Rename(extractedPath, destDir); err != nil {
		return fmt.Errorf("moving to versions directory: %w", err)
	}

	// Verify the installed binary works
	nodeBin := filepath.Join(platform.BinDir(destDir), "node"+platform.ExeSuffix())
	if out, err := exec.Command(nodeBin, "--version").Output(); err != nil {
		os.RemoveAll(destDir)
		return fmt.Errorf("installed binary verification failed (removing): %w", err)
	} else {
		fmt.Printf("Node.js %s installed successfully (verified: %s)\n", versionNum, strings.TrimSpace(string(out)))
	}
	return nil
}

// Uninstall removes an installed Node.js version.
func Uninstall(versionNum string) error {
	versionsDir, err := platform.VersionsDir()
	if err != nil {
		return err
	}
	destDir := filepath.Join(versionsDir, versionNum)

	if _, err := os.Stat(destDir); os.IsNotExist(err) {
		return fmt.Errorf("version %s is not installed", versionNum)
	}

	if err := os.RemoveAll(destDir); err != nil {
		return fmt.Errorf("removing version %s: %w", versionNum, err)
	}

	fmt.Printf("Node.js %s uninstalled.\n", versionNum)
	return nil
}

func downloadFile(url, dest string) error {
	client := httputil.NewClient(10 * time.Minute)
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	// Show progress
	total := resp.ContentLength
	var written int64
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			nw, writeErr := out.Write(buf[:n])
			if writeErr != nil {
				return writeErr
			}
			written += int64(nw)
			if total > 0 {
				pct := float64(written) / float64(total) * 100
				fmt.Printf("\r  %.1f%% (%s / %s)", pct, formatBytes(written), formatBytes(total))
			} else {
				fmt.Printf("\r  %s downloaded", formatBytes(written))
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	fmt.Println()
	return nil
}

func verifyChecksum(version, archivePath, filename string) error {
	// Download SHASUMS256.txt
	client := httputil.NewClient(15 * time.Second)
	resp, err := client.Get(platform.ChecksumsURL(version))
	if err != nil {
		return fmt.Errorf("fetching checksums: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("checksums returned status %d", resp.StatusCode)
	}

	// Parse and find the expected hash for our file
	var expectedHash string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == filename {
			expectedHash = parts[0]
			break
		}
	}
	if expectedHash == "" {
		return fmt.Errorf("checksum for %s not found in SHASUMS256.txt", filename)
	}

	// Compute actual hash
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actualHash := hex.EncodeToString(h.Sum(nil))

	if actualHash != expectedHash {
		return fmt.Errorf("hash mismatch: expected %s, got %s", expectedHash, actualHash)
	}

	return nil
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
