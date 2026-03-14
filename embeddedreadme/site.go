package embeddedreadme

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

//go:embed webdist
var bundledSite embed.FS

type payloadData struct {
	Title      string `json:"title"`
	Version    string `json:"version"`
	Generated  string `json:"generated"`
	Markdown   string `json:"markdown"`
	SourcePath string `json:"sourcePath"`
}

var scriptCloseTagRe = regexp.MustCompile(`(?i)</script>`)

// WriteVersionedDocs writes a standalone docs page to <baseDir>/docs/<version>/index.html.
// The resulting HTML has inline CSS, inline app JS, and inline markdown payload,
// so it can be opened directly with file:// without running a local web server.
func WriteVersionedDocs(baseDir, version string, markdown []byte, title, sourcePath string, force bool) (string, error) {
	if strings.TrimSpace(baseDir) == "" {
		return "", fmt.Errorf("base directory is required")
	}
	if strings.TrimSpace(version) == "" {
		version = "dev"
	}

	docsVersion := strings.TrimPrefix(version, "v")
	docDir := filepath.Join(baseDir, "docs", docsVersion)
	if err := os.MkdirAll(docDir, 0o755); err != nil {
		return "", fmt.Errorf("create docs directory: %w", err)
	}

	docPath := filepath.Join(docDir, "index.html")
	if !force {
		if info, err := os.Stat(docPath); err == nil && info.Size() > 0 {
			return docPath, nil
		}
	}

	payload := payloadData{
		Title:      title,
		Version:    version,
		Generated:  time.Now().Format("2006-01-02 15:04:05"),
		Markdown:   string(markdown),
		SourcePath: sourcePath,
	}

	htmlBytes, err := buildStandaloneHTML(payload)
	if err != nil {
		return "", err
	}

	if err := writeFileAtomic(docPath, htmlBytes, 0o644); err != nil {
		return "", fmt.Errorf("write docs html: %w", err)
	}

	return docPath, nil
}

func buildStandaloneHTML(payload payloadData) ([]byte, error) {
	indexBytes, err := fs.ReadFile(bundledSite, "webdist/index.html")
	if err != nil {
		return nil, fmt.Errorf("read bundled index.html: %w", err)
	}
	cssBytes, err := fs.ReadFile(bundledSite, "webdist/assets/index.css")
	if err != nil {
		return nil, fmt.Errorf("read bundled css: %w", err)
	}
	appBytes, err := fs.ReadFile(bundledSite, "webdist/assets/app.js")
	if err != nil {
		return nil, fmt.Errorf("read bundled app.js: %w", err)
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal docs payload: %w", err)
	}

	inline := string(indexBytes)
	inline, err = replaceOne(
		inline,
		`<link rel="stylesheet" crossorigin href="./assets/index.css">`,
		"<style>\n"+string(cssBytes)+"\n</style>",
		"css link",
	)
	if err != nil {
		return nil, err
	}

	payloadScript := "<script>window.__EMBEDDED_README_DATA__ = " + string(payloadJSON) + ";</script>"
	appScript := "<script type=\"module\">\n" + escapeInlineScript(string(appBytes)) + "\n</script>"
	inline, err = replaceOne(
		inline,
		`<script type="module" crossorigin src="./assets/app.js"></script>`,
		payloadScript+"\n    "+appScript,
		"app module script",
	)
	if err != nil {
		return nil, err
	}

	inline = strings.ReplaceAll(inline, `<script src="./docs-data.js"></script>`, "")

	if strings.Contains(inline, "./assets/") || strings.Contains(inline, "docs-data.js") {
		return nil, fmt.Errorf("standalone docs html still references external assets")
	}

	return []byte(inline), nil
}

func replaceOne(content, old, replacement, label string) (string, error) {
	if !strings.Contains(content, old) {
		return "", fmt.Errorf("%s not found in bundled index", label)
	}
	return strings.Replace(content, old, replacement, 1), nil
}

func escapeInlineScript(js string) string {
	return scriptCloseTagRe.ReplaceAllString(js, `<\/script>`)
}

func writeFileAtomic(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, content, mode); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}

// OpenInBrowser opens a file path in the user's default browser.
func OpenInBrowser(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	return openTarget(abs)
}

func openTarget(target string) error {

	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Start()
	case "darwin":
		return exec.Command("open", target).Start()
	default:
		if _, err := exec.LookPath("xdg-open"); err != nil {
			return fmt.Errorf("cannot open browser automatically: xdg-open not found")
		}
		return exec.Command("xdg-open", target).Start()
	}
}
