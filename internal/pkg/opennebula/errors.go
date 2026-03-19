// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package opennebula

import "errors"

var (
	// ErrNotFound indicates that the requested resource does not exist.
	ErrNotFound = errors.New("opennebula resource not found")
)
