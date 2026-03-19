// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package config implements the runtime configuration for the OpenNebula provider.
package config

import (
	"errors"
	"fmt"
	"io"
	"time"

	"go.yaml.in/yaml/v4"
)

const (
	// ProviderID is the only supported provider identifier for this repository.
	ProviderID = "opennebula"
)

// Config describes the provider runtime configuration.
//
// Example:
//
//	providerID: opennebula
//	opennebula:
//	  endpoint: https://opennebula.example.com/RPC2
//	  templateName: talos-omni-base
//	  imageNamePattern: "talos-opennebula-{{ .Arch }}-{{ .TalosVersion }}-schematic-{{ .SchematicID }}"
type Config struct {
	ProviderID string            `yaml:"providerID"`
	OpenNebula OpenNebulaConfig  `yaml:"opennebula"`
	Defaults   DefaultsConfig    `yaml:"defaults"`
	Features   FeaturesConfig    `yaml:"features"`
	Timeouts   TimeoutsConfig    `yaml:"timeouts"`
	Flavors    map[string]Flavor `yaml:"flavors"`
}

// OpenNebulaConfig contains platform-specific defaults and allow-lists.
type OpenNebulaConfig struct {
	Endpoint          string   `yaml:"endpoint"`
	TemplateName      string   `yaml:"templateName"`
	AllowedDatastores []string `yaml:"allowedDatastores,omitempty"`
	AllowedNetworks   []string `yaml:"allowedNetworks,omitempty"`
	ImageNamePattern  string   `yaml:"imageNamePattern"`
}

// DefaultsConfig stores default request behavior.
type DefaultsConfig struct {
	Flavor             string `yaml:"flavor"`
	Firmware           string `yaml:"firmware"`
	SecureBoot         bool   `yaml:"secureBoot"`
	GraphicsEnabled    bool   `yaml:"graphicsEnabled"`
	NetworkContextMode string `yaml:"networkContextMode"`
	HostnameStrategy   string `yaml:"hostnameStrategy"`
}

// FeaturesConfig toggles optional behaviors.
type FeaturesConfig struct {
	AllowExplicitResources bool `yaml:"allowExplicitResources"`
	EnableGPU              bool `yaml:"enableGPU"`
	HardDelete             bool `yaml:"hardDelete"`
}

// TimeoutsConfig holds runtime operation timeouts.
type TimeoutsConfig struct {
	Instantiate time.Duration `yaml:"instantiate"`
	VMReady     time.Duration `yaml:"vmReady"`
	Terminate   time.Duration `yaml:"terminate"`
}

// Flavor describes a named VM shape.
type Flavor struct {
	CPU         string            `yaml:"cpu"`
	VCPU        int               `yaml:"vcpu"`
	MemoryMiB   int               `yaml:"memoryMiB"`
	RootDiskGiB int               `yaml:"rootDiskGiB"`
	Tags        map[string]string `yaml:"tags,omitempty"`
}

// AuthConfig contains resolved runtime authentication values.
type AuthConfig struct {
	Session  string
	Username string
	Password string
}

// Load reads, defaults, and validates a provider config file.
func Load(r io.Reader) (Config, error) {
	var cfg Config

	decoder := yaml.NewDecoder(r)
	decoder.KnownFields(true)

	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}

	cfg.applyDefaults()

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (cfg *Config) applyDefaults() {
	if cfg.ProviderID == "" {
		cfg.ProviderID = ProviderID
	}

	if cfg.OpenNebula.ImageNamePattern == "" {
		cfg.OpenNebula.ImageNamePattern = "talos-opennebula-{{ .Arch }}-{{ .TalosVersion }}-schematic-{{ .SchematicID }}"
	}

	if cfg.Defaults.Firmware == "" {
		cfg.Defaults.Firmware = "uefi"
	}

	if cfg.Defaults.NetworkContextMode == "" {
		cfg.Defaults.NetworkContextMode = "auto"
	}

	if cfg.Defaults.HostnameStrategy == "" {
		cfg.Defaults.HostnameStrategy = "vm-name"
	}

	if cfg.Timeouts.Instantiate == 0 {
		cfg.Timeouts.Instantiate = 2 * time.Minute
	}

	if cfg.Timeouts.VMReady == 0 {
		cfg.Timeouts.VMReady = 10 * time.Minute
	}

	if cfg.Timeouts.Terminate == 0 {
		cfg.Timeouts.Terminate = 5 * time.Minute
	}
}

// Validate checks the runtime config for required fields and supported values.
func (cfg Config) Validate() error {
	if cfg.ProviderID != ProviderID {
		return fmt.Errorf("providerID must be %q", ProviderID)
	}

	if cfg.OpenNebula.Endpoint == "" {
		return errors.New("opennebula.endpoint is required")
	}

	if cfg.OpenNebula.TemplateName == "" {
		return errors.New("opennebula.templateName is required")
	}

	if cfg.Defaults.Firmware != "uefi" && cfg.Defaults.Firmware != "bios" {
		return fmt.Errorf("defaults.firmware must be %q or %q", "uefi", "bios")
	}

	if cfg.Defaults.NetworkContextMode != "auto" && cfg.Defaults.NetworkContextMode != "manual" {
		return fmt.Errorf("defaults.networkContextMode must be %q or %q", "auto", "manual")
	}

	if cfg.Defaults.HostnameStrategy != "vm-name" {
		return fmt.Errorf("defaults.hostnameStrategy must be %q", "vm-name")
	}

	if cfg.Defaults.SecureBoot && cfg.Defaults.Firmware != "uefi" {
		return errors.New("secure boot requires UEFI firmware")
	}

	if len(cfg.Flavors) == 0 {
		return errors.New("at least one flavor must be configured")
	}

	if cfg.Defaults.Flavor != "" {
		if _, ok := cfg.Flavors[cfg.Defaults.Flavor]; !ok {
			return fmt.Errorf("defaults.flavor %q is not defined", cfg.Defaults.Flavor)
		}
	}

	for name, flavor := range cfg.Flavors {
		if flavor.CPU == "" {
			return fmt.Errorf("flavor %q cpu is required", name)
		}

		if flavor.VCPU <= 0 {
			return fmt.Errorf("flavor %q vcpu must be > 0", name)
		}

		if flavor.MemoryMiB <= 0 {
			return fmt.Errorf("flavor %q memoryMiB must be > 0", name)
		}

		if flavor.RootDiskGiB <= 0 {
			return fmt.Errorf("flavor %q rootDiskGiB must be > 0", name)
		}
	}

	if cfg.Timeouts.Instantiate <= 0 || cfg.Timeouts.VMReady <= 0 || cfg.Timeouts.Terminate <= 0 {
		return errors.New("timeouts must be greater than zero")
	}

	return nil
}

// AllowedNetwork reports whether a network is allowed by runtime policy.
func (cfg Config) AllowedNetwork(name string) bool {
	if len(cfg.OpenNebula.AllowedNetworks) == 0 {
		return true
	}

	for _, value := range cfg.OpenNebula.AllowedNetworks {
		if value == name {
			return true
		}
	}

	return false
}

// AllowedDatastore reports whether a datastore is allowed by runtime policy.
func (cfg Config) AllowedDatastore(name string) bool {
	if len(cfg.OpenNebula.AllowedDatastores) == 0 {
		return true
	}

	for _, value := range cfg.OpenNebula.AllowedDatastores {
		if value == name {
			return true
		}
	}

	return false
}
