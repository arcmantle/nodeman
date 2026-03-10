package versions

import (
	"os"
	"path/filepath"
	"strings"
)

var versionFiles = []string{".nvmrc", ".node-version"}

// FindVersionFile walks from the current directory up to the filesystem root,
// looking for .nvmrc or .node-version files. Returns the version string,
// the path of the file found, or ("", "", nil) if none found.
func FindVersionFile() (version string, filePath string, err error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", "", err
	}

	for {
		for _, name := range versionFiles {
			p := filepath.Join(dir, name)
			data, err := os.ReadFile(p)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return "", "", err
			}
			v := strings.TrimSpace(string(data))
			if v != "" {
				return v, p, nil
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", "", nil
}
