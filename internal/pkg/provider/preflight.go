// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package provider

import (
	"context"
	"fmt"

	"github.com/siderolabs/omni/client/pkg/infra/provision"

	providerconfig "github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/config"
	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/opennebula"
	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/provider/resources"
)

func (p *Provisioner) resolveProvisionPlan(ctx context.Context, pctx provision.Context[*resources.Machine]) (ResolvedPlan, error) {
	data, resources, err := p.resolveRequest(ctx, pctx)
	if err != nil {
		return ResolvedPlan{}, err
	}

	templateRef, err := p.client.LookupTemplateByName(ctx, data.TemplateName)
	if err != nil {
		return ResolvedPlan{}, err
	}

	networks, err := p.client.LookupNetworksByName(ctx, networkNames(data.Networks))
	if err != nil {
		return ResolvedPlan{}, err
	}

	datastoreName := data.Datastore
	if datastoreName == "" {
		datastoreName = p.config.StoragePolicies.DefaultDatastore
	}

	datastoreRef := opennebula.DatastoreRef{}
	if datastoreName != "" {
		datastoreRef, err = p.client.LookupDatastoreByName(ctx, datastoreName)
		if err != nil {
			return ResolvedPlan{}, err
		}
		data.Datastore = datastoreName
	}

	hypervisor, err := p.resolveHypervisor(ctx)
	if err != nil {
		return ResolvedPlan{}, err
	}

	placement, err := p.selectPlacement(ctx, data, hypervisor)
	if err != nil {
		return ResolvedPlan{}, err
	}

	preflight := p.runPreflight(data, resources, hypervisor, placement)
	if preflight.Status == PreflightStatusFail || (p.config.Policy.Preflight.FailOnWarnings && len(preflight.Warnings) > 0) {
		if preflight.Status != PreflightStatusFail {
			preflight.Status = PreflightStatusFail
		}

		return ResolvedPlan{
			Data:             data,
			Resources:        resources,
			Template:         templateRef,
			Networks:         networks,
			Datastore:        datastoreRef,
			Hypervisor:       hypervisor,
			Placement:        placement,
			BootstrapProfile: p.resolveBootstrapProfile(hypervisor),
			Preflight:        preflight,
		}, fmt.Errorf("%w: preflight failed: %s", opennebula.ErrPolicy, firstPreflightProblem(preflight))
	}

	return ResolvedPlan{
		Data:             data,
		Resources:        resources,
		Template:         templateRef,
		Networks:         networks,
		Datastore:        datastoreRef,
		Hypervisor:       hypervisor,
		Placement:        placement,
		BootstrapProfile: p.resolveBootstrapProfile(hypervisor),
		Preflight:        preflight,
	}, nil
}

func (p *Provisioner) runPreflight(data ProviderData, resources ResolvedResources, hypervisor string, placement PlacementDecision) PreflightResult {
	result := PreflightResult{Status: PreflightStatusPass}

	if resources.VCPU <= 0 || resources.MemoryMiB <= 0 || resources.RootDiskGiB <= 0 {
		result.Errors = append(result.Errors, "resolved resources are incomplete")
	}
	if hypervisor == providerconfig.HypervisorQEMU && resources.CPU != "" && resources.CPU != "host-passthrough" {
		result.Warnings = append(result.Warnings, "qemu environment resolved with non-host-passthrough cpu setting")
	}
	if placement.Selected.ID == 0 {
		result.Errors = append(result.Errors, "no eligible host was selected")
	}
	if data.NetworkContextMode == "manual" {
		switch p.config.Policy.ManualNetworking.Mode {
		case providerconfig.ManualNetworkingWarn:
			result.Warnings = append(result.Warnings, "manual networking path is operator-accepted but not fully provider-validated")
		case providerconfig.ManualNetworkingRequireValidation:
			if !manualNetworksValidated(data, p.config) {
				result.Errors = append(result.Errors, "manual networking requires validated network profiles")
			}
		case providerconfig.ManualNetworkingDeny:
			result.Errors = append(result.Errors, "manual networking is disabled by runtime policy")
		}
	}

	if len(result.Errors) > 0 {
		result.Status = PreflightStatusFail
	} else if len(result.Warnings) > 0 {
		result.Status = PreflightStatusWarn
	}

	return result
}

func manualNetworksValidated(data ProviderData, cfg providerconfig.Config) bool {
	for _, network := range data.Networks {
		if network.Profile == "" {
			return false
		}

		profile, ok := cfg.ResolveNetworkProfile(network.Profile)
		if !ok || !profile.ManualValidated {
			return false
		}
	}

	return true
}

func (p *Provisioner) resolveBootstrapProfile(hypervisor string) string {
	switch p.config.Bootstrap.Profile {
	case providerconfig.BootstrapProfileLab, providerconfig.BootstrapProfileProduction, providerconfig.BootstrapProfileCustom:
		return p.config.Bootstrap.Profile
	}

	if hypervisor == providerconfig.HypervisorQEMU || p.config.Environment.Profile == providerconfig.EnvironmentProfileLabQEMU {
		return providerconfig.BootstrapProfileLab
	}

	return providerconfig.BootstrapProfileProduction
}

func firstPreflightProblem(result PreflightResult) string {
	if len(result.Errors) > 0 {
		return result.Errors[0]
	}
	if len(result.Warnings) > 0 {
		return result.Warnings[0]
	}

	return "unknown preflight failure"
}
