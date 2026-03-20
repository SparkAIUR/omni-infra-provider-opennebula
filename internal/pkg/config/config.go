// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package config implements the runtime configuration for the OpenNebula provider.
package config

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"go.yaml.in/yaml/v4"
)

const (
	// ProviderID is the only supported provider identifier for this repository.
	ProviderID = "opennebula"
	// HypervisorAuto detects the supported hypervisor from OpenNebula hosts.
	HypervisorAuto = "auto"
	// HypervisorKVM forces KVM for instantiated VMs.
	HypervisorKVM = "kvm"
	// HypervisorQEMU forces qemu for instantiated VMs.
	HypervisorQEMU = "qemu"
	// HostnameStrategyVMName keeps the legacy request-id based naming behavior.
	HostnameStrategyVMName = "vm-name"
	// HostnameStrategyClusterRoleSequence derives names from cluster name, role, and sequence.
	HostnameStrategyClusterRoleSequence = "cluster-role-sequence"
	// EnvironmentProfileCustom keeps all behavior explicit.
	EnvironmentProfileCustom = "custom"
	// EnvironmentProfileLabQEMU applies qemu-friendly defaults for emulated labs.
	EnvironmentProfileLabQEMU = "lab-qemu"
	// EnvironmentProfileProductionKVM applies production-oriented defaults.
	EnvironmentProfileProductionKVM = "production-kvm"
	// ManualNetworkingWarn records warnings but allows the request.
	ManualNetworkingWarn = "warn"
	// ManualNetworkingRequireValidation blocks requests until they use a validated path.
	ManualNetworkingRequireValidation = "require-validation"
	// ManualNetworkingDeny blocks all manual networking requests.
	ManualNetworkingDeny = "deny"
	// BootstrapProfileAuto infers the best bootstrap profile from the environment.
	BootstrapProfileAuto = "auto"
	// BootstrapProfileLab applies qemu-friendly waits and diagnostics.
	BootstrapProfileLab = "lab"
	// BootstrapProfileProduction applies production-oriented waits and diagnostics.
	BootstrapProfileProduction = "production"
	// BootstrapProfileCustom leaves bootstrap settings fully explicit.
	BootstrapProfileCustom = "custom"
)

// Config describes the provider runtime configuration.
type Config struct {
	ProviderID        string                    `yaml:"providerID"`
	OpenNebula        OpenNebulaConfig          `yaml:"opennebula"`
	Defaults          DefaultsConfig            `yaml:"defaults"`
	Features          FeaturesConfig            `yaml:"features"`
	Timeouts          TimeoutsConfig            `yaml:"timeouts"`
	Flavors           map[string]Flavor         `yaml:"flavors"`
	ImageManagement   ImageManagementConfig     `yaml:"imageManagement"`
	PlacementPolicies PlacementPoliciesConfig   `yaml:"placementPolicies"`
	Environment       EnvironmentConfig         `yaml:"environment"`
	Placement         PlacementConfig           `yaml:"placement"`
	Policy            PolicyConfig              `yaml:"policy"`
	Bootstrap         BootstrapConfig           `yaml:"bootstrap"`
	NetworkProfiles   map[string]NetworkProfile `yaml:"networkProfiles"`
	StoragePolicies   StoragePoliciesConfig     `yaml:"storagePolicies"`
	Observability     ObservabilityConfig       `yaml:"observability"`
	Explain           ExplainConfig             `yaml:"explain"`
	Limits            LimitsConfig              `yaml:"limits"`
}

