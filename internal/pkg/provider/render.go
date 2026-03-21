// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package provider

import (
	"fmt"
	"strings"
)

// RenderInput is the fully resolved template-render input.
type RenderInput struct {
	VMName          string
	MachineUUID     string
	Hypervisor      string
	ImageName       string
	Datastore       string
	Resources       ResolvedResources
	Networks        []RenderedNetwork
	FirmwareMode    string
	SecureBoot      bool
	GraphicsEnabled bool
	ContextKV       map[string]string
	Placement       ResolvedPlacement
	AdditionalDisks []AdditionalDisk
}

// RenderedNetwork is the fully resolved network attachment input.
type RenderedNetwork struct {
	Name  string
	Model string
}

// RenderTemplate renders the extra template string for OpenNebula instantiation.
func RenderTemplate(input RenderInput) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("NAME = %q\n\n", input.VMName))
	if input.Hypervisor != "" {
		builder.WriteString(fmt.Sprintf("HYPERVISOR = %q\n", input.Hypervisor))
	}
	builder.WriteString(fmt.Sprintf("CPU = %q\n", input.Resources.CPU))
	builder.WriteString(fmt.Sprintf("VCPU = %q\n", fmt.Sprintf("%d", input.Resources.VCPU)))
	builder.WriteString(fmt.Sprintf("MEMORY = %q\n\n", fmt.Sprintf("%d", input.Resources.MemoryMiB)))

	if input.Placement.SchedRequirements != "" {
		builder.WriteString(fmt.Sprintf("SCHED_REQUIREMENTS = %q\n\n", input.Placement.SchedRequirements))
	}

	if input.Placement.VMGroupName != "" {
		builder.WriteString("VMGROUP = [\n")
		builder.WriteString(fmt.Sprintf("  VMGROUP_NAME = %q", input.Placement.VMGroupName))
		if input.Placement.VMGroupRole != "" {
			builder.WriteString(fmt.Sprintf(",\n  ROLE = %q\n", input.Placement.VMGroupRole))
		} else {
			builder.WriteString("\n")
		}
		builder.WriteString("]\n\n")
	}

	builder.WriteString("OS = [\n")
	builder.WriteString(fmt.Sprintf("  FIRMWARE = %q,\n", strings.ToUpper(input.FirmwareMode)))
	if input.SecureBoot {
		builder.WriteString("  FIRMWARE_SECURE = \"YES\"")
	} else {
		builder.WriteString("  FIRMWARE_SECURE = \"NO\"")
	}
	if input.MachineUUID != "" {
		builder.WriteString(fmt.Sprintf(",\n  UUID = %q", input.MachineUUID))
	}
	builder.WriteString("\n")
	builder.WriteString("]\n\n")

	builder.WriteString("DISK = [\n")
	builder.WriteString(fmt.Sprintf("  IMAGE = %q,\n", input.ImageName))
	if input.Datastore != "" {
		builder.WriteString(fmt.Sprintf("  DATASTORE = %q,\n", input.Datastore))
	}
	builder.WriteString(fmt.Sprintf("  SIZE = %q\n", fmt.Sprintf("%d", input.Resources.RootDiskGiB*1024)))
	builder.WriteString("]\n\n")

	for _, disk := range input.AdditionalDisks {
		builder.WriteString("DISK = [\n")
		builder.WriteString("  TYPE = \"fs\",\n")
		builder.WriteString(fmt.Sprintf("  SIZE = %q,\n", fmt.Sprintf("%d", disk.SizeGiB*1024)))
		builder.WriteString(fmt.Sprintf("  FORMAT = %q\n", disk.Format))
		builder.WriteString("]\n\n")
	}

	for _, network := range input.Networks {
		model := network.Model
		if model == "" {
			model = "virtio"
		}

		builder.WriteString("NIC = [\n")
		builder.WriteString(fmt.Sprintf("  NETWORK = %q,\n", network.Name))
		builder.WriteString(fmt.Sprintf("  MODEL = %q\n", model))
		builder.WriteString("]\n\n")
	}

	builder.WriteString("GRAPHICS = [\n")
	if input.GraphicsEnabled {
		builder.WriteString("  TYPE = \"vnc\"\n")
	} else {
		builder.WriteString("  TYPE = \"none\"\n")
	}
	builder.WriteString("]\n\n")

	builder.WriteString("CONTEXT = [\n")
	keys := make([]string, 0, len(input.ContextKV))
	for key := range input.ContextKV {
		keys = append(keys, key)
	}
	sortStrings(keys)
	for idx, key := range keys {
		lineEnd := ","
		if idx == len(keys)-1 {
			lineEnd = ""
		}
		builder.WriteString(fmt.Sprintf("  %s = %q%s\n", key, input.ContextKV[key], lineEnd))
	}
	builder.WriteString("]\n")

	return builder.String()
}

func sortStrings(values []string) {
	for i := range values {
		for j := i + 1; j < len(values); j++ {
			if values[j] < values[i] {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}
