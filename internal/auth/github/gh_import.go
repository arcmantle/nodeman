package github

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/arcmantle/nodeman/internal/auth"
)

const ghRegistry = "https://npm.pkg.github.com/"

// GhImport obtains a GitHub Packages token by importing it from the gh CLI.
type GhImport struct {
	// cmdRunner is overridden in tests. nil means use real exec.
	cmdRunner func(name string, args ...string) ([]byte, error)
	// interactiveRunner runs a command connected to the real terminal. Overridden in tests.
	interactiveRunner func(name string, args ...string) error
	// prompter is overridden in tests. nil means use real stdin.
	prompter func(msg string) (bool, error)
}

func (g *GhImport) Name() string     { return "github" }
func (g *GhImport) Method() string   { return "gh CLI" }
func (g *GhImport) Registry() string { return ghRegistry }

func (g *GhImport) run(name string, args ...string) ([]byte, error) {
	if g.cmdRunner != nil {
		return g.cmdRunner(name, args...)
	}
	return exec.Command(name, args...).CombinedOutput()
}

func (g *GhImport) runInteractive(name string, args ...string) error {
	if g.interactiveRunner != nil {
		return g.interactiveRunner(name, args...)
	}
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (g *GhImport) prompt(msg string) (bool, error) {
	if g.prompter != nil {
		return g.prompter(msg)
	}
	fmt.Fprint(os.Stderr, msg+" [Y/n] ")
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, fmt.Errorf("reading input: %w", err)
		}
		return false, fmt.Errorf("reading input: unexpected EOF")
	}
	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return answer == "" || answer == "y" || answer == "yes", nil
}

// Login attempts to import a token from the gh CLI.
// scope is the npm scope (e.g. @my-org) for which auth is being configured;
// the OAuth scope (read:packages) is always requested regardless.
// Returns ErrProviderUnavailable if gh is not installed.
// Returns ErrProviderDeclined if the user declines to refresh scopes.
func (g *GhImport) Login(_ string) (string, error) {
	// Check gh is available
	if _, err := g.run("gh", "--version"); err != nil {
		return "", fmt.Errorf("%w: gh CLI is not installed or not in PATH", auth.ErrProviderUnavailable)
	}

	// Check if the token has read:packages scope
	statusOut, err := g.run("gh", "auth", "status")
	if err != nil {
		return "", fmt.Errorf("%w: gh is not authenticated (run 'gh auth login' first)", auth.ErrProviderUnavailable)
	}

	if !hasReadPackagesScope(string(statusOut)) {
		fmt.Fprintln(os.Stderr, "Your gh CLI token does not include the 'read:packages' scope.")
		ok, err := g.prompt("Run 'gh auth refresh -s read:packages' to add it?")
		if err != nil {
			return "", err
		}
		if !ok {
			return "", fmt.Errorf("%w: declined gh scope refresh", auth.ErrProviderDeclined)
		}

		if err := g.runInteractive("gh", "auth", "refresh", "--hostname", "github.com", "-s", "read:packages"); err != nil {
			return "", fmt.Errorf("gh auth refresh failed: %w", err)
		}
	}

	// Get the token
	tokenOut, err := g.run("gh", "auth", "token")
	if err != nil {
		return "", fmt.Errorf("failed to get gh auth token: %w", err)
	}

	token := strings.TrimSpace(string(tokenOut))
	if token == "" {
		return "", fmt.Errorf("%w: gh auth token returned empty", auth.ErrProviderUnavailable)
	}

	return token, nil
}

func hasReadPackagesScope(statusOutput string) bool {
	for _, line := range strings.Split(statusOutput, "\n") {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "read:packages") {
			return true
		}
	}
	return false
}
