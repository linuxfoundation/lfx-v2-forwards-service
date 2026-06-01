// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package model

import "errors"

// ErrAliasNotFound is returned by a ForwardEmailProvider when an alias does not
// exist (the forwardemail.net adapter maps an HTTP 404 to this sentinel).
var ErrAliasNotFound = errors.New("alias not found")

// ErrNoAliasForDomain is returned by an AuthServiceClient when the caller has no
// identity (alias) on the requested domain.
var ErrNoAliasForDomain = errors.New("no alias found for user on requested domain")
