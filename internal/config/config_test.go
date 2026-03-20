package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConfigMarshalIncludesPackageAuthMetadataOnly(t *testing.T) {
	cfg := Config{
		ActiveVersion:   "22.14.0",
		PreviousVersion: "20.18.3",
		PackageAuth: PackageAuthConfig{
			Enabled: true,
			Registries: []PackageAuthRegistry{{
				Registry: "https://registry.npmjs.org/",
				Scope:    "@my-org",
				Enabled:  true,
			}},
		},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	jsonText := string(data)
	if !strings.Contains(jsonText, "package_auth") {
		t.Fatalf("expected package_auth in json: %s", jsonText)
	}
	if strings.Contains(strings.ToLower(jsonText), "token") {
		t.Fatalf("config json unexpectedly contains token-like data: %s", jsonText)
	}
}
