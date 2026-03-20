package provider

import (
	"strings"
	"testing"

	providerconfig "github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/config"
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

	if data.ImagePolicy.Mode != "reuse-only" {
		t.Fatalf("expected default image policy reuse-only, got %q", data.ImagePolicy.Mode)
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
			name: "manual network without mask",
			data: ProviderData{
				Flavor:             "small",
				Networks:           []NetworkRef{{Name: "prod-lan"}},
				NetworkContextMode: "manual",
				StaticNetwork: []StaticNIC{{
					Name: "eth0",
					IP:   "172.22.0.200",
				}},
			},
			want: "staticNetwork[0].mask is required",
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
			name: "v1alpha1 with v1alpha2 fields",
			data: ProviderData{
				SchemaVersion: "v1alpha1",
				Flavor:        "small",
				Networks:      []NetworkRef{{Name: "prod-lan", Profile: "prod"}},
			},
			want: "v1alpha2-only fields",
		},
		{
			name: "placement host disabled",
			data: ProviderData{
				SchemaVersion: "v1alpha2",
				Flavor:        "small",
				Networks:      []NetworkRef{{Name: "prod-lan"}},
				Placement:     PlacementPolicy{Host: "compute-01"},
			},
			want: "placement.host is disabled",
		},
		{
			name: "additional disk too large",
			data: ProviderData{
				SchemaVersion:   "v1alpha2",
				Flavor:          "small",
				Networks:        []NetworkRef{{Name: "prod-lan"}},
				AdditionalDisks: []AdditionalDisk{{SizeGiB: 200}},
			},
			want: "additionalDisks[0].sizeGiB exceeds provider limit",
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

func TestValidateProviderDataResolvesProfilesAndModes(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.PlacementPolicies.AllowHostOverride = true
	cfg.PlacementPolicies.AllowedHosts = []string{"compute-01"}
	cfg.Features.HardDelete = true
	cfg.ImageManagement.ImportOnMiss = true

	data := &ProviderData{
		SchemaVersion: "v1alpha2",
		Flavor:        "small",
		Networks: []NetworkRef{
			{Profile: "prod", Model: "e1000"},
		},
		Placement:       PlacementPolicy{Host: "compute-01"},
		AdditionalDisks: []AdditionalDisk{{Name: "state", SizeGiB: 20}},
		Lifecycle:       LifecyclePolicy{DeleteMode: "hard"},
	}

	if err := ValidateProviderData(data, cfg); err != nil {
		t.Fatalf("ValidateProviderData() error = %v", err)
	}

	if data.Networks[0].Name != "prod-lan" {
		t.Fatalf("expected network profile to resolve name, got %+v", data.Networks[0])
	}

	if data.NetworkContextMode != "auto" {
		t.Fatalf("expected network mode from profile/defaults to resolve to auto, got %q", data.NetworkContextMode)
	}

	if data.ImagePolicy.Mode != "reuse-or-import" {
		t.Fatalf("expected image policy reuse-or-import, got %q", data.ImagePolicy.Mode)
	}
}

func TestValidateProviderDataDerivesManualNetwork(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.OpenNebula.AllowedNetworks = []string{"manual-lan"}

	data := &ProviderData{
		SchemaVersion:      "v1alpha2",
		Flavor:             "small",
		NetworkContextMode: "manual",
		Networks:           []NetworkRef{{Name: "manual-lan"}},
		StaticNetwork: []StaticNIC{{
			Name: "eth0",
			IP:   "172.22.0.200",
			Mask: "255.255.255.0",
		}},
	}

	if err := ValidateProviderData(data, cfg); err != nil {
		t.Fatalf("ValidateProviderData() error = %v", err)
	}

	if data.StaticNetwork[0].Network != "172.22.0.0" {
		t.Fatalf("expected derived network 172.22.0.0, got %q", data.StaticNetwork[0].Network)
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
		Limits:   providerconfig.LimitsConfig{MaxVCPU: 8, MaxMemoryMiB: 16384, MaxRootDiskGiB: 200},
	})
	if err != nil {
		t.Fatalf("ResolveResources() explicit error = %v", err)
	}

	if fromExplicit.VCPU != 4 || fromExplicit.MemoryMiB != 8192 {
		t.Fatalf("unexpected explicit resources: %+v", fromExplicit)
	}
}

func TestHostnamePatch(t *testing.T) {
	t.Parallel()

	patch := string(HostnameConfigPatch("worker-01"))
	if !strings.Contains(patch, "hostname: worker-01") {
		t.Fatalf("expected hostname in patch, got %q", patch)
	}
}

func TestRenderTemplateAndRedaction(t *testing.T) {
	t.Parallel()

	rendered := RenderTemplate(RenderInput{
		VMName:          "worker-01",
		MachineUUID:     "123e4567-e89b-12d3-a456-426614174000",
		Hypervisor:      "qemu",
		ImageName:       "talos-image",
		Datastore:       "fast-ssd",
		Resources:       ResolvedResources{CPU: "2", VCPU: 2, MemoryMiB: 4096, RootDiskGiB: 40},
		Networks:        []RenderedNetwork{{Name: "prod-lan", Model: "e1000"}},
		FirmwareMode:    "uefi",
		SecureBoot:      true,
		GraphicsEnabled: false,
		Placement: ResolvedPlacement{
			SchedRequirements: `NAME = "compute-01" & CLUSTER = "cluster-a"`,
			VMGroupName:       "control-plane",
			VMGroupRole:       "master",
		},
		AdditionalDisks: []AdditionalDisk{{Name: "state", SizeGiB: 20, Format: "qcow2"}},
		ContextKV: map[string]string{
			"ETH0_GATEWAY":       "172.22.0.1",
			"ETH0_IP":            "172.22.0.200",
			"ETH0_METHOD":        "static",
			"ETH0_MASK":          "255.255.255.0",
			"ETH0_NETWORK":       "172.22.0.0",
			"NETWORK":            "NO",
			"SET_HOSTNAME":       "worker-01",
			"USER_DATA":          "sensitive",
			"USER_DATA_ENCODING": "base64",
		},
	})

	if !strings.Contains(rendered, "NETWORK = \"NO\"") {
		t.Fatalf("expected NETWORK context, got %q", rendered)
	}

	if !strings.Contains(rendered, "HYPERVISOR = \"qemu\"") {
		t.Fatalf("expected hypervisor rendering, got %q", rendered)
	}

	if !strings.Contains(rendered, "ETH0_MASK = \"255.255.255.0\"") {
		t.Fatalf("expected manual network mask rendering, got %q", rendered)
	}

	if !strings.Contains(rendered, "ETH0_NETWORK = \"172.22.0.0\"") {
		t.Fatalf("expected manual network CIDR rendering, got %q", rendered)
	}

	if !strings.Contains(rendered, "ETH0_METHOD = \"static\"") {
		t.Fatalf("expected manual network method rendering, got %q", rendered)
	}

	if !strings.Contains(rendered, "TYPE = \"none\"") {
		t.Fatalf("expected graphics disabled rendering, got %q", rendered)
	}

	if !strings.Contains(rendered, `SCHED_REQUIREMENTS = "NAME = \"compute-01\" & CLUSTER = \"cluster-a\""`) {
		t.Fatalf("expected placement requirements, got %q", rendered)
	}

	if !strings.Contains(rendered, `VMGROUP_NAME = "control-plane"`) {
		t.Fatalf("expected vmgroup rendering, got %q", rendered)
	}

	if !strings.Contains(rendered, `MODEL = "e1000"`) {
		t.Fatalf("expected nic model rendering, got %q", rendered)
	}

	if !strings.Contains(rendered, `UUID = "123e4567-e89b-12d3-a456-426614174000"`) {
		t.Fatalf("expected machine UUID rendering, got %q", rendered)
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
		NetworkProfiles: map[string]providerconfig.NetworkProfile{
			"prod": {
				NetworkName: "prod-lan",
				Model:       "virtio",
				ContextMode: "auto",
			},
		},
		Limits: providerconfig.LimitsConfig{
			MaxRootDiskGiB:       200,
			MaxAdditionalDisks:   2,
			MaxAdditionalDiskGiB: 100,
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
