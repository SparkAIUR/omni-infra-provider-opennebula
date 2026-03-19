// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package provider

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/siderolabs/omni/client/pkg/infra/provision"
)

// BootstrapPayload returns the base64-encoded initial Talos config for USER_DATA.
func BootstrapPayload(connectionParams provision.ConnectionParams, hostname string) string {
	hostnameDoc := fmt.Sprintf("machine:\n  network:\n    hostname: %s\n", hostname)
	payload := strings.TrimSpace(connectionParams.JoinConfig)
	if payload != "" {
		payload += "\n---\n" + hostnameDoc
	} else {
		payload = hostnameDoc
	}

	return base64.StdEncoding.EncodeToString([]byte(payload))
}
