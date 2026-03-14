package cli

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/arcmantle/nodeman/internal/httputil"
	"github.com/spf13/cobra"
)

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func newUpgradeCmd(currentVersion string) *cobra.Command {
	return &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade nodeman to the latest version",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := httputil.NewClient(30 * time.Second)

			resp, err := client.Get("https://api.github.com/repos/arcmantle/nodeman/releases/latest")
			if err != nil {
				return fmt.Errorf("checking for updates: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				return fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
			}

			var release githubRelease
			if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
				return fmt.Errorf("parsing release info: %w", err)
			}

			latestVersion := strings.TrimPrefix(release.TagName, "v")
			current := strings.TrimPrefix(currentVersion, "v")

			if current == latestVersion {
				fmt.Printf("Already up to date (%s)\n", currentVersion)
				return nil
			}

			asset, err := selectReleaseAsset(release.Assets)
			if err != nil {
				return fmt.Errorf("no release binary found for %s/%s", runtime.GOOS, runtime.GOARCH)
			}

			fmt.Printf("Upgrading %s -> %s\n", currentVersion, release.TagName)

			// Download release asset
			dlClient := httputil.NewClient(5 * time.Minute)
			dlResp, err := dlClient.Get(asset.BrowserDownloadURL)
			if err != nil {
				return fmt.Errorf("downloading update: %w", err)
			}
			defer dlResp.Body.Close()

			if dlResp.StatusCode != 200 {
				return fmt.Errorf("download returned status %d", dlResp.StatusCode)
			}

			execPath, err := os.Executable()
			if err != nil {
				return fmt.Errorf("finding current executable: %w", err)
			}
			execPath, err = filepath.EvalSymlinks(execPath)
			if err != nil {
				return fmt.Errorf("resolving executable path: %w", err)
			}

			tmpDir, err := os.MkdirTemp("", "nodeman-upgrade-*")
			if err != nil {
				return fmt.Errorf("creating temp directory: %w", err)
			}
			defer os.RemoveAll(tmpDir)

			assetPath := filepath.Join(tmpDir, asset.Name)
			assetFile, err := os.Create(assetPath)
			if err != nil {
				return fmt.Errorf("creating asset file: %w", err)
			}

			if _, err := io.Copy(assetFile, dlResp.Body); err != nil {
				assetFile.Close()
				return fmt.Errorf("writing update: %w", err)
			}
			assetFile.Close()

			newBinaryPath, err := extractReleaseBinary(assetPath, asset.Name, tmpDir)
			if err != nil {
				return err
			}

			if err := replaceCurrentExecutable(execPath, newBinaryPath); err != nil {
				return fmt.Errorf("replacing binary: %w", err)
			}

			if runtime.GOOS == "windows" {
				fmt.Printf("Upgrade to %s scheduled. Restart your terminal and run 'nodeman --version'.\n", release.TagName)
			} else {
				fmt.Printf("Successfully upgraded to %s\n", release.TagName)
			}
			return nil
		},
	}
}

func selectReleaseAsset(assets []githubAsset) (githubAsset, error) {
	osName := runtime.GOOS
	arch := runtime.GOARCH

	if osName == "windows" {
		for _, asset := range assets {
			name := strings.ToLower(asset.Name)
			if strings.Contains(name, fmt.Sprintf("_%s_%s", osName, arch)) && strings.HasSuffix(name, ".zip") {
				return asset, nil
			}
		}

		for _, asset := range assets {
			name := strings.ToLower(asset.Name)
			if strings.Contains(name, fmt.Sprintf("-%s-%s", osName, arch)) && strings.HasSuffix(name, ".exe") {
				return asset, nil
			}
		}
	} else {
		for _, asset := range assets {
			name := strings.ToLower(asset.Name)
			if strings.Contains(name, fmt.Sprintf("_%s_%s", osName, arch)) && strings.HasSuffix(name, ".tar.gz") {
				return asset, nil
			}
		}

		for _, asset := range assets {
			name := strings.ToLower(asset.Name)
			if strings.Contains(name, fmt.Sprintf("-%s-%s", osName, arch)) && !strings.Contains(name, "checksums") {
				return asset, nil
			}
		}
	}

	return githubAsset{}, fmt.Errorf("matching release asset not found")
}

func extractReleaseBinary(assetPath, assetName, tmpDir string) (string, error) {
	lower := strings.ToLower(assetName)
	binName := "nodeman"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}

	outputPath := filepath.Join(tmpDir, "nodeman-upgrade-binary"+filepath.Ext(binName))

	if strings.HasSuffix(lower, ".zip") {
		r, err := zip.OpenReader(assetPath)
		if err != nil {
			return "", fmt.Errorf("opening zip archive: %w", err)
		}
		defer r.Close()

		for _, f := range r.File {
			if strings.EqualFold(path.Base(f.Name), binName) {
				rc, err := f.Open()
				if err != nil {
					return "", fmt.Errorf("opening archive entry: %w", err)
				}

				out, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
				if err != nil {
					rc.Close()
					return "", fmt.Errorf("creating upgrade binary: %w", err)
				}

				if _, err := io.Copy(out, rc); err != nil {
					out.Close()
					rc.Close()
					return "", fmt.Errorf("extracting upgrade binary: %w", err)
				}

				out.Close()
				rc.Close()
				return outputPath, nil
			}
		}

		return "", fmt.Errorf("binary %s not found in zip asset", binName)
	}

	if strings.HasSuffix(lower, ".tar.gz") {
		f, err := os.Open(assetPath)
		if err != nil {
			return "", fmt.Errorf("opening tar.gz archive: %w", err)
		}
		defer f.Close()

		gz, err := gzip.NewReader(f)
		if err != nil {
			return "", fmt.Errorf("opening gzip stream: %w", err)
		}
		defer gz.Close()

		tr := tar.NewReader(gz)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", fmt.Errorf("reading tar archive: %w", err)
			}

			if strings.EqualFold(path.Base(hdr.Name), binName) {
				out, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
				if err != nil {
					return "", fmt.Errorf("creating upgrade binary: %w", err)
				}

				if _, err := io.Copy(out, tr); err != nil {
					out.Close()
					return "", fmt.Errorf("extracting upgrade binary: %w", err)
				}

				out.Close()
				return outputPath, nil
			}
		}

		return "", fmt.Errorf("binary %s not found in tar.gz asset", binName)
	}

	if err := os.Chmod(assetPath, 0o755); err != nil {
		return "", fmt.Errorf("setting permissions: %w", err)
	}

	return assetPath, nil
}

func replaceCurrentExecutable(execPath, newBinaryPath string) error {
	if runtime.GOOS != "windows" {
		return os.Rename(newBinaryPath, execPath)
	}

	stagedPath := execPath + ".upgrade"
	if err := os.Remove(stagedPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	if err := copyFile(newBinaryPath, stagedPath); err != nil {
		return err
	}

	script := fmt.Sprintf(
		`$target='%s'; $new='%s'; for($i=0; $i -lt 120; $i++){ try { if(-not (Test-Path -LiteralPath $new)) { exit 1 }; Move-Item -LiteralPath $new -Destination $target -Force; exit 0 } catch { Start-Sleep -Milliseconds 250 } }; exit 1`,
		strings.ReplaceAll(execPath, "'", "''"),
		strings.ReplaceAll(stagedPath, "'", "''"),
	)

	cmd := exec.Command("powershell", "-NoProfile", "-WindowStyle", "Hidden", "-Command", script)
	return cmd.Start()
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

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return nil
}
