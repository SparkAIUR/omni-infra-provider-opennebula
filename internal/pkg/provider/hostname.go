// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package provider

import "fmt"

// HostnameConfigPatch renders the managed hostname patch for Omni.
func HostnameConfigPatch(hostname string) []byte {
	return []byte(fmt.Sprintf("machine:\n  network:\n    hostname: %s\n", hostname))
}
