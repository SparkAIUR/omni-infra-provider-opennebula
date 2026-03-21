package provider

import (
	"context"
	"fmt"
	"time"

	"go.yaml.in/yaml/v4"

	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/imagemanager"
	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/opennebula"
)

// ExplainResult is the non-mutating resolution output for operators.
type ExplainResult struct {
	GeneratedAt      time.Time               `json:"generatedAt" yaml:"generatedAt"`
	Data             ProviderData            `json:"providerData" yaml:"providerData"`
	Resources        ResolvedResources       `json:"resources" yaml:"resources"`
	Template         opennebula.TemplateRef  `json:"template" yaml:"template"`
	Networks         []opennebula.NetworkRef `json:"networks" yaml:"networks"`
	Datastore        opennebula.DatastoreRef `json:"datastore" yaml:"datastore"`
	Hypervisor       string                  `json:"hypervisor" yaml:"hypervisor"`
	StorageProfile   string                  `json:"storageProfile" yaml:"storageProfile"`
	Placement        PlacementDecision       `json:"placement" yaml:"placement"`
	BootstrapProfile string                  `json:"bootstrapProfile" yaml:"bootstrapProfile"`
	Preflight        PreflightResult         `json:"preflight" yaml:"preflight"`
	Image            imagemanager.Prediction `json:"image" yaml:"image"`
}

// ExplainInput provides the non-Omni values needed for a dry-run.
type ExplainInput struct {
	ProviderData ProviderData
	TalosVersion string
	SchematicID  string
	Architecture string
}

// SupportBundle is a portable debug snapshot built from explain and live inventory.
type SupportBundle struct {
	GeneratedAt   time.Time               `json:"generatedAt" yaml:"generatedAt"`
	RuntimeConfig any                     `json:"runtimeConfig" yaml:"runtimeConfig"`
	Explain       ExplainResult           `json:"explain" yaml:"explain"`
	Hosts         []opennebula.HostInfo   `json:"hosts" yaml:"hosts"`
	Datastore     opennebula.DatastoreRef `json:"datastore" yaml:"datastore"`
}

// ParseProviderData decodes providerData from YAML or JSON.
func ParseProviderData(raw []byte) (ProviderData, error) {
	var data ProviderData
	if err := yaml.Unmarshal(raw, &data); err != nil {
		return ProviderData{}, fmt.Errorf("decode providerData: %w", err)
	}

	return data, nil
}

// Explain resolves a providerData payload without mutating OpenNebula.
func (p *Provisioner) Explain(ctx context.Context, input ExplainInput) (ExplainResult, error) {
	data := input.ProviderData
	if err := ValidateProviderData(&data, p.config); err != nil {
		return ExplainResult{}, err
	}
	if input.TalosVersion == "" {
		input.TalosVersion = "v0.0.0"
	}
	if input.SchematicID == "" {
		input.SchematicID = "explain"
	}
	if input.Architecture == "" {
		input.Architecture = "amd64"
	}

	resources, err := ResolveResources(data, p.config)
	if err != nil {
		return ExplainResult{}, fmt.Errorf("resolve resources: %w", err)
	}

	templateRef, err := p.client.LookupTemplateByName(ctx, data.TemplateName)
	if err != nil {
		return ExplainResult{}, err
	}

	networks, err := p.client.LookupNetworksByName(ctx, networkNames(data.Networks))
	if err != nil {
		return ExplainResult{}, err
	}

	datastoreRef, err := p.client.LookupDatastoreByName(ctx, data.Datastore)
	if err != nil {
		return ExplainResult{}, err
	}

	hypervisor, err := p.resolveHypervisor(ctx)
	if err != nil {
		return ExplainResult{}, err
	}

	placement, err := p.selectPlacement(ctx, data, hypervisor)
	if err != nil {
		return ExplainResult{}, err
	}

	preflight := p.runPreflight(data, resources, datastoreRef, hypervisor, placement)
	bootstrapProfile := p.resolveBootstrapProfile(hypervisor)

	imageName, err := p.renderImageName(input.TalosVersion, input.SchematicID, data.Datastore)
	if err != nil {
		return ExplainResult{}, err
	}

	imagePrediction, err := p.imageManager.Predict(ctx, imagemanager.ResolveRequest{
		ImageName:    imageName,
		Arch:         input.Architecture,
		TalosVersion: input.TalosVersion,
		SchematicID:  input.SchematicID,
		Datastore:    data.Datastore,
		AllowImport:  imageImportPreference(data),
	})
	if err != nil && !opennebula.IsNotFoundError(err) {
		return ExplainResult{}, err
	}

	return ExplainResult{
		GeneratedAt:      time.Now().UTC(),
		Data:             data,
		Resources:        resources,
		Template:         templateRef,
		Networks:         networks,
		Datastore:        datastoreRef,
		Hypervisor:       hypervisor,
		StorageProfile:   effectiveStorageProfile(data),
		Placement:        placement,
		BootstrapProfile: bootstrapProfile,
		Preflight:        preflight,
		Image:            imagePrediction,
	}, nil
}

// BuildSupportBundle returns explain output plus inventory useful for debugging.
func (p *Provisioner) BuildSupportBundle(ctx context.Context, input ExplainInput) (SupportBundle, error) {
	explain, err := p.Explain(ctx, input)
	if err != nil {
		return SupportBundle{}, err
	}

	hosts, err := p.client.ListHosts(ctx, opennebula.HostListRequest{ResourcePool: p.config.OpenNebula.ResourcePool})
	if err != nil {
		return SupportBundle{}, err
	}

	return SupportBundle{
		GeneratedAt:   time.Now().UTC(),
		RuntimeConfig: p.config,
		Explain:       explain,
		Hosts:         hosts,
		Datastore:     explain.Datastore,
	}, nil
}
