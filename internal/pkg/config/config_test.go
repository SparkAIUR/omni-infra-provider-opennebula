package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadAppliesDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := Load(strings.NewReader(`
providerID: opennebula
opennebula:
  endpoint: https://one.example.com/RPC2
  templateName: talos-base
defaults:
  flavor: small
flavors:
  small:
    cpu: "2"
    vcpu: 2
    memoryMiB: 4096
    rootDiskGiB: 40
`))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.OpenNebula.ImageNamePattern == "" {
		t.Fatal("expected default image name pattern")
	}

	if cfg.Defaults.Firmware != "uefi" {
		t.Fatalf("expected default firmware uefi, got %q", cfg.Defaults.Firmware)
	}

	if cfg.Defaults.NetworkContextMode != "auto" {
		t.Fatalf("expected default networkContextMode auto, got %q", cfg.Defaults.NetworkContextMode)
	}

	if cfg.Timeouts.Instantiate != 2*time.Minute {
		t.Fatalf("expected instantiate timeout 2m, got %s", cfg.Timeouts.Instantiate)
	}
}

func TestLoadRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	_, err := Load(strings.NewReader(`
providerID: opennebula
opennebula:
  templateName: talos-base
flavors:
  small:
    cpu: "2"
    vcpu: 2
    memoryMiB: 4096
    rootDiskGiB: 40
`))
	if err == nil || !strings.Contains(err.Error(), "opennebula.endpoint is required") {
		t.Fatalf("expected missing endpoint error, got %v", err)
	}
}

func TestResolveAuthPrefersSession(t *testing.T) {
	t.Setenv(SessionEnvVar, "session-token")
	t.Setenv(UsernameEnvVar, "user")
	t.Setenv(PasswordEnvVar, "pass")

	auth, err := ResolveAuth()
	if err != nil {
		t.Fatalf("ResolveAuth() error = %v", err)
	}

	if auth.Session != "session-token" {
		t.Fatalf("expected session auth, got %+v", auth)
	}

	if auth.Username != "" || auth.Password != "" {
		t.Fatalf("expected username/password to be ignored when session is set, got %+v", auth)
	}
}

func TestResolveAuthRequiresCredentials(t *testing.T) {
	t.Setenv(SessionEnvVar, "")
	t.Setenv(UsernameEnvVar, "")
	t.Setenv(PasswordEnvVar, "")

	_, err := ResolveAuth()
	if err == nil || !strings.Contains(err.Error(), "OPENNEBULA_SESSION") {
		t.Fatalf("expected auth error, got %v", err)
	}
}
