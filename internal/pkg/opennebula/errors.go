// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package opennebula

import (
	"errors"
	"strings"
)

var (
	// ErrNotFound indicates that the requested resource does not exist.
	ErrNotFound = errors.New("opennebula resource not found")
	// ErrAuth indicates that the OpenNebula credentials are invalid or expired.
	ErrAuth = errors.New("opennebula auth error")
	// ErrQuota indicates that quota or capacity rules prevented the operation.
	ErrQuota = errors.New("opennebula quota error")
	// ErrPolicy indicates that policy validation or placement rules blocked the request.
	ErrPolicy = errors.New("opennebula policy error")
	// ErrRetryable indicates a transient OpenNebula API failure.
	ErrRetryable = errors.New("opennebula retryable error")
	// ErrTerminal indicates a non-retryable OpenNebula API failure.
	ErrTerminal = errors.New("opennebula terminal error")
)

// ErrorClass labels normalized OpenNebula failure groups for retries, state, and metrics.
type ErrorClass string

const (
	ErrorClassSuccess   ErrorClass = "success"
	ErrorClassUnknown   ErrorClass = "unknown"
	ErrorClassNotFound  ErrorClass = "not_found"
	ErrorClassAuth      ErrorClass = "auth"
	ErrorClassQuota     ErrorClass = "quota"
	ErrorClassPolicy    ErrorClass = "policy"
	ErrorClassRetryable ErrorClass = "retryable"
	ErrorClassTerminal  ErrorClass = "terminal"
)

// ClassifyError returns the normalized OpenNebula error class for an operation failure.
func ClassifyError(err error) ErrorClass {
	if err == nil {
		return ErrorClassSuccess
	}

	switch {
	case errors.Is(err, ErrNotFound):
		return ErrorClassNotFound
	case errors.Is(err, ErrAuth):
		return ErrorClassAuth
	case errors.Is(err, ErrQuota):
		return ErrorClassQuota
	case errors.Is(err, ErrPolicy):
		return ErrorClassPolicy
	case errors.Is(err, ErrRetryable):
		return ErrorClassRetryable
	case errors.Is(err, ErrTerminal):
		return ErrorClassTerminal
	}

	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "not found"):
		return ErrorClassNotFound
	case strings.Contains(message, "unauthorized"), strings.Contains(message, "forbidden"), strings.Contains(message, "authentication"):
		return ErrorClassAuth
	case strings.Contains(message, "quota"), strings.Contains(message, "capacity"), strings.Contains(message, "limit exceeded"):
		return ErrorClassQuota
	case strings.Contains(message, "policy"), strings.Contains(message, "placement"), strings.Contains(message, "forbidden host"):
		return ErrorClassPolicy
	case strings.Contains(message, "timeout"), strings.Contains(message, "temporar"), strings.Contains(message, "connection reset"), strings.Contains(message, "eof"):
		return ErrorClassRetryable
	default:
		return ErrorClassUnknown
	}
}

// IsNotFoundError reports whether the error represents a missing OpenNebula resource.
func IsNotFoundError(err error) bool {
	return ClassifyError(err) == ErrorClassNotFound
}

// IsRetryableClass reports whether an error class should trigger provider retries.
func IsRetryableClass(class ErrorClass) bool {
	return class == ErrorClassRetryable || class == ErrorClassQuota || class == ErrorClassUnknown
}
