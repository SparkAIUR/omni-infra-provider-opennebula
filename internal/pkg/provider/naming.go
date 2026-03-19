// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	infrares "github.com/siderolabs/omni/client/pkg/omni/resources/infra"
	omnires "github.com/siderolabs/omni/client/pkg/omni/resources/omni"
)

const (
	nodeRoleControlPlane = "cp"
	nodeRoleWorker       = "w"

	controlPlaneMachineSetSuffix = "-control-planes"
	workerMachineSetMarker       = "-workers-"
)

// CanonicalVMName normalizes the Omni request ID into an OpenNebula/Talos-safe name.
func CanonicalVMName(requestID string) string {
	lower := strings.ToLower(requestID)
	var builder strings.Builder
	lastDash := false

	for _, r := range lower {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}

	name := strings.Trim(builder.String(), "-")
	if name == "" {
		name = "opennebula-node"
	}

	if len(name) <= 63 {
		return name
	}

	sum := sha256.Sum256([]byte(requestID))
	suffix := hex.EncodeToString(sum[:4])

	return strings.TrimRight(name[:54], "-") + "-" + suffix
}

// NormalizeClusterPrefix normalizes an Omni cluster name into the deterministic naming prefix.
func NormalizeClusterPrefix(clusterName string) (string, error) {
	var builder strings.Builder

	for _, r := range strings.ToLower(clusterName) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
		}
	}

	prefix := builder.String()
	switch {
	case prefix == "":
		return "", fmt.Errorf("cluster name %q resolves to an empty prefix", clusterName)
	case len(prefix) > 57:
		return "", fmt.Errorf("cluster prefix %q exceeds 57 characters", prefix)
	default:
		return prefix, nil
	}
}

func roleFromLabels(labels interface {
	Get(string) (string, bool)
}, resourceID string) (string, error) {
	if _, ok := labels.Get(omnires.LabelControlPlaneRole); ok {
		return nodeRoleControlPlane, nil
	}

	if _, ok := labels.Get(omnires.LabelWorkerRole); ok {
		return nodeRoleWorker, nil
	}

	return "", fmt.Errorf("resource %q is missing control-plane/worker role labels", resourceID)
}

// MachineRequestRole returns the role token used by cluster-role sequence naming.
func MachineRequestRole(machineRequest *infrares.MachineRequest) (string, error) {
	return roleFromLabels(machineRequest.Metadata().Labels(), machineRequest.Metadata().ID())
}

// ClusterRoleFromMachineRequestSet infers the cluster name and role from the Omni machine-request-set naming convention.
func ClusterRoleFromMachineRequestSet(machineRequestSetID string) (string, string, error) {
	switch {
	case strings.HasSuffix(machineRequestSetID, controlPlaneMachineSetSuffix):
		clusterName := strings.TrimSuffix(machineRequestSetID, controlPlaneMachineSetSuffix)
		if clusterName == "" {
			return "", "", fmt.Errorf("machine request set %q resolves to an empty cluster name", machineRequestSetID)
		}

		return clusterName, nodeRoleControlPlane, nil
	case strings.Contains(machineRequestSetID, workerMachineSetMarker):
		clusterName, _, _ := strings.Cut(machineRequestSetID, workerMachineSetMarker)
		if clusterName == "" {
			return "", "", fmt.Errorf("machine request set %q resolves to an empty cluster name", machineRequestSetID)
		}

		return clusterName, nodeRoleWorker, nil
	default:
		return "", "", fmt.Errorf("machine request set %q does not match supported control-plane/worker naming", machineRequestSetID)
	}
}

// SequenceVMName formats the deterministic VM name for cluster-role sequence naming.
func SequenceVMName(clusterPrefix, role string, ordinal int) string {
	return fmt.Sprintf("%s%s%02d", clusterPrefix, role, ordinal)
}
