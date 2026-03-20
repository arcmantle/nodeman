package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/arcmantle/nodeman/internal/auth"
	"github.com/arcmantle/nodeman/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage package-manager registry auth stored in the OS keychain",
		Long: `Manage per-registry package-manager tokens that nodeman loads from the
OS keychain before launching npm, pnpm, or yarn-related commands.

Tokens are stored in the system keychain. nodeman only stores registry metadata
in ~/.nodeman/config.json and injects auth for child processes when needed.`,
	}

	cmd.AddCommand(
		newAuthListCmd(),
		newAuthSetCmd(),
		newAuthTestCmd(),
		newAuthRemoveCmd(),
		newAuthEnableCmd(),
		newAuthDisableCmd(),
	)

	return cmd
}

func newAuthTestCmd() *cobra.Command {
	var scope string

	cmd := &cobra.Command{
		Use:   "test [registry]",
		Short: "Validate package-manager auth mappings and keychain tokens",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if len(cfg.PackageAuth.Registries) == 0 {
				return fmt.Errorf("no package-manager auth mappings configured")
			}

			selectedRegistry := ""
			if len(args) == 1 {
				selectedRegistry, err = auth.NormalizeRegistry(args[0])
				if err != nil {
					return err
				}
			}
			selectedScope := auth.NormalizeScope(scope)

			status := "disabled"
			if cfg.PackageAuth.Enabled {
				status = "enabled"
			}
			fmt.Printf("Package auth is %s.\n", status)

			matched := 0
			failures := 0
			for _, entry := range cfg.PackageAuth.Registries {
				registry, err := auth.NormalizeRegistry(entry.Registry)
				if err != nil {
					if selectedRegistry != "" {
						continue
					}
					failures++
					fmt.Printf("  ✗ %s\n", err)
					continue
				}
				mappingScope := auth.NormalizeScope(entry.Scope)
				if selectedRegistry != "" && registry != selectedRegistry {
					continue
				}
				if selectedScope != "" && mappingScope != selectedScope {
					continue
				}

				matched++
				label := registry
				if mappingScope != "" {
					label += fmt.Sprintf(" (scope %s)", mappingScope)
				}

				if !cfg.PackageAuth.Enabled {
					fmt.Printf("  - %s: package auth is globally disabled\n", label)
					continue
				}
				if !entry.Enabled {
					fmt.Printf("  - %s: mapping is disabled\n", label)
					continue
				}

				present, err := auth.HasStoredToken(registry)
				if err != nil {
					failures++
					fmt.Printf("  ✗ %s: keychain lookup failed: %s\n", label, err)
					continue
				}
				if !present {
					failures++
					fmt.Printf("  ✗ %s: no token stored in the OS keychain\n", label)
					continue
				}

				authKey, err := auth.AuthTokenConfigKey(registry)
				if err != nil {
					failures++
					fmt.Printf("  ✗ %s: cannot render npm auth config key: %s\n", label, err)
					continue
				}
				fmt.Printf("  ✓ %s: token found, config key %s\n", label, authKey)
			}

			if matched == 0 {
				if selectedRegistry != "" || selectedScope != "" {
					return fmt.Errorf("no auth mappings matched the selection")
				}
				return fmt.Errorf("no package-manager auth mappings configured")
			}
			if failures > 0 {
				return fmt.Errorf("%d auth check(s) failed", failures)
			}

			fmt.Println("Auth checks passed.")
			return nil
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "", "Optional npm scope for the mapping to test")
	return cmd
}

func newAuthListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured package-manager registry auth mappings",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			if len(cfg.PackageAuth.Registries) == 0 {
				fmt.Println("No package-manager auth mappings configured.")
				fmt.Println("Use 'nodeman auth set <registry>' to add one.")
				return nil
			}

			status := "disabled"
			if cfg.PackageAuth.Enabled {
				status = "enabled"
			}
			fmt.Printf("Package auth is %s.\n", status)

			entries := append([]config.PackageAuthRegistry(nil), cfg.PackageAuth.Registries...)
			sort.Slice(entries, func(i, j int) bool {
				if entries[i].Registry == entries[j].Registry {
					return entries[i].Scope < entries[j].Scope
				}
				return entries[i].Registry < entries[j].Registry
			})

			fmt.Println("Configured registries:")
			for _, entry := range entries {
				state := "disabled"
				if entry.Enabled {
					state = "enabled"
				}
				detail := entry.Registry
				if entry.Scope != "" {
					detail += fmt.Sprintf(" (scope %s)", auth.NormalizeScope(entry.Scope))
				}
				fmt.Printf("  - [%s] %s\n", state, detail)
			}

			return nil
		},
	}
}

