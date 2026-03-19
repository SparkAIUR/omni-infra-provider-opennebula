// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package provider

// ProviderData is the public machine-class contract for the OpenNebula provider.
//
// Example:
//
//	schemaVersion: v1alpha1
//	flavor: medium
//	templateName: talos-omni-base
//	datastore: default
//	networks:
//	  - name: prod-lan
//	networkContextMode: auto
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
}

// ResourceOverrides allows operator-approved explicit resource sizing.
type ResourceOverrides struct {
	CPU         string `json:"cpu,omitempty" yaml:"cpu,omitempty"`
	VCPU        int    `json:"vcpu,omitempty" yaml:"vcpu,omitempty"`
	MemoryMiB   int    `json:"memoryMiB,omitempty" yaml:"memoryMiB,omitempty"`
	RootDiskGiB int    `json:"rootDiskGiB,omitempty" yaml:"rootDiskGiB,omitempty"`
}

// NetworkRef identifies an OpenNebula network by name.
type NetworkRef struct {
	Name string `json:"name" yaml:"name"`
}

// StaticNIC describes manual OpenNebula contextual network values.
type StaticNIC struct {
	Name    string   `json:"name" yaml:"name"`
	MAC     string   `json:"mac,omitempty" yaml:"mac,omitempty"`
	IP      string   `json:"ip,omitempty" yaml:"ip,omitempty"`
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

// ResolvedResources is the effective VM sizing after defaults and validation.
type ResolvedResources struct {
	CPU         string
	VCPU        int
	MemoryMiB   int
	RootDiskGiB int
}

func networkNames(networks []NetworkRef) []string {
	names := make([]string, 0, len(networks))
	for _, network := range networks {
		names = append(names, network.Name)
	}

	return names
}
