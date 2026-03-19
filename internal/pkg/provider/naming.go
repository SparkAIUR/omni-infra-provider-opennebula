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

// MachineRequestRole returns the role token used by cluster-role sequence naming.
func MachineRequestRole(machineRequest *infrares.MachineRequest) (string, error) {
	if _, ok := machineRequest.Metadata().Labels().Get(omnires.LabelControlPlaneRole); ok {
		return nodeRoleControlPlane, nil
	}

	if _, ok := machineRequest.Metadata().Labels().Get(omnires.LabelWorkerRole); ok {
		return nodeRoleWorker, nil
	}

	return "", fmt.Errorf("machine request %q is missing control-plane/worker role labels", machineRequest.Metadata().ID())
}

// SequenceVMName formats the deterministic VM name for cluster-role sequence naming.
func SequenceVMName(clusterPrefix, role string, ordinal int) string {
	return fmt.Sprintf("%s%s%02d", clusterPrefix, role, ordinal)
}
