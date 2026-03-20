// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package provider

import (
	"fmt"
	"net"
	"strings"

	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/config"
)

const (
	schemaV1Alpha1 = "v1alpha1"
	schemaV1Alpha2 = "v1alpha2"
)

// ValidateProviderData validates and applies policy to providerData.
func ValidateProviderData(data *ProviderData, cfg config.Config) error {
	if data.SchemaVersion == "" {
		data.SchemaVersion = schemaV1Alpha1
	}

	switch data.SchemaVersion {
	case schemaV1Alpha1, schemaV1Alpha2:
	default:
		return fmt.Errorf("unsupported schemaVersion %q", data.SchemaVersion)
	}

	if data.SchemaVersion == schemaV1Alpha1 && hasV1Alpha2Fields(data) {
		return fmt.Errorf("v1alpha2-only fields require schemaVersion %q", schemaV1Alpha2)
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

	if !cfg.AllowedTemplate(data.TemplateName) {
		return fmt.Errorf("template %q is not allowed by runtime config", data.TemplateName)
	}

	if err := normalizeNetworks(data, cfg); err != nil {
		return err
	}

	if data.Datastore == "" {
		data.Datastore = cfg.StoragePolicies.DefaultDatastore
	}

	if data.Datastore != "" && !cfg.AllowedDatastore(data.Datastore) {
		return fmt.Errorf("datastore %q is not allowed by runtime config", data.Datastore)
	}

	if data.NetworkContextMode == "manual" && len(data.StaticNetwork) == 0 {
		return fmt.Errorf("manual networkContextMode requires staticNetwork entries")
	}
	if data.NetworkContextMode == "manual" && cfg.Policy.ManualNetworking.Mode == config.ManualNetworkingDeny {
		return fmt.Errorf("manual networkContextMode is disabled by provider config")
	}

	if err := normalizeStaticNetwork(data); err != nil {
		return err
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

	if err := validateImagePolicy(data, cfg); err != nil {
		return err
	}

	if err := validatePlacement(data, cfg); err != nil {
		return err
	}

	if err := validateAdditionalDisks(data, cfg); err != nil {
		return err
	}

	if err := validateLifecycle(data, cfg); err != nil {
		return err
	}

	return nil
}

func normalizeNetworks(data *ProviderData, cfg config.Config) error {
	originalMode := data.NetworkContextMode
	if data.NetworkContextMode == "" {
		data.NetworkContextMode = cfg.Defaults.NetworkContextMode
	}

	if len(data.Networks) == 0 {
		return fmt.Errorf("at least one network is required")
	}

	resolvedModes := map[string]struct{}{}
	for index := range data.Networks {
		network := &data.Networks[index]
		if network.Profile != "" {
			profile, ok := cfg.ResolveNetworkProfile(network.Profile)
			if !ok {
				return fmt.Errorf("network profile %q is not defined", network.Profile)
			}

			if network.Name == "" {
				network.Name = profile.NetworkName
			}
			if network.Model == "" {
				network.Model = profile.Model
			}
			if network.Mode == "" {
				network.Mode = profile.ContextMode
			}

			if data.NetworkContextMode == "manual" && cfg.Policy.ManualNetworking.Mode == config.ManualNetworkingRequireValidation && !profile.ManualValidated {
				return fmt.Errorf("network profile %q is not validated for manual networking", network.Profile)
			}
		}

		if strings.TrimSpace(network.Name) == "" {
			return fmt.Errorf("network name cannot be empty")
		}
		if !cfg.AllowedNetwork(network.Name) {
			return fmt.Errorf("network %q is not allowed by runtime config", network.Name)
		}

		if network.Mode != "" {
			if network.Mode != "auto" && network.Mode != "manual" {
				return fmt.Errorf("network %q mode must be %q or %q", network.Name, "auto", "manual")
			}
			resolvedModes[network.Mode] = struct{}{}
		}
	}

	switch data.NetworkContextMode {
	case "auto", "manual":
	default:
		return fmt.Errorf("unsupported networkContextMode %q", data.NetworkContextMode)
	}

	if len(resolvedModes) > 1 {
		return fmt.Errorf("all networks must use the same mode when networks[*].mode is set")
	}

	for mode := range resolvedModes {
		if originalMode != "" && originalMode != mode {
			return fmt.Errorf("networkContextMode %q conflicts with network mode %q", data.NetworkContextMode, mode)
		}
		data.NetworkContextMode = mode
	}

	return nil
}

func validateImagePolicy(data *ProviderData, cfg config.Config) error {
	if data.ImagePolicy.Mode == "" {
		if cfg.ImageManagement.ImportOnMiss {
			data.ImagePolicy.Mode = "reuse-or-import"
		} else {
			data.ImagePolicy.Mode = "reuse-only"
		}
	}

	switch data.ImagePolicy.Mode {
	case "reuse-only":
	case "reuse-or-import":
		if data.ImagePolicy.Mode == "reuse-or-import" && !cfg.ImageManagement.ImportOnMiss {
			return fmt.Errorf("imagePolicy.mode %q is not allowed because image import is disabled", data.ImagePolicy.Mode)
		}
	default:
		return fmt.Errorf("unsupported imagePolicy.mode %q", data.ImagePolicy.Mode)
	}

	return nil
}

func normalizeStaticNetwork(data *ProviderData) error {
	if data.NetworkContextMode != "manual" {
		return nil
	}

	for index := range data.StaticNetwork {
		nic := &data.StaticNetwork[index]

		if nic.IP == "" {
			continue
		}

		if parsedIP := net.ParseIP(strings.TrimSpace(nic.IP)).To4(); parsedIP == nil {
			return fmt.Errorf("staticNetwork[%d].ip must be a valid IPv4 address", index)
		}

		if nic.Mask == "" {
			return fmt.Errorf("staticNetwork[%d].mask is required for manual IPv4 networking", index)
		}

		if parsedMask := net.ParseIP(strings.TrimSpace(nic.Mask)).To4(); parsedMask == nil {
			return fmt.Errorf("staticNetwork[%d].mask must be a valid IPv4 netmask", index)
		}

		if nic.Network != "" {
			if parsedNetwork := net.ParseIP(strings.TrimSpace(nic.Network)).To4(); parsedNetwork == nil {
				return fmt.Errorf("staticNetwork[%d].network must be a valid IPv4 network", index)
			}

			continue
		}

		derivedNetwork, err := deriveNetworkCIDR(nic.IP, nic.Mask)
		if err != nil {
			return fmt.Errorf("staticNetwork[%d]: %w", index, err)
		}

		nic.Network = derivedNetwork
	}

	return nil
}

func validatePlacement(data *ProviderData, cfg config.Config) error {
	if data.Placement.Host != "" {
		if !cfg.PlacementPolicies.AllowHostOverride {
			return fmt.Errorf("placement.host is disabled by runtime config")
		}
		if !cfg.AllowedHost(data.Placement.Host) {
			return fmt.Errorf("placement.host %q is not allowed by runtime config", data.Placement.Host)
		}
	}

	if data.Placement.Cluster != "" {
		if !cfg.PlacementPolicies.AllowClusterOverride {
			return fmt.Errorf("placement.cluster is disabled by runtime config")
		}
		if !cfg.AllowedCluster(data.Placement.Cluster) {
			return fmt.Errorf("placement.cluster %q is not allowed by runtime config", data.Placement.Cluster)
		}
	}

	if data.Placement.VMGroup != "" {
		if !cfg.PlacementPolicies.AllowVMGroupOverride {
			return fmt.Errorf("placement.vmGroup is disabled by runtime config")
		}
		if !cfg.AllowedVMGroup(data.Placement.VMGroup) {
			return fmt.Errorf("placement.vmGroup %q is not allowed by runtime config", data.Placement.VMGroup)
		}
	}

	if data.Placement.Role != "" {
		return fmt.Errorf("placement.role is not supported yet")
	}

	return nil
}

func validateAdditionalDisks(data *ProviderData, cfg config.Config) error {
	if len(data.AdditionalDisks) == 0 {
		return nil
	}

	if cfg.Limits.MaxAdditionalDisks > 0 && len(data.AdditionalDisks) > cfg.Limits.MaxAdditionalDisks {
		return fmt.Errorf("additionalDisks exceeds provider limit of %d", cfg.Limits.MaxAdditionalDisks)
	}

	for index, disk := range data.AdditionalDisks {
		if disk.SizeGiB <= 0 {
			return fmt.Errorf("additionalDisks[%d].sizeGiB must be > 0", index)
		}
		if cfg.Limits.MaxAdditionalDiskGiB > 0 && disk.SizeGiB > cfg.Limits.MaxAdditionalDiskGiB {
			return fmt.Errorf("additionalDisks[%d].sizeGiB exceeds provider limit of %d", index, cfg.Limits.MaxAdditionalDiskGiB)
		}

		if disk.Format == "" {
			data.AdditionalDisks[index].Format = "qcow2"
		}

		switch data.AdditionalDisks[index].Format {
		case "qcow2", "raw":
		default:
			return fmt.Errorf("additionalDisks[%d].format must be %q or %q", index, "qcow2", "raw")
		}
	}

	return nil
}

func validateLifecycle(data *ProviderData, cfg config.Config) error {
	if data.Lifecycle.DeleteMode == "" {
		data.Lifecycle.DeleteMode = "terminate"
		if cfg.Features.HardDelete {
			data.Lifecycle.DeleteMode = "hard"
		}
	}

	switch data.Lifecycle.DeleteMode {
	case "normal":
		data.Lifecycle.DeleteMode = "terminate"
	case "terminate":
	case "hard":
		if data.Lifecycle.DeleteMode == "hard" && !cfg.Features.HardDelete {
			return fmt.Errorf("lifecycle.deleteMode %q is not allowed by runtime config", data.Lifecycle.DeleteMode)
		}
	default:
		return fmt.Errorf("unsupported lifecycle.deleteMode %q", data.Lifecycle.DeleteMode)
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

		if cfg.Limits.MaxVCPU > 0 && data.Resources.VCPU > cfg.Limits.MaxVCPU {
			return ResolvedResources{}, fmt.Errorf("resources.vcpu exceeds provider limit of %d", cfg.Limits.MaxVCPU)
		}
		if cfg.Limits.MaxMemoryMiB > 0 && data.Resources.MemoryMiB > cfg.Limits.MaxMemoryMiB {
			return ResolvedResources{}, fmt.Errorf("resources.memoryMiB exceeds provider limit of %d", cfg.Limits.MaxMemoryMiB)
		}
		if cfg.Limits.MaxRootDiskGiB > 0 && data.Resources.RootDiskGiB > cfg.Limits.MaxRootDiskGiB {
			return ResolvedResources{}, fmt.Errorf("resources.rootDiskGiB exceeds provider limit of %d", cfg.Limits.MaxRootDiskGiB)
		}
		if cfg.Policy.Minimums.VCPU > 0 && data.Resources.VCPU < cfg.Policy.Minimums.VCPU {
			return ResolvedResources{}, fmt.Errorf("resources.vcpu is below provider minimum of %d", cfg.Policy.Minimums.VCPU)
		}
		if cfg.Policy.Minimums.MemoryMiB > 0 && data.Resources.MemoryMiB < cfg.Policy.Minimums.MemoryMiB {
			return ResolvedResources{}, fmt.Errorf("resources.memoryMiB is below provider minimum of %d", cfg.Policy.Minimums.MemoryMiB)
		}
		if cfg.Policy.Minimums.RootDiskGiB > 0 && data.Resources.RootDiskGiB < cfg.Policy.Minimums.RootDiskGiB {
			return ResolvedResources{}, fmt.Errorf("resources.rootDiskGiB is below provider minimum of %d", cfg.Policy.Minimums.RootDiskGiB)
		}
		if cfg.Policy.Maximums.VCPU > 0 && data.Resources.VCPU > cfg.Policy.Maximums.VCPU {
			return ResolvedResources{}, fmt.Errorf("resources.vcpu exceeds provider maximum of %d", cfg.Policy.Maximums.VCPU)
		}
		if cfg.Policy.Maximums.MemoryMiB > 0 && data.Resources.MemoryMiB > cfg.Policy.Maximums.MemoryMiB {
			return ResolvedResources{}, fmt.Errorf("resources.memoryMiB exceeds provider maximum of %d", cfg.Policy.Maximums.MemoryMiB)
		}
		if cfg.Policy.Maximums.RootDiskGiB > 0 && data.Resources.RootDiskGiB > cfg.Policy.Maximums.RootDiskGiB {
			return ResolvedResources{}, fmt.Errorf("resources.rootDiskGiB exceeds provider maximum of %d", cfg.Policy.Maximums.RootDiskGiB)
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

	if cfg.Limits.MaxRootDiskGiB > 0 && rootDiskGiB > cfg.Limits.MaxRootDiskGiB {
		return ResolvedResources{}, fmt.Errorf("rootDiskGiB exceeds provider limit of %d", cfg.Limits.MaxRootDiskGiB)
	}
	if cfg.Policy.Minimums.VCPU > 0 && flavor.VCPU < cfg.Policy.Minimums.VCPU {
		return ResolvedResources{}, fmt.Errorf("flavor %q vcpu is below provider minimum of %d", data.Flavor, cfg.Policy.Minimums.VCPU)
	}
	if cfg.Policy.Minimums.MemoryMiB > 0 && flavor.MemoryMiB < cfg.Policy.Minimums.MemoryMiB {
		return ResolvedResources{}, fmt.Errorf("flavor %q memoryMiB is below provider minimum of %d", data.Flavor, cfg.Policy.Minimums.MemoryMiB)
	}
	if cfg.Policy.Minimums.RootDiskGiB > 0 && rootDiskGiB < cfg.Policy.Minimums.RootDiskGiB {
		return ResolvedResources{}, fmt.Errorf("rootDiskGiB is below provider minimum of %d", cfg.Policy.Minimums.RootDiskGiB)
	}
	if cfg.Policy.Maximums.VCPU > 0 && flavor.VCPU > cfg.Policy.Maximums.VCPU {
		return ResolvedResources{}, fmt.Errorf("flavor %q vcpu exceeds provider maximum of %d", data.Flavor, cfg.Policy.Maximums.VCPU)
	}
	if cfg.Policy.Maximums.MemoryMiB > 0 && flavor.MemoryMiB > cfg.Policy.Maximums.MemoryMiB {
		return ResolvedResources{}, fmt.Errorf("flavor %q memoryMiB exceeds provider maximum of %d", data.Flavor, cfg.Policy.Maximums.MemoryMiB)
	}
	if cfg.Policy.Maximums.RootDiskGiB > 0 && rootDiskGiB > cfg.Policy.Maximums.RootDiskGiB {
		return ResolvedResources{}, fmt.Errorf("rootDiskGiB exceeds provider maximum of %d", cfg.Policy.Maximums.RootDiskGiB)
	}

	return ResolvedResources{
		CPU:         flavor.CPU,
		VCPU:        flavor.VCPU,
		MemoryMiB:   flavor.MemoryMiB,
		RootDiskGiB: rootDiskGiB,
	}, nil
}

func hasV1Alpha2Fields(data *ProviderData) bool {
	if data.ImagePolicy.Mode != "" || data.Placement.Host != "" || data.Placement.Cluster != "" || data.Placement.VMGroup != "" || data.Placement.Role != "" || len(data.AdditionalDisks) > 0 || data.Lifecycle.DeleteMode != "" {
		return true
	}

	for _, network := range data.Networks {
		if network.Profile != "" || network.Mode != "" || network.Model != "" {
			return true
		}
	}

	return false
}
