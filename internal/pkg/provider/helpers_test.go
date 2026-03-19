package provider

import (
	"encoding/base64"
	"strings"
	"testing"

	providerconfig "github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/config"
	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/opennebula"
	"github.com/siderolabs/omni/client/pkg/infra/provision"
)

func TestCanonicalVMName(t *testing.T) {
	t.Parallel()

	name := CanonicalVMName("REQ_Prod/Node.01")
	if name != "req-prod-node-01" {
		t.Fatalf("expected normalized name, got %q", name)
	}

	longName := CanonicalVMName(strings.Repeat("Ab", 40))
	if len(longName) > 63 {
		t.Fatalf("expected long name to be truncated to <= 63 chars, got %d", len(longName))
	}
}

func TestValidateProviderDataAppliesDefaults(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	data := &ProviderData{
		Networks: []NetworkRef{{Name: "prod-lan"}},
	}

	if err := ValidateProviderData(data, cfg); err != nil {
		t.Fatalf("ValidateProviderData() error = %v", err)
	}

	if data.Flavor != "small" {
		t.Fatalf("expected default flavor small, got %q", data.Flavor)
	}

	if data.TemplateName != "talos-base" {
		t.Fatalf("expected default template name, got %q", data.TemplateName)
	}

	if data.NetworkContextMode != "auto" {
		t.Fatalf("expected default networkContextMode auto, got %q", data.NetworkContextMode)
	}

	if data.Firmware.Mode != "uefi" {
		t.Fatalf("expected default firmware uefi, got %q", data.Firmware.Mode)
	}

	if data.Firmware.SecureBoot == nil || *data.Firmware.SecureBoot {
		t.Fatalf("expected default secureBoot=false, got %+v", data.Firmware.SecureBoot)
	}

	if data.Graphics.Enabled == nil || *data.Graphics.Enabled {
		t.Fatalf("expected default graphics.enabled=false, got %+v", data.Graphics.Enabled)
	}
}

func TestValidateProviderDataRejectsInvalidCombinations(t *testing.T) {
	t.Parallel()

	cfg := testConfig()

	cases := []struct {
		name string
		data ProviderData
		want string
	}{
		{
			name: "both flavor and resources",
			data: ProviderData{
				Flavor:    "small",
				Resources: &ResourceOverrides{CPU: "2", VCPU: 2, MemoryMiB: 2048, RootDiskGiB: 20},
				Networks:  []NetworkRef{{Name: "prod-lan"}},
			},
			want: "either flavor or resources",
		},
		{
			name: "manual network without static config",
			data: ProviderData{
				Flavor:             "small",
				Networks:           []NetworkRef{{Name: "prod-lan"}},
				NetworkContextMode: "manual",
			},
			want: "manual networkContextMode requires staticNetwork entries",
		},
		{
			name: "gpu disabled",
			data: ProviderData{
				Flavor:   "small",
				Networks: []NetworkRef{{Name: "prod-lan"}},
				GPU:      &GPURequest{Enabled: true},
			},
			want: "gpu support is disabled",
		},
		{
			name: "secure boot on bios",
			data: ProviderData{
				Flavor:   "small",
				Networks: []NetworkRef{{Name: "prod-lan"}},
				Firmware: FirmwareConfig{
					Mode:       "bios",
					SecureBoot: boolPtr(true),
				},
			},
			want: "secure boot requires uefi firmware",
		},
		{
			name: "explicit resources missing values",
			data: ProviderData{
				Resources: &ResourceOverrides{CPU: "2"},
				Networks:  []NetworkRef{{Name: "prod-lan"}},
			},
			want: "",
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateProviderData(&tt.data, cfg)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("ValidateProviderData() unexpected error = %v", err)
				}
				if _, resolveErr := ResolveResources(tt.data, cfg); resolveErr == nil {
					t.Fatal("expected ResolveResources() to reject incomplete explicit resources")
				}
				return
			}

			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestResolveResources(t *testing.T) {
	t.Parallel()

	cfg := testConfig()

	fromFlavor, err := ResolveResources(ProviderData{Flavor: "small", RootDiskGiB: 80}, cfg)
	if err != nil {
		t.Fatalf("ResolveResources() flavor error = %v", err)
	}

	if fromFlavor.RootDiskGiB != 80 {
		t.Fatalf("expected root disk override from flavor path, got %d", fromFlavor.RootDiskGiB)
	}

	explicit := ProviderData{
		Resources: &ResourceOverrides{CPU: "4", VCPU: 4, MemoryMiB: 8192, RootDiskGiB: 100},
	}
	fromExplicit, err := ResolveResources(explicit, providerconfig.Config{
		Features: providerconfig.FeaturesConfig{AllowExplicitResources: true},
	})
	if err != nil {
		t.Fatalf("ResolveResources() explicit error = %v", err)
	}

	if fromExplicit.VCPU != 4 || fromExplicit.MemoryMiB != 8192 {
		t.Fatalf("unexpected explicit resources: %+v", fromExplicit)
	}
}

func TestBootstrapPayloadAndHostnamePatch(t *testing.T) {
	t.Parallel()

	payload := BootstrapPayload(provision.ConnectionParams{
		JoinConfig: "cluster:\n  id: test",
	}, "worker-01")

	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}

	decodedString := string(decoded)
	if !strings.Contains(decodedString, "cluster:\n  id: test") {
		t.Fatalf("expected join config in payload, got %q", decodedString)
	}

	if !strings.Contains(decodedString, "hostname: worker-01") {
		t.Fatalf("expected hostname in payload, got %q", decodedString)
	}

	patch := string(HostnameConfigPatch("worker-01"))
	if !strings.Contains(patch, "hostname: worker-01") {
		t.Fatalf("expected hostname in patch, got %q", patch)
	}
}

