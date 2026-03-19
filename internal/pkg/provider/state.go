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
