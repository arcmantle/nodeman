package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/arcmantle/nodeman/internal/auth"
	"github.com/arcmantle/nodeman/internal/httputil"
)

// ClientID is the public client ID for the arcmantle nodeman GitHub OAuth App.
// This is safe to embed in source — it's a public client with no secret.
const ClientID = "Ov23liB4vU3ptnXoDoTC"

// DeviceFlow obtains a GitHub Packages token via the OAuth Device Authorization Flow (RFC 8628).
type DeviceFlow struct {
	// Overridable for testing.
	codeURL  string
	tokenURL string
	clientID string
	client   *http.Client
	openURL  func(string) error
	writer   io.Writer
}

// DeviceFlowOption configures the DeviceFlow provider.
type DeviceFlowOption func(*DeviceFlow)

// WithEndpoints overrides the GitHub OAuth endpoints (for testing).
func WithEndpoints(codeURL, tokenURL string) DeviceFlowOption {
	return func(d *DeviceFlow) {
		d.codeURL = codeURL
		d.tokenURL = tokenURL
	}
}

// WithClientID overrides the OAuth client ID.
func WithClientID(id string) DeviceFlowOption {
	return func(d *DeviceFlow) { d.clientID = id }
}

// WithHTTPClient overrides the HTTP client.
func WithHTTPClient(c *http.Client) DeviceFlowOption {
	return func(d *DeviceFlow) { d.client = c }
}

// WithOpenURL overrides the browser-open function.
func WithOpenURL(fn func(string) error) DeviceFlowOption {
	return func(d *DeviceFlow) { d.openURL = fn }
}

// WithWriter overrides the output writer for user instructions.
func WithWriter(w io.Writer) DeviceFlowOption {
	return func(d *DeviceFlow) { d.writer = w }
}

// NewDeviceFlow creates a DeviceFlow provider with the given options.
func NewDeviceFlow(opts ...DeviceFlowOption) *DeviceFlow {
	d := &DeviceFlow{
		codeURL:  "https://github.com/login/device/code",
		tokenURL: "https://github.com/login/oauth/access_token",
		clientID: ClientID,
		client:   httputil.NewClient(30 * time.Second),
		openURL:  openBrowser,
		writer:   nil, // set lazily to os.Stderr
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

func (d *DeviceFlow) Name() string     { return "github" }
func (d *DeviceFlow) Method() string   { return "device flow" }
func (d *DeviceFlow) Registry() string { return ghRegistry }

func (d *DeviceFlow) w() io.Writer {
	if d.writer != nil {
		return d.writer
	}
	return os.Stderr
}

type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
}

// Login performs the device flow and returns an access token.
// scope is the npm scope (e.g. @my-org) for which auth is being configured;
// the OAuth scope (read:packages) is always requested regardless.
func (d *DeviceFlow) Login(_ string) (string, error) {
	// Step 1: Request device code
	codeResp, err := d.requestDeviceCode()
	if err != nil {
		return "", fmt.Errorf("requesting device code: %w", err)
	}

	// Step 2: Show user instructions
	w := d.w()
	fmt.Fprintf(w, "\nTo authenticate, open this URL in your browser:\n\n")
	fmt.Fprintf(w, "  %s\n\n", codeResp.VerificationURI)
	fmt.Fprintf(w, "And enter this code: %s\n\n", codeResp.UserCode)

	// Try to open browser automatically
	if err := d.openURL(codeResp.VerificationURI); err == nil {
		fmt.Fprintf(w, "(Browser opened automatically)\n\n")
	}

	fmt.Fprintf(w, "Waiting for authorization...\n")

	// Step 3: Poll for token
	interval := time.Duration(codeResp.Interval) * time.Second
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(codeResp.ExpiresIn) * time.Second)

	for {
		if time.Now().After(deadline) {
			return "", fmt.Errorf("device flow authorization expired — please try again")
		}

		time.Sleep(interval)

		tokenResp, err := d.pollForToken(codeResp.DeviceCode)
		if err != nil {
			return "", fmt.Errorf("polling for token: %w", err)
		}

		switch tokenResp.Error {
		case "":
			// Success
			token := strings.TrimSpace(tokenResp.AccessToken)
			if token == "" {
				return "", fmt.Errorf("received empty access token from GitHub")
			}
			fmt.Fprintf(w, "Authorization successful!\n")
			return token, nil
		case "authorization_pending":
			continue
		case "slow_down":
			interval += 5 * time.Second
			continue
		case "expired_token":
			return "", fmt.Errorf("device flow authorization expired — please try again")
		case "access_denied":
			return "", fmt.Errorf("%w: user denied authorization", auth.ErrProviderDeclined)
		default:
			return "", fmt.Errorf("unexpected device flow error: %s", tokenResp.Error)
		}
	}
}

func (d *DeviceFlow) requestDeviceCode() (*deviceCodeResponse, error) {
	data := url.Values{
		"client_id": {d.clientID},
		"scope":     {"read:packages"},
	}

	req, err := http.NewRequest("POST", d.codeURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result deviceCodeResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if result.DeviceCode == "" || result.UserCode == "" {
		return nil, fmt.Errorf("incomplete device code response")
	}

	return &result, nil
}

func (d *DeviceFlow) pollForToken(deviceCode string) (*tokenResponse, error) {
	data := url.Values{
		"client_id":   {d.clientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}

	req, err := http.NewRequest("POST", d.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var result tokenResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	return &result, nil
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", url).Start()
	default:
		return fmt.Errorf("unsupported platform")
	}
}
