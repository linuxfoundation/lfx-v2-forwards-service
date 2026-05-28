// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import "fmt"

// serviceUnavailableError indicates a downstream dependency is not reachable.
type serviceUnavailableError struct {
	message string
	cause   error
}

func (e *serviceUnavailableError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.message, e.cause)
	}
	return e.message
}

func (e *serviceUnavailableError) Unwrap() error { return e.cause }

func newServiceUnavailable(msg string, cause ...error) error {
	e := &serviceUnavailableError{message: msg}
	if len(cause) > 0 {
		e.cause = cause[0]
	}
	return e
}
