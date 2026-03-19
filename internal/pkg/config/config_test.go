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

	if cfg.ImageManagement.RetainGenerations != 2 {
		t.Fatalf("expected retainGenerations=2, got %d", cfg.ImageManagement.RetainGenerations)
	}

	if cfg.ImageManagement.PollInterval != 5*time.Second {
		t.Fatalf("expected image poll interval 5s, got %s", cfg.ImageManagement.PollInterval)
	}

	if cfg.Observability.ListenAddress != ":9977" {
		t.Fatalf("expected default listen address :9977, got %q", cfg.Observability.ListenAddress)
	}

	if cfg.Observability.MetricsPath != "/metrics" || cfg.Observability.HealthPath != "/healthz" || cfg.Observability.ReadyPath != "/readyz" {
		t.Fatalf("unexpected observability defaults: %+v", cfg.Observability)
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

func TestLoadRejectsInvalidObservabilityAndNetworkProfileConfig(t *testing.T) {
	t.Parallel()

	_, err := Load(strings.NewReader(`
providerID: opennebula
opennebula:
  endpoint: https://one.example.com/RPC2
  templateName: talos-base
observability:
  metricsPath: metrics
networkProfiles:
  prod:
    networkName: prod-lan
    contextMode: broken
flavors:
  small:
    cpu: "2"
    vcpu: 2
    memoryMiB: 4096
    rootDiskGiB: 40
`))
	if err == nil {
		t.Fatal("expected invalid config error")
	}

	if !strings.Contains(err.Error(), "networkProfiles.prod.contextMode") && !strings.Contains(err.Error(), "observability.metricsPath") {
		t.Fatalf("expected observability or network profile error, got %v", err)
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

	if auth.Mode() != "session" {
		t.Fatalf("expected session auth mode, got %q", auth.Mode())
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

func TestAuthRedacted(t *testing.T) {
	t.Parallel()

	auth := AuthConfig{
		Session:  "session-token",
		Username: "user",
		Password: "pass",
	}

	redacted := auth.Redacted()
	if redacted.Session != "REDACTED" || redacted.Username != "REDACTED" || redacted.Password != "REDACTED" {
		t.Fatalf("expected auth fields to be redacted, got %+v", redacted)
	}
}
