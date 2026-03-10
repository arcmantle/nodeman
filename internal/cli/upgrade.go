package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/roen/nodeman/internal/httputil"
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

			resp, err := client.Get("https://api.github.com/repos/roen/nodeman/releases/latest")
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

			// Find the right asset for this OS/arch
			suffix := fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
			if runtime.GOOS == "windows" {
				suffix += ".exe"
			}

			var assetURL string
			for _, a := range release.Assets {
				if strings.Contains(a.Name, suffix) {
					assetURL = a.BrowserDownloadURL
					break
				}
			}
			if assetURL == "" {
				return fmt.Errorf("no release binary found for %s/%s", runtime.GOOS, runtime.GOARCH)
			}

			fmt.Printf("Upgrading %s → %s\n", currentVersion, release.TagName)

			// Download to temp file
			dlClient := httputil.NewClient(5 * time.Minute)
			dlResp, err := dlClient.Get(assetURL)
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

			tmpFile, err := os.CreateTemp(filepath.Dir(execPath), "nodeman-upgrade-*")
			if err != nil {
				return fmt.Errorf("creating temp file: %w", err)
			}
			tmpPath := tmpFile.Name()

			if _, err := io.Copy(tmpFile, dlResp.Body); err != nil {
				tmpFile.Close()
				os.Remove(tmpPath)
				return fmt.Errorf("writing update: %w", err)
			}
			tmpFile.Close()

			// Make executable
			if err := os.Chmod(tmpPath, 0o755); err != nil {
				os.Remove(tmpPath)
				return fmt.Errorf("setting permissions: %w", err)
			}

			// Atomic rename over current binary
			if err := os.Rename(tmpPath, execPath); err != nil {
				os.Remove(tmpPath)
				return fmt.Errorf("replacing binary: %w", err)
			}

			fmt.Printf("Successfully upgraded to %s\n", release.TagName)
			return nil
		},
	}
}
