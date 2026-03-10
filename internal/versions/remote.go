package versions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/roen/nodeman/internal/httputil"
	"github.com/roen/nodeman/internal/platform"
)

const remoteIndexURL = "https://nodejs.org/dist/index.json"

const cacheTTL = 1 * time.Hour

// RemoteVersion represents a single entry from the Node.js dist index.
type RemoteVersion struct {
	Version  string      `json:"version"`
	Date     string      `json:"date"`
	Files    []string    `json:"files"`
	NPM      string      `json:"npm"`
	LTS      interface{} `json:"lts"`
	Security bool        `json:"security"`
}

// LTSName returns the LTS codename, or "" if not an LTS release.
func (rv RemoteVersion) LTSName() string {
	if s, ok := rv.LTS.(string); ok {
		return s
	}
	return ""
}

// IsLTS returns true if this version is an LTS release.
func (rv RemoteVersion) IsLTS() bool {
	return rv.LTSName() != ""
}

// VersionNumber returns the version without the leading "v".
func (rv RemoteVersion) VersionNumber() string {
	return strings.TrimPrefix(rv.Version, "v")
}

func cacheFilePath() (string, error) {
	root, err := platform.RootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "cache", "remote-versions.json"), nil
}

func loadCache() ([]RemoteVersion, bool) {
	p, err := cacheFilePath()
	if err != nil {
		return nil, false
	}
	info, err := os.Stat(p)
	if err != nil {
		return nil, false
	}
	if time.Since(info.ModTime()) > cacheTTL {
		return nil, false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, false
	}
	var versions []RemoteVersion
	if err := json.Unmarshal(data, &versions); err != nil {
		return nil, false
	}
	return versions, true
}

func saveCache(versions []RemoteVersion) {
	p, err := cacheFilePath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	data, err := json.Marshal(versions)
	if err != nil {
		return
	}
	os.WriteFile(p, data, 0o644)
}

// FetchRemoteVersions downloads and parses the Node.js dist index.
// Results are cached for 1 hour. Use FetchRemoteVersionsNoCache to bypass.
func FetchRemoteVersions() ([]RemoteVersion, error) {
	if cached, ok := loadCache(); ok {
		return cached, nil
	}
	return FetchRemoteVersionsNoCache()
}

// FetchRemoteVersionsNoCache always fetches fresh data from nodejs.org.
func FetchRemoteVersionsNoCache() ([]RemoteVersion, error) {
	client := httputil.NewClient(30 * time.Second)
	resp, err := client.Get(remoteIndexURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch remote versions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, remoteIndexURL)
	}

	var versions []RemoteVersion
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return nil, fmt.Errorf("failed to parse remote versions: %w", err)
	}
	saveCache(versions)
	return versions, nil
}