func newAuthSetCmd() *cobra.Command {
	var scope string
	var token string
	var tokenEnv string
	var tokenStdin bool

	cmd := &cobra.Command{
		Use:   "set <registry>",
		Short: "Store a registry token in the OS keychain and enable it for nodeman",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			registry, err := auth.NormalizeRegistry(args[0])
			if err != nil {
				return err
			}
			scope = auth.NormalizeScope(scope)

			resolvedToken, err := resolveAuthToken(token, tokenEnv, tokenStdin)
			if err != nil {
				return err
			}
			if err := auth.StoreToken(registry, resolvedToken); err != nil {
				return fmt.Errorf("storing token in keychain: %w", err)
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}

			updated := false
			for index, entry := range cfg.PackageAuth.Registries {
				if sameRegistryEntry(entry, registry, scope) {
					cfg.PackageAuth.Registries[index].Registry = registry
					cfg.PackageAuth.Registries[index].Scope = scope
					cfg.PackageAuth.Registries[index].Enabled = true
					updated = true
					break
				}
			}
			if !updated {
				cfg.PackageAuth.Registries = append(cfg.PackageAuth.Registries, config.PackageAuthRegistry{
					Registry: registry,
					Scope:    scope,
					Enabled:  true,
				})
			}
			cfg.PackageAuth.Enabled = true

			if err := config.Save(cfg); err != nil {
				return err
			}

			if scope != "" {
				fmt.Printf("Stored auth for %s with scope %s.\n", registry, scope)
			} else {
				fmt.Printf("Stored auth for %s.\n", registry)
			}
			fmt.Println("nodeman will inject this token for package-manager subprocesses.")
			return nil
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "", "Optional npm scope to map to this registry (for example @my-org)")
	cmd.Flags().StringVar(&token, "token", "", "Registry token to store in the keychain")
	cmd.Flags().StringVar(&tokenEnv, "token-env", "", "Read the registry token from the named environment variable")
	cmd.Flags().BoolVar(&tokenStdin, "token-stdin", false, "Read the registry token from stdin")

	return cmd
}

func newAuthRemoveCmd() *cobra.Command {
	var scope string

	cmd := &cobra.Command{
		Use:   "remove <registry>",
		Short: "Remove a registry auth mapping and optionally delete its keychain token",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			registry, err := auth.NormalizeRegistry(args[0])
			if err != nil {
				return err
			}
			scope = auth.NormalizeScope(scope)

			cfg, err := config.Load()
			if err != nil {
				return err
			}

			filtered := make([]config.PackageAuthRegistry, 0, len(cfg.PackageAuth.Registries))
			removed := false
			for _, entry := range cfg.PackageAuth.Registries {
				if sameRegistryEntry(entry, registry, scope) {
					removed = true
					continue
				}
				filtered = append(filtered, entry)
			}
			if !removed {
				return fmt.Errorf("no auth mapping found for %s", registry)
			}

			cfg.PackageAuth.Registries = filtered
			if len(filtered) == 0 {
				cfg.PackageAuth.Enabled = false
			}
			if err := config.Save(cfg); err != nil {
				return err
			}

			if !registryStillReferenced(filtered, registry) {
				if err := auth.DeleteToken(registry); err != nil {
					return fmt.Errorf("removing keychain token: %w", err)
				}
				fmt.Printf("Removed auth mapping and deleted keychain token for %s.\n", registry)
				return nil
			}

			fmt.Printf("Removed auth mapping for %s.\n", registry)
			fmt.Println("The keychain token was kept because another mapping still references that registry.")
			return nil
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "", "Optional npm scope for the mapping to remove")
	return cmd
}

func newAuthEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable",
		Short: "Enable package-manager auth injection",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cfg.PackageAuth.Enabled = true
			if err := config.Save(cfg); err != nil {
				return err
			}
			fmt.Println("Package-manager auth enabled.")
			return nil
		},
	}
}

func newAuthDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable",
		Short: "Disable package-manager auth injection without deleting saved mappings",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cfg.PackageAuth.Enabled = false
			if err := config.Save(cfg); err != nil {
				return err
			}
			fmt.Println("Package-manager auth disabled.")
			return nil
		},
	}
}

func resolveAuthToken(flagToken, tokenEnv string, tokenStdin bool) (string, error) {
	provided := 0
	if flagToken != "" {
		provided++
	}
	if tokenEnv != "" {
		provided++
	}
	if tokenStdin {
		provided++
	}
	if provided > 1 {
		return "", fmt.Errorf("use only one of --token, --token-env, or --token-stdin")
	}

	if flagToken != "" {
		return strings.TrimSpace(flagToken), nil
	}
	if tokenEnv != "" {
		value := strings.TrimSpace(os.Getenv(tokenEnv))
		if value == "" {
			return "", fmt.Errorf("environment variable %s is empty", tokenEnv)
		}
		return value, nil
	}
	if tokenStdin {
		data, err := io.ReadAll(bufio.NewReader(os.Stdin))
		if err != nil {
			return "", fmt.Errorf("reading token from stdin: %w", err)
		}
		value := strings.TrimSpace(string(data))
		if value == "" {
			return "", fmt.Errorf("token from stdin is empty")
		}
		return value, nil
	}

	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("stdin is not a terminal; use --token-stdin or --token-env")
	}

	fmt.Fprint(os.Stdout, "Token: ")
	data, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stdout)
	if err != nil {
		return "", fmt.Errorf("reading token from terminal: %w", err)
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", fmt.Errorf("token cannot be empty")
	}
	return value, nil
}

func sameRegistryEntry(entry config.PackageAuthRegistry, registry, scope string) bool {
	return entry.Registry == registry && auth.NormalizeScope(entry.Scope) == scope
}

func registryStillReferenced(entries []config.PackageAuthRegistry, registry string) bool {
	for _, entry := range entries {
		if entry.Registry == registry {
			return true
		}
	}
	return false
}
