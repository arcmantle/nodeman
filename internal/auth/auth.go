package auth

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/arcmantle/nodeman/internal/config"
	keyring "github.com/zalando/go-keyring"
)

const keychainService = "nodeman/npm-auth"

var packageManagerCommands = map[string]bool{
	"npm":      true,
	"npx":      true,
	"pnpm":     true,
	"pnpx":     true,
	"yarn":     true,
	"yarnpkg":  true,
	"corepack": true,
}

type keychainBackend interface {
	Set(service, user, password string) error
	Get(service, user string) (string, error)
	Delete(service, user string) error
}

type systemKeychain struct{}

func (systemKeychain) Set(service, user, password string) error {
	return keyring.Set(service, user, password)
}

func (systemKeychain) Get(service, user string) (string, error) {
	return keyring.Get(service, user)
}

func (systemKeychain) Delete(service, user string) error {
	return keyring.Delete(service, user)
}

var backend keychainBackend = systemKeychain{}

// PreparedEnv contains environment overrides and cleanup for a launched package-manager command.
type PreparedEnv struct {
	Env      []string
	Warnings []string
	cleanup  func() error
}

// Cleanup removes any temporary files created while preparing the environment.
func (p *PreparedEnv) Cleanup() error {
	if p == nil || p.cleanup == nil {
		return nil
	}
	return p.cleanup()
}

// IsPackageManagerCommand reports whether nodeman should inject package-manager auth for this shim.
func IsPackageManagerCommand(name string) bool {
	return packageManagerCommands[strings.ToLower(strings.TrimSpace(name))]
}

// HasStoredToken reports whether the keychain contains a token for the given registry.
func HasStoredToken(registry string) (bool, error) {
	normalized, err := NormalizeRegistry(registry)
	if err != nil {
		return false, err
	}

	token, err := backend.Get(keychainService, keychainAccount(normalized))
	if err == nil {
		return strings.TrimSpace(token) != "", nil
	}
	if errors.Is(err, keyring.ErrNotFound) {
		return false, nil
	}
	return false, err
}

// AuthTokenConfigKey returns the npm-compatible auth token key for a registry.
func AuthTokenConfigKey(registry string) (string, error) {
	normalized, err := NormalizeRegistry(registry)
	if err != nil {
		return "", err
	}
	return npmAuthTokenKey(normalized)
}

// HasActiveMappings reports whether package auth is enabled and has at least one active registry mapping.
func HasActiveMappings(cfg *config.Config) bool {
	if cfg == nil || !cfg.PackageAuth.Enabled {
		return false
	}
	return len(uniqueEnabledRegistries(cfg.PackageAuth.Registries)) > 0
}

// NormalizeRegistry canonicalizes a registry URL for config storage and npmrc rendering.
func NormalizeRegistry(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", fmt.Errorf("registry cannot be empty")
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "https://" + trimmed
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("parsing registry %q: %w", input, err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("registry %q must include a host", input)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("registry %q must use http or https", input)
	}

	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	u.Host = strings.ToLower(u.Host)
	if u.Path == "" {
		u.Path = "/"
	}
	if !strings.HasPrefix(u.Path, "/") {
		u.Path = "/" + u.Path
	}
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}

	return u.String(), nil
}

// NormalizeScope canonicalizes an npm scope. Empty input stays empty.
func NormalizeScope(input string) string {
	trimmed := strings.TrimSpace(input)
	trimmed = strings.TrimPrefix(trimmed, "@")
	if trimmed == "" {
		return ""
	}
	return "@" + trimmed
}

// StoreToken writes a registry token to the OS keychain.
func StoreToken(registry, token string) error {
	normalized, err := NormalizeRegistry(registry)
	if err != nil {
		return err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("token cannot be empty")
	}
	return backend.Set(keychainService, keychainAccount(normalized), token)
}

