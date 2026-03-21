// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package provider

import (
	"time"

	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/provider/resources"
)

const (
	annotationMachineUUID = "opennebula.omni.sidero.dev/machine-uuid"
	annotationDeleteMode  = "opennebula.omni.sidero.dev/delete-mode"
)

func setStringAnnotation(machine *resources.Machine, key, value string) {
	if value == "" {
		machine.Metadata().Annotations().Delete(key)
		return
	}

	machine.Metadata().Annotations().Set(key, value)
}

// SetMachineUUID stores the generated machine UUID on the resource annotations.
func SetMachineUUID(machine *resources.Machine, value string) {
	setStringAnnotation(machine, annotationMachineUUID, value)
}

// GetMachineUUID reads the generated machine UUID from the resource annotations.
func GetMachineUUID(machine *resources.Machine) string {
	value, ok := machine.Metadata().Annotations().Get(annotationMachineUUID)
	if !ok {
		return ""
	}

	return value
}

// SetDeleteMode stores the effective lifecycle.deleteMode on the resource annotations.
func SetDeleteMode(machine *resources.Machine, value string) {
	setStringAnnotation(machine, annotationDeleteMode, value)
}

// GetDeleteMode reads the effective lifecycle.deleteMode from the resource annotations.
func GetDeleteMode(machine *resources.Machine) string {
	value, ok := machine.Metadata().Annotations().Get(annotationDeleteMode)
	if !ok {
		return ""
	}

	return value
}

// SetVMID persists the OpenNebula VM ID on the provider state resource.
func SetVMID(machine *resources.Machine, vmID int) {
	machine.TypedSpec().Value.VmId = int32(vmID)
}

// GetVMID reads the persisted OpenNebula VM ID from the provider state resource.
func GetVMID(machine *resources.Machine) int {
	return int(machine.TypedSpec().Value.VmId)
}

// SetTemplateName persists the resolved template name.
func SetTemplateName(machine *resources.Machine, value string) {
	machine.TypedSpec().Value.TemplateName = value
}

// SetTemplateID persists the resolved template ID.
func SetTemplateID(machine *resources.Machine, value int) {
	machine.TypedSpec().Value.TemplateId = int32(value)
}

// SetImageID persists the resolved image ID.
func SetImageID(machine *resources.Machine, value int) {
	machine.TypedSpec().Value.ImageId = int32(value)
}

// SetImageName persists the resolved image name.
func SetImageName(machine *resources.Machine, value string) {
	machine.TypedSpec().Value.ImageName = value
}

// SetImageSource persists the image source URL used for import resolution.
func SetImageSource(machine *resources.Machine, value string) {
	machine.TypedSpec().Value.ImageSource = value
}

// SetImageChecksum persists the checksum used to verify an imported image.
func SetImageChecksum(machine *resources.Machine, value string) {
	machine.TypedSpec().Value.ImageChecksum = value
}

// SetDatastore persists the resolved datastore name.
func SetDatastore(machine *resources.Machine, value string) {
	machine.TypedSpec().Value.Datastore = value
}

// SetDatastoreID persists the resolved datastore ID.
func SetDatastoreID(machine *resources.Machine, value int) {
	machine.TypedSpec().Value.DatastoreId = int32(value)
}

// SetFlavor persists the resolved flavor name.
func SetFlavor(machine *resources.Machine, value string) {
	machine.TypedSpec().Value.Flavor = value
}

// SetPhase persists the provider phase and stamps the last successful phase transition time.
func SetPhase(machine *resources.Machine, value string) {
	machine.TypedSpec().Value.Phase = value
	machine.TypedSpec().Value.LastSuccessfulPhaseAt = time.Now().UTC().Format(time.RFC3339)
}

// SetLastError persists the last error message for troubleshooting.
func SetLastError(machine *resources.Machine, value string) {
	machine.TypedSpec().Value.LastError = value
}

// SetLastRetryClassification persists the latest retry/error classification.
func SetLastRetryClassification(machine *resources.Machine, value string) {
	machine.TypedSpec().Value.LastRetryClassification = value
}

// SetNetworkNames persists the resolved network names.
func SetNetworkNames(machine *resources.Machine, names []string) {
	machine.TypedSpec().Value.NetworkNames = append([]string(nil), names...)
}

// SetNetworkIDs persists the resolved network IDs.
func SetNetworkIDs(machine *resources.Machine, ids []int) {
	values := make([]int32, 0, len(ids))
	for _, id := range ids {
		values = append(values, int32(id))
	}

	machine.TypedSpec().Value.NetworkIds = values
}

