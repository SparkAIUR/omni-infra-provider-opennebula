// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
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
