// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package provider

import "github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/opennebula"

// PreflightStatus describes the summarized compatibility outcome.
type PreflightStatus string

const (
	PreflightStatusPass   PreflightStatus = "pass"
	PreflightStatusWarn   PreflightStatus = "warn"
	PreflightStatusFail   PreflightStatus = "fail"
	DriftStatusHealthy    string          = "healthy"
	DriftStatusWarning    string          = "warning"
	DriftStatusActionable string          = "actionable"
	ImageActionReused     string          = "reused"
	ImageActionImported   string          = "imported"
	ImageActionWaited     string          = "waited_for_existing_import"
	ImageActionEvicted    string          = "evicted_previous_generation"
	ImageActionUnknown    string          = "unknown"
)

// PreflightResult is the structured outcome of compatibility validation.
type PreflightResult struct {
	Status   PreflightStatus
	Errors   []string
	Warnings []string
}

// PlacementDecision captures the resolved host and the explanation summary.
type PlacementDecision struct {
	Selected      opennebula.HostInfo
	ScoreSummary  string
	Reason        string
	TopCandidates []ScoredHost
}

// ScoredHost pairs a host with its computed score.
type ScoredHost struct {
	Host   opennebula.HostInfo
	Score  float64
	Reason string
}

// ResolvedPlan is the authoritative resolution bundle used by preflight and provision.
type ResolvedPlan struct {
	Data             ProviderData
	Resources        ResolvedResources
	Template         opennebula.TemplateRef
	Networks         []opennebula.NetworkRef
	Datastore        opennebula.DatastoreRef
	Hypervisor       string
	Placement        PlacementDecision
	BootstrapProfile string
	Preflight        PreflightResult
}
