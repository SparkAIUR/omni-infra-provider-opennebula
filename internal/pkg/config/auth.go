// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package config

import (
	"errors"
	"os"
)

const (
	// SessionEnvVar is the raw OpenNebula session env var.
	SessionEnvVar = "OPENNEBULA_SESSION"
	// UsernameEnvVar is the OpenNebula username env var.
	UsernameEnvVar = "OPENNEBULA_USERNAME"
	// PasswordEnvVar is the OpenNebula password env var.
	PasswordEnvVar = "OPENNEBULA_PASSWORD"
)

// ResolveAuth resolves auth from the supported environment variables.
func ResolveAuth() (AuthConfig, error) {
	session := os.Getenv(SessionEnvVar)
	if session != "" {
		return AuthConfig{Session: session}, nil
	}

	username := os.Getenv(UsernameEnvVar)
	password := os.Getenv(PasswordEnvVar)
	if username == "" || password == "" {
		return AuthConfig{}, errors.New("set OPENNEBULA_SESSION or OPENNEBULA_USERNAME and OPENNEBULA_PASSWORD")
	}

	return AuthConfig{
		Username: username,
		Password: password,
	}, nil
}
