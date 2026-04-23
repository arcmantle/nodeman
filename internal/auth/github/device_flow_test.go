package github

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/arcmantle/nodeman/internal/auth"
)

func TestDeviceFlow_Success(t *testing.T) {
	pollCount := atomic.Int32{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/device/code":
			json.NewEncoder(w).Encode(deviceCodeResponse{
				DeviceCode:      "dev-code-123",
				UserCode:        "ABCD-1234",
				VerificationURI: "https://github.com/login/device",
				ExpiresIn:       900,
				Interval:        0, // test uses minimum 5s but we override timing
			})
		case "/access_token":
			n := pollCount.Add(1)
			if n < 2 {
				json.NewEncoder(w).Encode(tokenResponse{Error: "authorization_pending"})
				return
			}
			json.NewEncoder(w).Encode(tokenResponse{
				AccessToken: "gho_device_flow_token",
				TokenType:   "bearer",
				Scope:       "read:packages",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	var output bytes.Buffer
	d := NewDeviceFlow(
		WithEndpoints(srv.URL+"/device/code", srv.URL+"/access_token"),
		WithClientID("test-client-id"),
		WithHTTPClient(srv.Client()),
		WithOpenURL(func(string) error { return nil }),
		WithWriter(&output),
	)

	// Override the sleep-based polling by running in a test with a fast server
	// The test server responds immediately, so the 5s sleep is the bottleneck.
	// For a fast test, we need to reduce the interval. We can't easily do that
	// without changing the implementation, so let's just test the request logic.
	// Instead, let's test with a direct call to the internal methods.

	// Test requestDeviceCode
	codeResp, err := d.requestDeviceCode()
	if err != nil {
		t.Fatalf("requestDeviceCode: %v", err)
	}
	if codeResp.DeviceCode != "dev-code-123" {
		t.Fatalf("expected device code dev-code-123, got %q", codeResp.DeviceCode)
	}
	if codeResp.UserCode != "ABCD-1234" {
		t.Fatalf("expected user code ABCD-1234, got %q", codeResp.UserCode)
	}

	// Test pollForToken — first call returns pending
	resp1, err := d.pollForToken("dev-code-123")
	if err != nil {
		t.Fatalf("pollForToken (1): %v", err)
	}
	if resp1.Error != "authorization_pending" {
		t.Fatalf("expected authorization_pending, got %q", resp1.Error)
	}

	// Second call returns the token
	resp2, err := d.pollForToken("dev-code-123")
	if err != nil {
		t.Fatalf("pollForToken (2): %v", err)
	}
	if resp2.AccessToken != "gho_device_flow_token" {
		t.Fatalf("expected token gho_device_flow_token, got %q", resp2.AccessToken)
	}
}

func TestDeviceFlow_SlowDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/access_token":
			json.NewEncoder(w).Encode(tokenResponse{Error: "slow_down"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	d := NewDeviceFlow(
		WithEndpoints(srv.URL+"/device/code", srv.URL+"/access_token"),
		WithClientID("test-client-id"),
		WithHTTPClient(srv.Client()),
	)

	resp, err := d.pollForToken("dev-code-123")
	if err != nil {
		t.Fatalf("pollForToken: %v", err)
	}
	if resp.Error != "slow_down" {
		t.Fatalf("expected slow_down, got %q", resp.Error)
	}
}

func TestDeviceFlow_AccessDenied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/device/code":
			json.NewEncoder(w).Encode(deviceCodeResponse{
				DeviceCode:      "dev-code-denied",
				UserCode:        "DENY-CODE",
				VerificationURI: "https://github.com/login/device",
				ExpiresIn:       900,
				Interval:        0,
			})
		case "/access_token":
			json.NewEncoder(w).Encode(tokenResponse{Error: "access_denied"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// We can test the error classification via pollForToken directly
	d := NewDeviceFlow(
		WithEndpoints(srv.URL+"/device/code", srv.URL+"/access_token"),
		WithClientID("test-client-id"),
		WithHTTPClient(srv.Client()),
	)

	resp, err := d.pollForToken("dev-code-denied")
	if err != nil {
		t.Fatalf("pollForToken: %v", err)
	}
	if resp.Error != "access_denied" {
		t.Fatalf("expected access_denied, got %q", resp.Error)
	}
}

func TestDeviceFlow_ExpiredToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/access_token":
			json.NewEncoder(w).Encode(tokenResponse{Error: "expired_token"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	d := NewDeviceFlow(
		WithEndpoints(srv.URL+"/device/code", srv.URL+"/access_token"),
		WithClientID("test-client-id"),
		WithHTTPClient(srv.Client()),
	)

	resp, err := d.pollForToken("dev-code-expired")
	if err != nil {
		t.Fatalf("pollForToken: %v", err)
	}
	if resp.Error != "expired_token" {
		t.Fatalf("expected expired_token, got %q", resp.Error)
	}
}

func TestDeviceFlow_BadCodeResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer srv.Close()

	d := NewDeviceFlow(
		WithEndpoints(srv.URL+"/device/code", srv.URL+"/access_token"),
		WithClientID("test-client-id"),
		WithHTTPClient(srv.Client()),
	)

	_, err := d.requestDeviceCode()
	if err == nil {
		t.Fatal("expected error for bad status code")
	}
}

func TestDeviceFlow_LoginAccessDenied(t *testing.T) {
	// Full Login() flow that immediately returns access_denied
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/device/code":
			json.NewEncoder(w).Encode(deviceCodeResponse{
				DeviceCode:      "dev-denied",
				UserCode:        "DENY",
				VerificationURI: "https://github.com/login/device",
				ExpiresIn:       900,
				Interval:        1, // 1 second for fast test
			})
		case "/access_token":
			json.NewEncoder(w).Encode(tokenResponse{Error: "access_denied"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	var output bytes.Buffer
	d := NewDeviceFlow(
		WithEndpoints(srv.URL+"/device/code", srv.URL+"/access_token"),
		WithClientID("test-client-id"),
		WithHTTPClient(srv.Client()),
		WithOpenURL(func(string) error { return nil }),
		WithWriter(&output),
	)

	_, err := d.Login("@my-org")
	if !errors.Is(err, auth.ErrProviderDeclined) {
		t.Fatalf("expected ErrProviderDeclined, got: %v", err)
	}
}