// OpenNebulaConfig contains platform-specific defaults and allow-lists.
type OpenNebulaConfig struct {
	Endpoint          string   `yaml:"endpoint"`
	TemplateName      string   `yaml:"templateName"`
	Hypervisor        string   `yaml:"hypervisor,omitempty"`
	ResourcePool      string   `yaml:"resourcePool,omitempty"`
	AllowedTemplates  []string `yaml:"allowedTemplates,omitempty"`
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

// ImageManagementConfig controls provider-managed Talos image lookup/import behavior.
type ImageManagementConfig struct {
	ImportOnMiss        bool             `yaml:"importOnMiss"`
	RequireChecksum     bool             `yaml:"requireChecksum"`
	ArtifactURLTemplate string           `yaml:"artifactURLTemplate,omitempty"`
	ChecksumURLTemplate string           `yaml:"checksumURLTemplate,omitempty"`
	StagingDir          string           `yaml:"stagingDir,omitempty"`
	RetainGenerations   int              `yaml:"retainGenerations,omitempty"`
	PollInterval        time.Duration    `yaml:"pollInterval,omitempty"`
	ImportTimeout       time.Duration    `yaml:"importTimeout,omitempty"`
	CleanupOnDelete     bool             `yaml:"cleanupOnDelete,omitempty"`
	ImportLock          ImportLockConfig `yaml:"importLock,omitempty"`
	Cache               ImageCacheConfig `yaml:"cache,omitempty"`
}

// ImportLockConfig controls process-local image import serialization.
type ImportLockConfig struct {
	Enabled    bool          `yaml:"enabled,omitempty"`
	StaleAfter time.Duration `yaml:"staleAfter,omitempty"`
}

// ImageCacheConfig controls image reuse and staged artifact cleanup.
type ImageCacheConfig struct {
	Prefetch          ImagePrefetchConfig `yaml:"prefetch,omitempty"`
	RetainGenerations int                 `yaml:"retainGenerations,omitempty"`
	GC                ImageGCConfig       `yaml:"gc,omitempty"`
}

// ImagePrefetchConfig declares a hot set of images for operators to keep warm.
type ImagePrefetchConfig struct {
	Enabled bool     `yaml:"enabled,omitempty"`
	Entries []string `yaml:"entries,omitempty"`
}

// ImageGCConfig controls staged cache cleanup.
type ImageGCConfig struct {
	Enabled bool          `yaml:"enabled,omitempty"`
	MaxAge  time.Duration `yaml:"maxAge,omitempty"`
}

// PlacementPoliciesConfig controls allowed placement overrides.
type PlacementPoliciesConfig struct {
	AllowHostOverride    bool     `yaml:"allowHostOverride"`
	AllowClusterOverride bool     `yaml:"allowClusterOverride"`
	AllowVMGroupOverride bool     `yaml:"allowVMGroupOverride"`
	AllowedHosts         []string `yaml:"allowedHosts,omitempty"`
	AllowedClusters      []string `yaml:"allowedClusters,omitempty"`
	AllowedVMGroups      []string `yaml:"allowedVMGroups,omitempty"`
}

// EnvironmentConfig groups opinionated default bundles.
type EnvironmentConfig struct {
	Profile string `yaml:"profile,omitempty"`
}

// PlacementConfig controls provider-side host scoring.
type PlacementConfig struct {
	Strategy          string                 `yaml:"strategy,omitempty"`
	PreferKVMWhenAuto bool                   `yaml:"preferKVMWhenAuto,omitempty"`
	Scoring           PlacementScoringConfig `yaml:"scoring,omitempty"`
}

// PlacementScoringConfig weights host scoring heuristics.
type PlacementScoringConfig struct {
	CPUHeadroomWeight       float64 `yaml:"cpuHeadroomWeight,omitempty"`
	MemoryHeadroomWeight    float64 `yaml:"memoryHeadroomWeight,omitempty"`
	VMCountWeight           float64 `yaml:"vmCountWeight,omitempty"`
	HypervisorMatchWeight   float64 `yaml:"hypervisorMatchWeight,omitempty"`
	DatastoreAffinityWeight float64 `yaml:"datastoreAffinityWeight,omitempty"`
	EnvironmentBiasWeight   float64 `yaml:"environmentBiasWeight,omitempty"`
}

// PolicyConfig controls preflight and request guardrails.
type PolicyConfig struct {
	Preflight        PreflightPolicyConfig        `yaml:"preflight,omitempty"`
	Minimums         ResourceBoundPolicyConfig    `yaml:"minimums,omitempty"`
	Maximums         ResourceBoundPolicyConfig    `yaml:"maximums,omitempty"`
	ManualNetworking ManualNetworkingPolicyConfig `yaml:"manualNetworking,omitempty"`
}

// PreflightPolicyConfig controls compatibility checks before instantiate.
type PreflightPolicyConfig struct {
	Enabled                        bool `yaml:"enabled,omitempty"`
	FailOnWarnings                 bool `yaml:"failOnWarnings,omitempty"`
	RequireManualNetworkValidation bool `yaml:"requireManualNetworkValidation,omitempty"`
}

// ResourceBoundPolicyConfig constrains effective VM sizing.
type ResourceBoundPolicyConfig struct {
	VCPU        int `yaml:"vcpu,omitempty"`
	MemoryMiB   int `yaml:"memoryMiB,omitempty"`
	RootDiskGiB int `yaml:"rootDiskGiB,omitempty"`
}

// ManualNetworkingPolicyConfig controls how manual network requests are treated.
type ManualNetworkingPolicyConfig struct {
	Mode string `yaml:"mode,omitempty"`
}

// BootstrapConfig controls runtime bootstrap timing.
type BootstrapConfig struct {
	Profile  string                  `yaml:"profile,omitempty"`
	Timeouts BootstrapTimeoutsConfig `yaml:"timeouts,omitempty"`
}

// BootstrapTimeoutsConfig configures profile-aware wait windows.
type BootstrapTimeoutsConfig struct {
	InitialContact   time.Duration `yaml:"initialContact,omitempty"`
	Ready            time.Duration `yaml:"ready,omitempty"`
	DiagnosticsGrace time.Duration `yaml:"diagnosticsGrace,omitempty"`
}

// NetworkProfile stores reusable network attachment defaults.
type NetworkProfile struct {
	NetworkName     string `yaml:"networkName"`
	Model           string `yaml:"model,omitempty"`
	ContextMode     string `yaml:"contextMode,omitempty"`
	ManualValidated bool   `yaml:"manualValidated,omitempty"`
}

// StoragePoliciesConfig controls datastore defaults for root and additional disks.
type StoragePoliciesConfig struct {
	DefaultDatastore         string   `yaml:"defaultDatastore,omitempty"`
	AdditionalDiskDatastores []string `yaml:"additionalDiskDatastores,omitempty"`
}

// ObservabilityConfig controls HTTP endpoints for metrics and health checks.
type ObservabilityConfig struct {
	ListenAddress string `yaml:"listenAddress"`
	MetricsPath   string `yaml:"metricsPath"`
	HealthPath    string `yaml:"healthPath"`
	ReadyPath     string `yaml:"readyPath"`
}

// ExplainConfig controls non-mutating operator explain surfaces.
type ExplainConfig struct {
	Enabled bool `yaml:"enabled,omitempty"`
}

// LimitsConfig constrains self-service resource overrides.
type LimitsConfig struct {
	MaxVCPU              int `yaml:"maxVCPU,omitempty"`
	MaxMemoryMiB         int `yaml:"maxMemoryMiB,omitempty"`
	MaxRootDiskGiB       int `yaml:"maxRootDiskGiB,omitempty"`
	MaxAdditionalDisks   int `yaml:"maxAdditionalDisks,omitempty"`
	MaxAdditionalDiskGiB int `yaml:"maxAdditionalDiskGiB,omitempty"`
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

	if cfg.Environment.Profile == "" {
		cfg.Environment.Profile = EnvironmentProfileCustom
	}

	cfg.applyProfileDefaults()

	if cfg.OpenNebula.ImageNamePattern == "" {
		cfg.OpenNebula.ImageNamePattern = "talos-opennebula-{{ .Arch }}-{{ .TalosVersion }}-schematic-{{ .SchematicID }}"
	}

	if cfg.OpenNebula.Hypervisor == "" {
		cfg.OpenNebula.Hypervisor = HypervisorAuto
	}

	if cfg.Defaults.Firmware == "" {
		cfg.Defaults.Firmware = "uefi"
	}

	if cfg.Defaults.NetworkContextMode == "" {
		cfg.Defaults.NetworkContextMode = "auto"
	}

	if cfg.Defaults.HostnameStrategy == "" {
		cfg.Defaults.HostnameStrategy = HostnameStrategyVMName
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

	if cfg.ImageManagement.RetainGenerations == 0 {
		cfg.ImageManagement.RetainGenerations = 2
	}
	if cfg.ImageManagement.PollInterval == 0 {
		cfg.ImageManagement.PollInterval = 5 * time.Second
	}
	if cfg.ImageManagement.ImportTimeout == 0 {
		cfg.ImageManagement.ImportTimeout = 20 * time.Minute
	}
	if cfg.ImageManagement.StagingDir == "" {
		cfg.ImageManagement.StagingDir = "/var/tmp/omni-infra-provider-opennebula/images"
	}
	if cfg.ImageManagement.ImportLock.StaleAfter == 0 {
		cfg.ImageManagement.ImportLock.StaleAfter = 30 * time.Minute
	}
	if cfg.ImageManagement.Cache.RetainGenerations == 0 {
		cfg.ImageManagement.Cache.RetainGenerations = cfg.ImageManagement.RetainGenerations
	}
	if cfg.ImageManagement.Cache.GC.MaxAge == 0 {
		cfg.ImageManagement.Cache.GC.MaxAge = 7 * 24 * time.Hour
	}

	if cfg.Placement.Strategy == "" {
		cfg.Placement.Strategy = "balanced"
	}
	if cfg.Placement.Scoring.CPUHeadroomWeight == 0 {
		cfg.Placement.Scoring.CPUHeadroomWeight = 1.0
	}
	if cfg.Placement.Scoring.MemoryHeadroomWeight == 0 {
		cfg.Placement.Scoring.MemoryHeadroomWeight = 1.0
	}
	if cfg.Placement.Scoring.VMCountWeight == 0 {
		cfg.Placement.Scoring.VMCountWeight = 0.25
	}
	if cfg.Placement.Scoring.HypervisorMatchWeight == 0 {
		cfg.Placement.Scoring.HypervisorMatchWeight = 2.0
	}
	if cfg.Placement.Scoring.DatastoreAffinityWeight == 0 {
		cfg.Placement.Scoring.DatastoreAffinityWeight = 0.5
	}
	if cfg.Placement.Scoring.EnvironmentBiasWeight == 0 {
		cfg.Placement.Scoring.EnvironmentBiasWeight = 1.0
	}
	if !cfg.Placement.PreferKVMWhenAuto {
		cfg.Placement.PreferKVMWhenAuto = true
	}

	if !cfg.Policy.Preflight.Enabled {
		cfg.Policy.Preflight.Enabled = true
	}
	if !cfg.Policy.Preflight.RequireManualNetworkValidation {
		cfg.Policy.Preflight.RequireManualNetworkValidation = true
	}
	if cfg.Policy.ManualNetworking.Mode == "" {
		cfg.Policy.ManualNetworking.Mode = ManualNetworkingRequireValidation
	}

	if cfg.Bootstrap.Profile == "" {
		cfg.Bootstrap.Profile = BootstrapProfileAuto
	}
	if cfg.Bootstrap.Timeouts.InitialContact == 0 {
		cfg.Bootstrap.Timeouts.InitialContact = 10 * time.Minute
	}
	if cfg.Bootstrap.Timeouts.Ready == 0 {
		cfg.Bootstrap.Timeouts.Ready = 20 * time.Minute
	}
	if cfg.Bootstrap.Timeouts.DiagnosticsGrace == 0 {
		cfg.Bootstrap.Timeouts.DiagnosticsGrace = 2 * time.Minute
	}

	if cfg.Observability.ListenAddress == "" {
		cfg.Observability.ListenAddress = ":9977"
	}
	if cfg.Observability.MetricsPath == "" {
		cfg.Observability.MetricsPath = "/metrics"
	}
	if cfg.Observability.HealthPath == "" {
		cfg.Observability.HealthPath = "/healthz"
	}
	if cfg.Observability.ReadyPath == "" {
		cfg.Observability.ReadyPath = "/readyz"
	}

	if !cfg.Explain.Enabled {
		cfg.Explain.Enabled = true
	}

	if cfg.Limits.MaxVCPU == 0 {
		cfg.Limits.MaxVCPU = 64
	}
	if cfg.Limits.MaxMemoryMiB == 0 {
		cfg.Limits.MaxMemoryMiB = 262144
	}
	if cfg.Limits.MaxRootDiskGiB == 0 {
		cfg.Limits.MaxRootDiskGiB = 2048
	}
	if cfg.Limits.MaxAdditionalDisks == 0 {
		cfg.Limits.MaxAdditionalDisks = 8
	}
	if cfg.Limits.MaxAdditionalDiskGiB == 0 {
		cfg.Limits.MaxAdditionalDiskGiB = 2048
	}
}

func (cfg *Config) applyProfileDefaults() {
	switch cfg.Environment.Profile {
	case EnvironmentProfileCustom:
		return
	case EnvironmentProfileLabQEMU:
		if cfg.OpenNebula.Hypervisor == "" {
			cfg.OpenNebula.Hypervisor = HypervisorQEMU
		}
		if cfg.Bootstrap.Profile == "" {
			cfg.Bootstrap.Profile = BootstrapProfileLab
		}
		if cfg.Policy.ManualNetworking.Mode == "" {
			cfg.Policy.ManualNetworking.Mode = ManualNetworkingRequireValidation
		}
	case EnvironmentProfileProductionKVM:
		if cfg.OpenNebula.Hypervisor == "" {
			cfg.OpenNebula.Hypervisor = HypervisorAuto
		}
		if cfg.Bootstrap.Profile == "" {
			cfg.Bootstrap.Profile = BootstrapProfileProduction
		}
		if cfg.Policy.ManualNetworking.Mode == "" {
			cfg.Policy.ManualNetworking.Mode = ManualNetworkingRequireValidation
		}
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

	switch cfg.OpenNebula.Hypervisor {
	case HypervisorAuto, HypervisorKVM, HypervisorQEMU:
	default:
		return fmt.Errorf("opennebula.hypervisor must be %q, %q, or %q", HypervisorAuto, HypervisorKVM, HypervisorQEMU)
	}

	switch cfg.Environment.Profile {
	case EnvironmentProfileCustom, EnvironmentProfileLabQEMU, EnvironmentProfileProductionKVM:
	default:
		return fmt.Errorf("environment.profile must be %q, %q, or %q", EnvironmentProfileCustom, EnvironmentProfileLabQEMU, EnvironmentProfileProductionKVM)
	}

	if cfg.Defaults.Firmware != "uefi" && cfg.Defaults.Firmware != "bios" {
		return fmt.Errorf("defaults.firmware must be %q or %q", "uefi", "bios")
	}
	if cfg.Defaults.NetworkContextMode != "auto" && cfg.Defaults.NetworkContextMode != "manual" {
		return fmt.Errorf("defaults.networkContextMode must be %q or %q", "auto", "manual")
	}

	switch cfg.Defaults.HostnameStrategy {
	case HostnameStrategyVMName, HostnameStrategyClusterRoleSequence:
	default:
		return fmt.Errorf("defaults.hostnameStrategy must be %q or %q", HostnameStrategyVMName, HostnameStrategyClusterRoleSequence)
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

	if cfg.ImageManagement.RetainGenerations < 1 {
		return errors.New("imageManagement.retainGenerations must be >= 1")
	}
	if cfg.ImageManagement.Cache.RetainGenerations < 1 {
		return errors.New("imageManagement.cache.retainGenerations must be >= 1")
	}
	if cfg.ImageManagement.PollInterval <= 0 || cfg.ImageManagement.ImportTimeout <= 0 {
		return errors.New("imageManagement pollInterval and importTimeout must be greater than zero")
	}
	if strings.TrimSpace(cfg.ImageManagement.StagingDir) == "" {
		return errors.New("imageManagement.stagingDir is required")
	}
	if cfg.ImageManagement.ImportLock.StaleAfter <= 0 {
		return errors.New("imageManagement.importLock.staleAfter must be greater than zero")
	}
	if cfg.ImageManagement.Cache.GC.MaxAge <= 0 {
		return errors.New("imageManagement.cache.gc.maxAge must be greater than zero")
	}

	if cfg.StoragePolicies.DefaultDatastore != "" && !cfg.AllowedDatastore(cfg.StoragePolicies.DefaultDatastore) {
		return fmt.Errorf("storagePolicies.defaultDatastore %q is not allowed", cfg.StoragePolicies.DefaultDatastore)
	}

	for name, profile := range cfg.NetworkProfiles {
		if strings.TrimSpace(name) == "" {
			return errors.New("networkProfiles keys cannot be empty")
		}
		if strings.TrimSpace(profile.NetworkName) == "" {
			return fmt.Errorf("networkProfiles.%s.networkName is required", name)
		}
		if profile.ContextMode != "" && profile.ContextMode != "auto" && profile.ContextMode != "manual" {
			return fmt.Errorf("networkProfiles.%s.contextMode must be %q or %q", name, "auto", "manual")
		}
	}

	switch cfg.Placement.Strategy {
	case "balanced":
	default:
		return fmt.Errorf("placement.strategy must be %q", "balanced")
	}

	switch cfg.Policy.ManualNetworking.Mode {
	case ManualNetworkingWarn, ManualNetworkingRequireValidation, ManualNetworkingDeny:
	default:
		return fmt.Errorf("policy.manualNetworking.mode must be %q, %q, or %q", ManualNetworkingWarn, ManualNetworkingRequireValidation, ManualNetworkingDeny)
	}

	switch cfg.Bootstrap.Profile {
	case BootstrapProfileAuto, BootstrapProfileLab, BootstrapProfileProduction, BootstrapProfileCustom:
	default:
		return fmt.Errorf("bootstrap.profile must be %q, %q, %q, or %q", BootstrapProfileAuto, BootstrapProfileLab, BootstrapProfileProduction, BootstrapProfileCustom)
	}

	if cfg.Bootstrap.Timeouts.InitialContact <= 0 || cfg.Bootstrap.Timeouts.Ready <= 0 || cfg.Bootstrap.Timeouts.DiagnosticsGrace <= 0 {
		return errors.New("bootstrap timeouts must be greater than zero")
	}
	if cfg.Policy.Minimums.VCPU < 0 || cfg.Policy.Minimums.MemoryMiB < 0 || cfg.Policy.Minimums.RootDiskGiB < 0 {
		return errors.New("policy.minimums must be >= 0")
	}
	if cfg.Policy.Maximums.VCPU < 0 || cfg.Policy.Maximums.MemoryMiB < 0 || cfg.Policy.Maximums.RootDiskGiB < 0 {
		return errors.New("policy.maximums must be >= 0")
	}

	if err := validateHTTPPath("observability.metricsPath", cfg.Observability.MetricsPath); err != nil {
		return err
	}
	if err := validateHTTPPath("observability.healthPath", cfg.Observability.HealthPath); err != nil {
		return err
	}
	if err := validateHTTPPath("observability.readyPath", cfg.Observability.ReadyPath); err != nil {
		return err
	}

	if cfg.Limits.MaxVCPU <= 0 || cfg.Limits.MaxMemoryMiB <= 0 || cfg.Limits.MaxRootDiskGiB <= 0 ||
		cfg.Limits.MaxAdditionalDisks <= 0 || cfg.Limits.MaxAdditionalDiskGiB <= 0 {
		return errors.New("limits must be greater than zero")
	}

	return nil
}

func validateHTTPPath(field, value string) error {
	if !strings.HasPrefix(value, "/") {
		return fmt.Errorf("%s must start with /", field)
	}

	return nil
}

// AllowedTemplate reports whether a template is allowed by runtime policy.
func (cfg Config) AllowedTemplate(name string) bool {
	if len(cfg.OpenNebula.AllowedTemplates) == 0 {
		return true
	}

	return listContains(cfg.OpenNebula.AllowedTemplates, name)
}

// AllowedNetwork reports whether a network is allowed by runtime policy.
func (cfg Config) AllowedNetwork(name string) bool {
	if len(cfg.OpenNebula.AllowedNetworks) == 0 {
		return true
	}

	return listContains(cfg.OpenNebula.AllowedNetworks, name)
}

// AllowedDatastore reports whether a datastore is allowed by runtime policy.
func (cfg Config) AllowedDatastore(name string) bool {
	if len(cfg.OpenNebula.AllowedDatastores) == 0 {
		return true
	}

	return listContains(cfg.OpenNebula.AllowedDatastores, name)
}

// AllowedHost reports whether a host placement override is allowed.
func (cfg Config) AllowedHost(name string) bool {
	if len(cfg.PlacementPolicies.AllowedHosts) == 0 {
		return true
	}

	return listContains(cfg.PlacementPolicies.AllowedHosts, name)
}

// AllowedCluster reports whether a cluster placement override is allowed.
func (cfg Config) AllowedCluster(name string) bool {
	if len(cfg.PlacementPolicies.AllowedClusters) == 0 {
		return true
	}

	return listContains(cfg.PlacementPolicies.AllowedClusters, name)
}

// AllowedVMGroup reports whether a VM group placement override is allowed.
func (cfg Config) AllowedVMGroup(name string) bool {
	if len(cfg.PlacementPolicies.AllowedVMGroups) == 0 {
		return true
	}

	return listContains(cfg.PlacementPolicies.AllowedVMGroups, name)
}

// AllowedAdditionalDiskDatastore reports whether a datastore may host provider-managed extra disks.
func (cfg Config) AllowedAdditionalDiskDatastore(name string) bool {
	if len(cfg.StoragePolicies.AdditionalDiskDatastores) == 0 {
		return cfg.AllowedDatastore(name)
	}

	return listContains(cfg.StoragePolicies.AdditionalDiskDatastores, name)
}

// ResolveNetworkProfile returns a named network profile if one exists.
func (cfg Config) ResolveNetworkProfile(name string) (NetworkProfile, bool) {
	profile, ok := cfg.NetworkProfiles[name]
	return profile, ok
}

func listContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}

	return false
}
