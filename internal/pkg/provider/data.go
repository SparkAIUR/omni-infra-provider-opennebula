// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package provider

// ProviderData is the public machine-class contract for the OpenNebula provider.
//
// Example:
//
//	schemaVersion: v1alpha2
//	flavor: medium
//	templateName: talos-omni-base
//	datastore: default
//	networks:
//	  - profile: prod
//	imagePolicy:
//	  mode: reuse-or-import
//	placement:
//	  host: compute-01
type ProviderData struct {
	SchemaVersion      string             `json:"schemaVersion,omitempty" yaml:"schemaVersion,omitempty"`
	Flavor             string             `json:"flavor,omitempty" yaml:"flavor,omitempty"`
	Resources          *ResourceOverrides `json:"resources,omitempty" yaml:"resources,omitempty"`
	TemplateName       string             `json:"templateName,omitempty" yaml:"templateName,omitempty"`
	Datastore          string             `json:"datastore,omitempty" yaml:"datastore,omitempty"`
	Networks           []NetworkRef       `json:"networks,omitempty" yaml:"networks,omitempty"`
	NetworkContextMode string             `json:"networkContextMode,omitempty" yaml:"networkContextMode,omitempty"`
	StaticNetwork      []StaticNIC        `json:"staticNetwork,omitempty" yaml:"staticNetwork,omitempty"`
	RootDiskGiB        int                `json:"rootDiskGiB,omitempty" yaml:"rootDiskGiB,omitempty"`
	Firmware           FirmwareConfig     `json:"firmware,omitempty" yaml:"firmware,omitempty"`
	Graphics           GraphicsConfig     `json:"graphics,omitempty" yaml:"graphics,omitempty"`
	Tags               map[string]string  `json:"tags,omitempty" yaml:"tags,omitempty"`
	GPU                *GPURequest        `json:"gpu,omitempty" yaml:"gpu,omitempty"`
	ImagePolicy        ImagePolicy        `json:"imagePolicy,omitempty" yaml:"imagePolicy,omitempty"`
	Placement          PlacementPolicy    `json:"placement,omitempty" yaml:"placement,omitempty"`
	AdditionalDisks    []AdditionalDisk   `json:"additionalDisks,omitempty" yaml:"additionalDisks,omitempty"`
	Lifecycle          LifecyclePolicy    `json:"lifecycle,omitempty" yaml:"lifecycle,omitempty"`
}

// ResourceOverrides allows operator-approved explicit resource sizing.
type ResourceOverrides struct {
	CPU         string `json:"cpu,omitempty" yaml:"cpu,omitempty"`
	VCPU        int    `json:"vcpu,omitempty" yaml:"vcpu,omitempty"`
	MemoryMiB   int    `json:"memoryMiB,omitempty" yaml:"memoryMiB,omitempty"`
	RootDiskGiB int    `json:"rootDiskGiB,omitempty" yaml:"rootDiskGiB,omitempty"`
}

// NetworkRef identifies an OpenNebula network by name or profile.
type NetworkRef struct {
	Name    string `json:"name,omitempty" yaml:"name,omitempty"`
	Profile string `json:"profile,omitempty" yaml:"profile,omitempty"`
	Mode    string `json:"mode,omitempty" yaml:"mode,omitempty"`
	Model   string `json:"model,omitempty" yaml:"model,omitempty"`
}

// StaticNIC describes manual OpenNebula contextual network values.
type StaticNIC struct {
	Name    string   `json:"name" yaml:"name"`
	MAC     string   `json:"mac,omitempty" yaml:"mac,omitempty"`
	IP      string   `json:"ip,omitempty" yaml:"ip,omitempty"`
	Mask    string   `json:"mask,omitempty" yaml:"mask,omitempty"`
	Network string   `json:"network,omitempty" yaml:"network,omitempty"`
	Gateway string   `json:"gateway,omitempty" yaml:"gateway,omitempty"`
	DNS     []string `json:"dns,omitempty" yaml:"dns,omitempty"`
}

// FirmwareConfig controls guest firmware defaults.
type FirmwareConfig struct {
	Mode       string `json:"mode,omitempty" yaml:"mode,omitempty"`
	SecureBoot *bool  `json:"secureBoot,omitempty" yaml:"secureBoot,omitempty"`
}

// GraphicsConfig exposes console graphics behavior.
type GraphicsConfig struct {
	Enabled *bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`
}

// GPURequest is reserved for a future, feature-gated GPU implementation.
type GPURequest struct {
	Enabled bool   `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Mode    string `json:"mode,omitempty" yaml:"mode,omitempty"`
	Vendor  string `json:"vendor,omitempty" yaml:"vendor,omitempty"`
	Device  string `json:"device,omitempty" yaml:"device,omitempty"`
	Count   int    `json:"count,omitempty" yaml:"count,omitempty"`
	Profile string `json:"profile,omitempty" yaml:"profile,omitempty"`
}

// ImagePolicy controls whether the provider may import missing Talos images.
type ImagePolicy struct {
	Mode string `json:"mode,omitempty" yaml:"mode,omitempty"`
}

// PlacementPolicy controls safe placement overrides exposed to self-service users.
type PlacementPolicy struct {
	Host    string `json:"host,omitempty" yaml:"host,omitempty"`
	Cluster string `json:"cluster,omitempty" yaml:"cluster,omitempty"`
	VMGroup string `json:"vmGroup,omitempty" yaml:"vmGroup,omitempty"`
	Role    string `json:"role,omitempty" yaml:"role,omitempty"`
}

// AdditionalDisk defines a volatile extra disk attached to the Talos VM.
type AdditionalDisk struct {
	Name    string `json:"name,omitempty" yaml:"name,omitempty"`
	SizeGiB int    `json:"sizeGiB" yaml:"sizeGiB"`
	Format  string `json:"format,omitempty" yaml:"format,omitempty"`
}

// LifecyclePolicy controls per-machine deletion behavior.
type LifecyclePolicy struct {
	DeleteMode string `json:"deleteMode,omitempty" yaml:"deleteMode,omitempty"`
}

// ResolvedResources is the effective VM sizing after defaults and validation.
type ResolvedResources struct {
	CPU         string
	VCPU        int
	MemoryMiB   int
	RootDiskGiB int
}

// ResolvedPlacement is the rendered placement policy used in the OpenNebula template.
type ResolvedPlacement struct {
	SchedRequirements string
	VMGroupName       string
	VMGroupRole       string
}

func networkNames(networks []NetworkRef) []string {
	names := make([]string, 0, len(networks))
	for _, network := range networks {
		names = append(names, network.Name)
	}

	return names
}
