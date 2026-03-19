// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package provider

import "strings"

const redactedMarker = "\"REDACTED\""

// RedactTemplateForLog removes secret-bearing values from rendered OpenNebula templates.
func RedactTemplateForLog(value string) string {
	lines := strings.Split(value, "\n")

	for index, line := range lines {
		if !strings.Contains(line, "USER_DATA") || strings.Contains(line, "USER_DATA_ENCODING") {
			continue
		}

		prefix, _, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		lines[index] = strings.TrimRight(prefix, " ") + " = " + redactedMarker
	}

	return strings.Join(lines, "\n")
}
