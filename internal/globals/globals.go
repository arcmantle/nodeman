package globals

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/arcmantle/nodeman/internal/auth"
	"github.com/arcmantle/nodeman/internal/platform"
)

// Manifest represents the tracked global npm packages.
type Manifest struct {
	Packages []string `json:"packages"`
}

func manifestPath() (string, error) {
	root, err := platform.RootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "globals.json"), nil
}

// LoadManifest reads the globals manifest from disk.
func LoadManifest() (*Manifest, error) {
	p, err := manifestPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &Manifest{}, nil
		}
		return nil, fmt.Errorf("reading globals manifest: %w", err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing globals manifest: %w", err)
	}
	return &m, nil
}

// SaveManifest writes the globals manifest to disk.
func SaveManifest(m *Manifest) error {
	if err := platform.EnsureDirs(); err != nil {
		return err
	}

	p, err := manifestPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(p, data, 0o600)
}

// Add adds a package to the globals manifest if not already present.
func Add(pkg string) error {
	m, err := LoadManifest()
	if err != nil {
		return err
	}

	for _, p := range m.Packages {
		if p == pkg {
			fmt.Printf("%s is already tracked.\n", pkg)
			return nil
		}
	}

	m.Packages = append(m.Packages, pkg)
	if err := SaveManifest(m); err != nil {
		return err
	}
	fmt.Printf("Added %s to global packages manifest.\n", pkg)
	return nil
}

// Remove removes a package from the globals manifest.
func Remove(pkg string) error {
	m, err := LoadManifest()
	if err != nil {
		return err
	}

	found := false
	var filtered []string
	for _, p := range m.Packages {
		if p == pkg {
			found = true
			continue
		}
		filtered = append(filtered, p)
	}

	if !found {
		return fmt.Errorf("%s is not in the globals manifest", pkg)
	}

	m.Packages = filtered
	if err := SaveManifest(m); err != nil {
		return err
	}
	fmt.Printf("Removed %s from global packages manifest.\n", pkg)
	return nil
}

// List prints all tracked global packages.
func List() error {
	m, err := LoadManifest()
	if err != nil {
		return err
	}

	if len(m.Packages) == 0 {
		fmt.Println("No global packages tracked. Use 'nodeman globals add <package>' to add one.")
		return nil
	}

	fmt.Println("Tracked global packages:")
	for _, p := range m.Packages {
		fmt.Printf("  - %s\n", p)
	}
	return nil
}

// ReinstallAll installs all tracked global packages using the npm at the given bin directory.
func ReinstallAll(binDir string) error {
	m, err := LoadManifest()
	if err != nil {
		return err
	}

	if len(m.Packages) == 0 {
		return nil
	}

	npmPath, err := platform.ResolveBinCommand(binDir, "npm")
	if err != nil {
		return err
	}

	fmt.Printf("Reinstalling %d global package(s)...\n", len(m.Packages))
	args := append([]string{"install", "-g"}, m.Packages...)
	cmd := platform.CommandForBinary(npmPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	prepared, err := auth.PrepareEnvironment(os.Environ())
	if err != nil {
		return fmt.Errorf("preparing package auth: %w", err)
	}
	defer prepared.Cleanup()
	cmd.Env = prepared.Env
	for _, warning := range prepared.Warnings {
		fmt.Printf("Warning: %s\n", warning)
	}

	if err := cmd.Run(); err != nil {
		// Report but don't fail the entire use operation
		fmt.Printf("Warning: some global packages may have failed to install: %s\n", err)
		fmt.Printf("Packages: %s\n", strings.Join(m.Packages, ", "))
	} else {
		fmt.Println("Global packages reinstalled successfully.")
	}
	return nil
}
