package github

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/arcmantle/nodeman/internal/auth"
)

func TestGhImport_GhNotInstalled(t *testing.T) {
	g := &GhImport{
		cmdRunner: func(name string, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("exec: not found")
		},
	}

	_, err := g.Login("@my-org")
	if !errors.Is(err, auth.ErrProviderUnavailable) {
		t.Fatalf("expected ErrProviderUnavailable, got: %v", err)
	}
}

func TestGhImport_GhNotAuthenticated(t *testing.T) {
	g := &GhImport{
		cmdRunner: func(name string, args ...string) ([]byte, error) {
			if args[0] == "--version" {
				return []byte("gh version 2.40.0"), nil
			}
			// auth status fails when not logged in
			return []byte("You are not logged in"), fmt.Errorf("exit 1")
		},
	}

	_, err := g.Login("@my-org")
	if !errors.Is(err, auth.ErrProviderUnavailable) {
		t.Fatalf("expected ErrProviderUnavailable, got: %v", err)
	}
}

func TestGhImport_HasScope(t *testing.T) {
	g := &GhImport{
		cmdRunner: func(name string, args ...string) ([]byte, error) {
			key := strings.Join(append([]string{name}, args...), " ")
			switch {
			case key == "gh --version":
				return []byte("gh version 2.40.0"), nil
			case key == "gh auth status":
				return []byte("  ✓ Token: gho_xxx\n  ✓ Token scopes: 'read:packages', 'repo'\n"), nil
			case key == "gh auth token":
				return []byte("gho_test_token_123\n"), nil
			default:
				return nil, fmt.Errorf("unexpected command: %s", key)
			}
		},
	}

	token, err := g.Login("@my-org")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "gho_test_token_123" {
		t.Fatalf("expected token gho_test_token_123, got %q", token)
	}
}

func TestGhImport_MissingScope_AcceptRefresh(t *testing.T) {
	refreshCalled := false
	g := &GhImport{
		cmdRunner: func(name string, args ...string) ([]byte, error) {
			key := strings.Join(append([]string{name}, args...), " ")
			switch {
			case key == "gh --version":
				return []byte("gh version 2.40.0"), nil
			case key == "gh auth status":
				return []byte("  ✓ Token: gho_xxx\n  ✓ Token scopes: 'repo'\n"), nil
			case key == "gh auth token":
				return []byte("gho_refreshed_token\n"), nil
			default:
				return nil, fmt.Errorf("unexpected command: %s", key)
			}
		},
		interactiveRunner: func(name string, args ...string) error {
			key := strings.Join(append([]string{name}, args...), " ")
			if key == "gh auth refresh --hostname github.com -s read:packages" {
				refreshCalled = true
				return nil
			}
			return fmt.Errorf("unexpected interactive command: %s", key)
		},
		prompter: func(msg string) (bool, error) {
			return true, nil // accept
		},
	}

	token, err := g.Login("@my-org")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !refreshCalled {
		t.Fatal("expected gh auth refresh to be called")
	}
	if token != "gho_refreshed_token" {
		t.Fatalf("expected token gho_refreshed_token, got %q", token)
	}
}

func TestGhImport_MissingScope_DeclineRefresh(t *testing.T) {
	g := &GhImport{
		cmdRunner: func(name string, args ...string) ([]byte, error) {
			key := strings.Join(append([]string{name}, args...), " ")
			switch {
			case key == "gh --version":
				return []byte("gh version 2.40.0"), nil
			case key == "gh auth status":
				return []byte("  ✓ Token: gho_xxx\n  ✓ Token scopes: 'repo'\n"), nil
			default:
				return nil, fmt.Errorf("unexpected command: %s", key)
			}
		},
		prompter: func(msg string) (bool, error) {
			return false, nil // decline
		},
	}

	_, err := g.Login("@my-org")
	if !errors.Is(err, auth.ErrProviderDeclined) {
		t.Fatalf("expected ErrProviderDeclined, got: %v", err)
	}
}
