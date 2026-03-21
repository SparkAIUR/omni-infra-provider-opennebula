// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package provider

import (
	"context"
	"fmt"
	"sort"
	"strings"

	providerconfig "github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/config"
	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/opennebula"
)

func (p *Provisioner) selectPlacement(ctx context.Context, data ProviderData, hypervisor string) (PlacementDecision, error) {
	hosts, err := p.client.ListHosts(ctx, opennebula.HostListRequest{ResourcePool: p.config.OpenNebula.ResourcePool})
	if err != nil {
		return PlacementDecision{}, err
	}

	requiredTags := effectiveRequiredHostTags(data, hosts)
	candidates := make([]ScoredHost, 0, len(hosts))
	for _, host := range hosts {
		if data.Placement.Cluster != "" && !strings.EqualFold(host.ClusterName, data.Placement.Cluster) {
			continue
		}
		if data.Placement.Host != "" && !strings.EqualFold(host.Name, data.Placement.Host) {
			continue
		}
		if !host.Enabled || !host.Schedulable {
			continue
		}
		if hypervisor != "" && !strings.EqualFold(host.Hypervisor, hypervisor) {
			continue
		}
		if data.Placement.NetworkZone != "" && !p.config.HostInNetworkZone(data.Placement.NetworkZone, host.Name) {
			continue
		}
		if !hostMatchesTags(host, requiredTags, data.Placement.ExcludedHostTags) {
			continue
		}

		score, reason := p.scoreHost(host, hypervisor, data.Placement.NetworkZone, requiredTags)
		candidates = append(candidates, ScoredHost{Host: host, Score: score, Reason: reason})
	}

	if len(candidates) == 0 {
		return PlacementDecision{}, fmt.Errorf("%w: no eligible hosts matched the resolved request", opennebula.ErrPolicy)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			if candidates[i].Host.ID == candidates[j].Host.ID {
				return candidates[i].Host.Name < candidates[j].Host.Name
			}

			return candidates[i].Host.ID < candidates[j].Host.ID
		}

		return candidates[i].Score > candidates[j].Score
	})

	selected := candidates[0]
	reason := selected.Reason
	if data.Placement.Host != "" {
		reason = fmt.Sprintf("placement.host override selected %s", selected.Host.Name)
	}

	return PlacementDecision{
		Selected:      selected.Host,
		Reason:        reason,
		ScoreSummary:  formatScoreSummary(candidates),
		TopCandidates: candidates,
	}, nil
}

func (p *Provisioner) scoreHost(host opennebula.HostInfo, hypervisor string, networkZone string, requiredTags []string) (float64, string) {
	scoring := p.config.Placement.Scoring
	score := host.CPUHeadroomRatio()*scoring.CPUHeadroomWeight +
		host.MemoryHeadroomRatio()*scoring.MemoryHeadroomWeight -
		float64(host.RunningVMs)*scoring.VMCountWeight

	reasons := []string{
		fmt.Sprintf("cpu_headroom=%.2f", host.CPUHeadroomRatio()),
		fmt.Sprintf("memory_headroom=%.2f", host.MemoryHeadroomRatio()),
		fmt.Sprintf("running_vms=%d", host.RunningVMs),
	}

	if hypervisor == providerconfig.HypervisorKVM && strings.EqualFold(host.Hypervisor, providerconfig.HypervisorKVM) {
		score += scoring.HypervisorMatchWeight
		reasons = append(reasons, "matched_resolved_hypervisor=kvm")
	}
	if hypervisor == providerconfig.HypervisorQEMU && strings.EqualFold(host.Hypervisor, providerconfig.HypervisorQEMU) {
		score += scoring.HypervisorMatchWeight
		reasons = append(reasons, "matched_resolved_hypervisor=qemu")
	}
	if p.config.Environment.Profile == providerconfig.EnvironmentProfileLabQEMU && strings.EqualFold(host.Hypervisor, providerconfig.HypervisorQEMU) {
		score += scoring.EnvironmentBiasWeight
		reasons = append(reasons, "environment_bias=lab-qemu")
	}
	if p.config.Environment.Profile == providerconfig.EnvironmentProfileProductionKVM && strings.EqualFold(host.Hypervisor, providerconfig.HypervisorKVM) {
		score += scoring.EnvironmentBiasWeight
		reasons = append(reasons, "environment_bias=production-kvm")
	}
	if p.config.Environment.Profile == providerconfig.EnvironmentProfileMixedStaging {
		score += scoring.EnvironmentBiasWeight * 0.5
		reasons = append(reasons, "environment_bias=mixed-staging")
	}
	if networkZone != "" {
		reasons = append(reasons, "network_zone="+networkZone)
	}
	if len(requiredTags) > 0 {
		reasons = append(reasons, "required_tags="+strings.Join(requiredTags, "|"))
	}
	if len(host.Tags) > 0 {
		reasons = append(reasons, "host_tags="+strings.Join(host.Tags, "|"))
	}

	return score, strings.Join(reasons, ", ")
}

func hostMatchesTags(host opennebula.HostInfo, requiredTags, excludedTags []string) bool {
	for _, tag := range requiredTags {
		if !host.HasTag(tag) {
			return false
		}
	}
	for _, tag := range excludedTags {
		if host.HasTag(tag) {
			return false
		}
	}

	return true
}

func effectiveRequiredHostTags(data ProviderData, hosts []opennebula.HostInfo) []string {
	required := append([]string(nil), data.Placement.RequiredHostTags...)
	storageTag := strings.TrimSpace(data.Placement.StorageProfile)
	if storageTag == "" || storageTag == providerconfig.StorageProfileAny {
		return required
	}

	for _, host := range hosts {
		if host.HasTag(storageTag) {
			required = append(required, storageTag)
			break
		}
	}

	return required
}

func formatScoreSummary(candidates []ScoredHost) string {
	limit := len(candidates)
	if limit > 3 {
		limit = 3
	}

	parts := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		parts = append(parts, fmt.Sprintf("%s=%.2f", candidates[i].Host.Name, candidates[i].Score))
	}

	return strings.Join(parts, ", ")
}