func TestRenderTemplateAndRedaction(t *testing.T) {
	t.Parallel()

	rendered := RenderTemplate(RenderInput{
		VMName:          "worker-01",
		ImageName:       "talos-image",
		Datastore:       "fast-ssd",
		Resources:       ResolvedResources{CPU: "2", VCPU: 2, MemoryMiB: 4096, RootDiskGiB: 40},
		Networks:        []opennebula.NetworkRef{{Name: "prod-lan"}},
		FirmwareMode:    "uefi",
		SecureBoot:      true,
		GraphicsEnabled: false,
		ContextKV: map[string]string{
			"NETWORK":            "YES",
			"USER_DATA":          "sensitive",
			"USER_DATA_ENCODING": "base64",
		},
	})

	if !strings.Contains(rendered, "NETWORK = \"YES\"") {
		t.Fatalf("expected NETWORK context, got %q", rendered)
	}

	if !strings.Contains(rendered, "TYPE = \"none\"") {
		t.Fatalf("expected graphics disabled rendering, got %q", rendered)
	}

	redacted := RedactTemplateForLog(rendered)
	if strings.Contains(redacted, "sensitive") {
		t.Fatalf("expected USER_DATA to be redacted, got %q", redacted)
	}

	if !strings.Contains(redacted, "USER_DATA = \"REDACTED\"") {
		t.Fatalf("expected redacted USER_DATA marker, got %q", redacted)
	}
}

func testConfig() providerconfig.Config {
	return providerconfig.Config{
		ProviderID: providerconfig.ProviderID,
		OpenNebula: providerconfig.OpenNebulaConfig{
			Endpoint:          "https://one.example.com/RPC2",
			TemplateName:      "talos-base",
			AllowedDatastores: []string{"fast-ssd"},
			AllowedNetworks:   []string{"prod-lan"},
			ImageNamePattern:  "talos-opennebula-{{ .Arch }}-{{ .TalosVersion }}-schematic-{{ .SchematicID }}",
		},
		Defaults: providerconfig.DefaultsConfig{
			Flavor:             "small",
			Firmware:           "uefi",
			SecureBoot:         false,
			GraphicsEnabled:    false,
			NetworkContextMode: "auto",
			HostnameStrategy:   "vm-name",
		},
		Features: providerconfig.FeaturesConfig{
			AllowExplicitResources: true,
		},
		Flavors: map[string]providerconfig.Flavor{
			"small": {
				CPU:         "2",
				VCPU:        2,
				MemoryMiB:   4096,
				RootDiskGiB: 40,
			},
		},
	}
}