// SetClusterName persists the source Omni cluster name used for VM naming.
func SetClusterName(machine *resources.Machine, value string) {
	machine.TypedSpec().Value.ClusterName = value
}

// SetClusterPrefix persists the normalized cluster prefix used for VM naming.
func SetClusterPrefix(machine *resources.Machine, value string) {
	machine.TypedSpec().Value.ClusterPrefix = value
}

// SetNodeRole persists the normalized node role token used for VM naming.
func SetNodeRole(machine *resources.Machine, value string) {
	machine.TypedSpec().Value.NodeRole = value
}

// SetSequenceNumber persists the cluster-role sequence ordinal used for VM naming.
func SetSequenceNumber(machine *resources.Machine, value int) {
	machine.TypedSpec().Value.SequenceNumber = int32(value)
}

// SetReservationID persists the provider-owned name reservation resource id.
func SetReservationID(machine *resources.Machine, value string) {
	machine.TypedSpec().Value.ReservationId = value
}

// SetResolvedHypervisor persists the runtime-resolved hypervisor.
func SetResolvedHypervisor(machine *resources.Machine, value string) {
	machine.TypedSpec().Value.ResolvedHypervisor = value
}

// SetResolvedHost persists the chosen host.
func SetResolvedHost(machine *resources.Machine, hostID int, hostName string) {
	machine.TypedSpec().Value.ResolvedHostId = int32(hostID)
	machine.TypedSpec().Value.ResolvedHostName = hostName
}

// SetResolvedHostTags persists the selected host topology tags.
func SetResolvedHostTags(machine *resources.Machine, tags []string) {
	machine.TypedSpec().Value.ResolvedHostTags = append([]string(nil), tags...)
}

// SetResolvedCluster persists the chosen cluster.
func SetResolvedCluster(machine *resources.Machine, clusterID int, clusterName string) {
	machine.TypedSpec().Value.ResolvedClusterId = int32(clusterID)
	machine.TypedSpec().Value.ResolvedClusterName = clusterName
}

// SetResolvedStorageProfile persists the effective storage profile.
func SetResolvedStorageProfile(machine *resources.Machine, value string) {
	machine.TypedSpec().Value.ResolvedStorageProfile = value
}

// SetResolvedDatastoreCapabilities persists normalized datastore capability labels.
func SetResolvedDatastoreCapabilities(machine *resources.Machine, capabilities []string) {
	machine.TypedSpec().Value.ResolvedDatastoreCapabilities = append([]string(nil), capabilities...)
}

// SetPlacementDecision persists placement reasoning.
func SetPlacementDecision(machine *resources.Machine, reason, summary string) {
	machine.TypedSpec().Value.PlacementReason = reason
	machine.TypedSpec().Value.PlacementScoreSummary = summary
}

// SetPreflight persists the preflight summary.
func SetPreflight(machine *resources.Machine, status string, errors, warnings []string) {
	machine.TypedSpec().Value.PreflightStatus = status
	machine.TypedSpec().Value.PreflightErrors = append([]string(nil), errors...)
	machine.TypedSpec().Value.PreflightWarnings = append([]string(nil), warnings...)
}

// SetImageAction persists the image resolution action summary.
func SetImageAction(machine *resources.Machine, action string, cacheHit, checksumVerified bool) {
	machine.TypedSpec().Value.ImageAction = action
	machine.TypedSpec().Value.ImageCacheHit = cacheHit
	machine.TypedSpec().Value.ImageChecksumVerified = checksumVerified
}

// SetBootstrapProfile persists the resolved bootstrap profile.
func SetBootstrapProfile(machine *resources.Machine, value string) {
	machine.TypedSpec().Value.BootstrapProfile = value
}

// SetDrift persists drift status details.
func SetDrift(machine *resources.Machine, status string, details []string) {
	machine.TypedSpec().Value.DriftStatus = status
	machine.TypedSpec().Value.DriftDetails = append([]string(nil), details...)
}

// SetDiagnosticFingerprint persists an operator-facing failure fingerprint.
func SetDiagnosticFingerprint(machine *resources.Machine, value string) {
	machine.TypedSpec().Value.DiagnosticFingerprint = value
}

// GetReservationID reads the provider-owned name reservation resource id.
func GetReservationID(machine *resources.Machine) string {
	return machine.TypedSpec().Value.ReservationId
}
