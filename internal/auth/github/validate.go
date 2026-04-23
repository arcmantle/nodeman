package github

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/arcmantle/nodeman/internal/httputil"
)

// TokenStatus describes the result of validating a GitHub token.
type TokenStatus struct {
	Valid   bool
	Scopes  []string
	Message string
}

// ValidateToken checks a GitHub token by calling the GitHub API.
// Returns the token's validity and scopes. Only makes sense for GitHub registries.
func ValidateToken(token string) TokenStatus {
	client := httputil.NewClient(5 * time.Second)

	req, err := http.NewRequest("GET", "https://api.github.com/", nil)
	if err != nil {
		return TokenStatus{Valid: false, Message: fmt.Sprintf("failed to create request: %s", err)}
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return TokenStatus{Valid: false, Message: "network error (validity unknown)"}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return TokenStatus{Valid: false, Message: "token is expired or revoked"}
	}

	if resp.StatusCode != http.StatusOK {
		return TokenStatus{Valid: false, Message: fmt.Sprintf("unexpected status %d", resp.StatusCode)}
	}

	scopes := parseScopes(resp.Header.Get("X-OAuth-Scopes"))
	hasReadPackages := false
	for _, s := range scopes {
		if s == "read:packages" {
			hasReadPackages = true
			break
		}
	}

	if !hasReadPackages {
		return TokenStatus{
			Valid:   true,
			Scopes:  scopes,
			Message: "token valid but missing read:packages scope",
		}
	}

	return TokenStatus{
		Valid:   true,
		Scopes:  scopes,
		Message: "token valid",
	}
}

// IsGitHubRegistry returns true if the registry URL is a GitHub Packages registry.
func IsGitHubRegistry(registry string) bool {
	lower := strings.ToLower(registry)
	return strings.Contains(lower, "npm.pkg.github.com")
}

func parseScopes(header string) []string {
	if header == "" {
		return nil
	}
	parts := strings.Split(header, ",")
	scopes := make([]string, 0, len(parts))
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s != "" {
			scopes = append(scopes, s)
		}
	}
	return scopes
}