// DeleteToken removes a registry token from the OS keychain.
func DeleteToken(registry string) error {
	normalized, err := NormalizeRegistry(registry)
	if err != nil {
		return err
	}
	err = backend.Delete(keychainService, keychainAccount(normalized))
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// PrepareEnvironment builds a temporary npm-compatible config file and env overrides.
func PrepareEnvironment(baseEnv []string) (*PreparedEnv, error) {
	prepared := &PreparedEnv{Env: append([]string(nil), baseEnv...)}

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	if !cfg.PackageAuth.Enabled || len(cfg.PackageAuth.Registries) == 0 {
		return prepared, nil
	}

	contents, warnings, err := buildUserConfig(cfg.PackageAuth.Registries, prepared.Env)
	if err != nil {
		return nil, err
	}
	prepared.Warnings = append(prepared.Warnings, warnings...)
	if contents == "" {
		return prepared, nil
	}

	f, err := os.CreateTemp("", "nodeman-npmrc-*.npmrc")
	if err != nil {
		return nil, fmt.Errorf("creating temporary npm config: %w", err)
	}
	if _, err := f.WriteString(contents); err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, fmt.Errorf("writing temporary npm config: %w", err)
	}
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, fmt.Errorf("setting permissions on temporary npm config: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return nil, fmt.Errorf("closing temporary npm config: %w", err)
	}

	prepared.Env = setEnv(prepared.Env, "NPM_CONFIG_USERCONFIG", f.Name())
	prepared.Env = setEnv(prepared.Env, "npm_config_userconfig", f.Name())
	prepared.cleanup = func() error {
		if err := os.Remove(f.Name()); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	return prepared, nil
}

// DoctorStatus summarizes package auth config and keychain availability.
func DoctorStatus(cfg *config.Config) (bool, string) {
	if cfg == nil {
		return false, "config unavailable"
	}
	if !cfg.PackageAuth.Enabled {
		if len(cfg.PackageAuth.Registries) == 0 {
			return true, "disabled"
		}
		return true, fmt.Sprintf("disabled (%d registries configured)", len(cfg.PackageAuth.Registries))
	}

	enabledRegistries := uniqueEnabledRegistries(cfg.PackageAuth.Registries)
	if len(enabledRegistries) == 0 {
		return false, "enabled but no registry mappings configured"
	}

	resolved := 0
	missing := 0
	for _, registry := range enabledRegistries {
		_, err := backend.Get(keychainService, keychainAccount(registry))
		switch {
		case err == nil:
			resolved++
		case errors.Is(err, keyring.ErrNotFound):
			missing++
		default:
			return false, fmt.Sprintf("keychain lookup failed for %s: %s", registry, err)
		}
	}

	if missing > 0 {
		return false, fmt.Sprintf("enabled with %d registry mapping(s), %d token(s) missing", len(enabledRegistries), missing)
	}
	return true, fmt.Sprintf("enabled with %d registry mapping(s)", resolved)
}

func buildUserConfig(registries []config.PackageAuthRegistry, baseEnv []string) (string, []string, error) {
	existingContent, err := readExistingUserConfig(baseEnv)
	if err != nil {
		return "", nil, err
	}

	warnings := []string{}
	lines := []string{}
	seenLines := map[string]bool{}
	tokenCache := map[string]string{}

	for _, entry := range registries {
		if !entry.Enabled {
			continue
		}

		registry, err := NormalizeRegistry(entry.Registry)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("skipping invalid registry %q: %s", entry.Registry, err))
			continue
		}

		token, ok := tokenCache[registry]
		if !ok {
			token, err = backend.Get(keychainService, keychainAccount(registry))
			if err != nil {
				if errors.Is(err, keyring.ErrNotFound) {
					warnings = append(warnings, fmt.Sprintf("no token stored for %s", registry))
				} else {
					warnings = append(warnings, fmt.Sprintf("failed to load token for %s: %s", registry, err))
				}
				continue
			}
			tokenCache[registry] = token
		}

		scope := NormalizeScope(entry.Scope)
		if scope != "" {
			appendLine(&lines, seenLines, fmt.Sprintf("%s:registry=%s", scope, registry))
		}

		authKey, err := npmAuthTokenKey(registry)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("cannot render auth config for %s: %s", registry, err))
			continue
		}
		appendLine(&lines, seenLines, fmt.Sprintf("%s=%s", authKey, token))
	}

	if len(lines) == 0 {
		return "", warnings, nil
	}

	var builder strings.Builder
	if existingContent != "" {
		builder.WriteString(strings.TrimRight(existingContent, "\n"))
		builder.WriteString("\n\n")
	}
	builder.WriteString("; generated by nodeman\n")
	for _, line := range lines {
		builder.WriteString(line)
		builder.WriteByte('\n')
	}

	return builder.String(), warnings, nil
}

func readExistingUserConfig(baseEnv []string) (string, error) {
	path := userConfigPath(baseEnv)
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("reading npm user config %s: %w", path, err)
	}
	return string(data), nil
}

func userConfigPath(baseEnv []string) string {
	for _, entry := range baseEnv {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if strings.EqualFold(key, "npm_config_userconfig") && strings.TrimSpace(value) != "" {
			return value
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".npmrc")
}

func npmAuthTokenKey(registry string) (string, error) {
	u, err := url.Parse(registry)
	if err != nil {
		return "", err
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	return fmt.Sprintf("//%s%s:_authToken", u.Host, path), nil
}

func keychainAccount(registry string) string {
	return registry
}

func setEnv(base []string, key, value string) []string {
	result := make([]string, 0, len(base)+1)
	replaced := false
	for _, entry := range base {
		name, _, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(name, key) {
			if !replaced {
				result = append(result, key+"="+value)
				replaced = true
			}
			continue
		}
		result = append(result, entry)
	}
	if !replaced {
		result = append(result, key+"="+value)
	}
	return result
}

func appendLine(lines *[]string, seen map[string]bool, line string) {
	if seen[line] {
		return
	}
	seen[line] = true
	*lines = append(*lines, line)
}

func uniqueEnabledRegistries(entries []config.PackageAuthRegistry) []string {
	seen := map[string]bool{}
	registries := []string{}
	for _, entry := range entries {
		if !entry.Enabled {
			continue
		}
		registry, err := NormalizeRegistry(entry.Registry)
		if err != nil || seen[registry] {
			continue
		}
		seen[registry] = true
		registries = append(registries, registry)
	}
	sort.Strings(registries)
	return registries
}
