// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package provider

import (
	"fmt"
	"strings"

	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/config"
)

// ValidateProviderData validates and applies policy to providerData.
func ValidateProviderData(data *ProviderData, cfg config.Config) error {
	if data.SchemaVersion == "" {
		data.SchemaVersion = "v1alpha1"
	}

	if data.SchemaVersion != "v1alpha1" {
		return fmt.Errorf("unsupported schemaVersion %q", data.SchemaVersion)
	}

	if data.Flavor == "" && data.Resources == nil {
		data.Flavor = cfg.Defaults.Flavor
	}

	if data.Flavor == "" && data.Resources == nil {
		if cfg.Features.AllowExplicitResources {
			return fmt.Errorf("set flavor or resources")
		}

		return fmt.Errorf("flavor is required when explicit resources are disabled")
	}

	if data.Flavor != "" && data.Resources != nil {
		return fmt.Errorf("set either flavor or resources, not both")
	}

	if data.Flavor == "" && !cfg.Features.AllowExplicitResources {
		return fmt.Errorf("explicit resources are disabled by provider config")
	}

	if data.Flavor != "" {
		if _, ok := cfg.Flavors[data.Flavor]; !ok {
			return fmt.Errorf("unknown flavor %q", data.Flavor)
		}
	}

	if data.TemplateName == "" {
		data.TemplateName = cfg.OpenNebula.TemplateName
	}

	if data.NetworkContextMode == "" {
		data.NetworkContextMode = cfg.Defaults.NetworkContextMode
	}

	switch data.NetworkContextMode {
	case "auto", "manual":
	default:
		return fmt.Errorf("unsupported networkContextMode %q", data.NetworkContextMode)
	}

	if len(data.Networks) == 0 {
		return fmt.Errorf("at least one network is required")
	}

	for _, network := range data.Networks {
		if strings.TrimSpace(network.Name) == "" {
			return fmt.Errorf("network name cannot be empty")
		}
		if !cfg.AllowedNetwork(network.Name) {
			return fmt.Errorf("network %q is not allowed by runtime config", network.Name)
		}
	}

	if data.Datastore != "" && !cfg.AllowedDatastore(data.Datastore) {
		return fmt.Errorf("datastore %q is not allowed by runtime config", data.Datastore)
	}

	if data.NetworkContextMode == "manual" && len(data.StaticNetwork) == 0 {
		return fmt.Errorf("manual networkContextMode requires staticNetwork entries")
	}

	if data.Firmware.Mode == "" {
		data.Firmware.Mode = cfg.Defaults.Firmware
	}

	if data.Firmware.SecureBoot == nil {
		data.Firmware.SecureBoot = boolPtr(cfg.Defaults.SecureBoot)
	}

	if data.Graphics.Enabled == nil {
		data.Graphics.Enabled = boolPtr(cfg.Defaults.GraphicsEnabled)
	}

	if effectiveSecureBoot(data) && data.Firmware.Mode != "uefi" {
		return fmt.Errorf("secure boot requires uefi firmware")
	}

	if data.GPU != nil && data.GPU.Enabled && !cfg.Features.EnableGPU {
		return fmt.Errorf("gpu support is disabled")
	}

	return nil
}

func boolPtr(value bool) *bool {
	return &value
}

func effectiveSecureBoot(data *ProviderData) bool {
	return data.Firmware.SecureBoot != nil && *data.Firmware.SecureBoot
}

func effectiveGraphicsEnabled(data *ProviderData) bool {
	return data.Graphics.Enabled != nil && *data.Graphics.Enabled
}

// ResolveResources returns the effective VM sizing.
func ResolveResources(data ProviderData, cfg config.Config) (ResolvedResources, error) {
	if data.Resources != nil {
		if data.Resources.CPU == "" || data.Resources.VCPU <= 0 || data.Resources.MemoryMiB <= 0 || data.Resources.RootDiskGiB <= 0 {
			return ResolvedResources{}, fmt.Errorf("resources must define cpu, vcpu, memoryMiB, and rootDiskGiB")
		}

		return ResolvedResources{
			CPU:         data.Resources.CPU,
			VCPU:        data.Resources.VCPU,
			MemoryMiB:   data.Resources.MemoryMiB,
			RootDiskGiB: data.Resources.RootDiskGiB,
		}, nil
	}

	flavor := cfg.Flavors[data.Flavor]
	rootDiskGiB := flavor.RootDiskGiB
	if data.RootDiskGiB > 0 {
		rootDiskGiB = data.RootDiskGiB
	}

	return ResolvedResources{
		CPU:         flavor.CPU,
		VCPU:        flavor.VCPU,
		MemoryMiB:   flavor.MemoryMiB,
		RootDiskGiB: rootDiskGiB,
	}, nil
}
