package auth

import "errors"

// ErrProviderUnavailable indicates a provider's prerequisites are not met (e.g. gh CLI not installed).
var ErrProviderUnavailable = errors.New("auth provider unavailable")

// ErrProviderDeclined indicates the user declined an interactive step offered by the provider.
var ErrProviderDeclined = errors.New("auth provider declined by user")

// AuthProvider represents a strategy for obtaining a registry auth token.
type AuthProvider interface {
	Name() string
	Method() string
	Registry() string
	Login(scope string) (token string, err error)
}

var providers = map[string][]AuthProvider{}

// RegisterProvider adds an auth provider for the given provider name.
// Multiple providers can be registered under the same name (e.g. gh-import and device-flow for "github").
// They are tried in registration order.
func RegisterProvider(name string, p AuthProvider) {
	providers[name] = append(providers[name], p)
}

// ProvidersByName returns the providers registered under the given name, in registration order.
func ProvidersByName(name string) ([]AuthProvider, bool) {
	ps, ok := providers[name]
	return ps, ok
}

// RegisteredProviders returns the names of all registered provider groups.
func RegisteredProviders() []string {
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	return names
}
