// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package provider

import (
	"fmt"
	"strings"

	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/opennebula"
)

// RenderInput is the fully resolved template-render input.
type RenderInput struct {
	VMName          string
	ImageName       string
	Datastore       string
	Resources       ResolvedResources
	Networks        []opennebula.NetworkRef
	FirmwareMode    string
	SecureBoot      bool
	GraphicsEnabled bool
	ContextKV       map[string]string
}

// RenderTemplate renders the extra template string for OpenNebula instantiation.
func RenderTemplate(input RenderInput) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("NAME = %q\n\n", input.VMName))
	builder.WriteString(fmt.Sprintf("CPU = %q\n", input.Resources.CPU))
	builder.WriteString(fmt.Sprintf("VCPU = %q\n", fmt.Sprintf("%d", input.Resources.VCPU)))
	builder.WriteString(fmt.Sprintf("MEMORY = %q\n\n", fmt.Sprintf("%d", input.Resources.MemoryMiB)))

	builder.WriteString("OS = [\n")
	builder.WriteString(fmt.Sprintf("  FIRMWARE = %q,\n", strings.ToUpper(input.FirmwareMode)))
	if input.SecureBoot {
		builder.WriteString("  FIRMWARE_SECURE = \"YES\"\n")
	} else {
		builder.WriteString("  FIRMWARE_SECURE = \"NO\"\n")
	}
	builder.WriteString("]\n\n")

	builder.WriteString("DISK = [\n")
	builder.WriteString(fmt.Sprintf("  IMAGE = %q,\n", input.ImageName))
	if input.Datastore != "" {
		builder.WriteString(fmt.Sprintf("  DATASTORE = %q,\n", input.Datastore))
	}
	builder.WriteString(fmt.Sprintf("  SIZE = %q\n", fmt.Sprintf("%d", input.Resources.RootDiskGiB*1024)))
	builder.WriteString("]\n\n")

	for _, network := range input.Networks {
		builder.WriteString("NIC = [\n")
		builder.WriteString(fmt.Sprintf("  NETWORK = %q,\n", network.Name))
		builder.WriteString("  MODEL = \"virtio\"\n")
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
