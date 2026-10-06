// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

// Package xctrtest provides test fixtures and assertions for the xctr package:
// ready-made echo-server container requests, a Docker client helper, and
// container-start assertions.
package xctrtest

import (
	"github.com/ctx42/testing/pkg/assert"
	"github.com/ctx42/testing/pkg/check"
	"github.com/ctx42/testing/pkg/tester"
)

// AssertBuildArg asserts the Docker build args set has given key and value. A
// nil value, which makes Docker use the ARG default, fails the assertion.
func AssertBuildArg(t tester.T, key, want string, set map[string]*string) bool {
	t.Helper()
	val, exist := assert.HasKey(t, key, set)
	if !exist {
		return false
	}
	if err := check.NotNil(val); err != nil {
		t.Error(err)
		return false
	}
	return assert.Equal(t, want, *val)
}
