// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctr

import (
	"context"
	"errors"
)

// CleanupFn is a cleanup callback function run by [CleanupTrait.Cleanup].
type CleanupFn func(context.Context) error

// CleanupTrait provides registration and execution of cleanup callback
// functions. Embed it in another struct to give that struct cleanup
// capabilities.
type CleanupTrait struct {
	// Slice of cleanup functions called from Cleanup method.
	cleanup []CleanupFn
}

// RegisterCleanup registers a custom cleanup function to be called by the
// Cleanup method. Implements a fluent interface.
func (cle *CleanupTrait) RegisterCleanup(fns ...CleanupFn) *CleanupTrait {
	cle.cleanup = append(cle.cleanup, fns...)
	return cle
}

// Cleanup runs all registered cleanup functions in the reverse order.
func (cle *CleanupTrait) Cleanup(ctx context.Context) error {
	var ers error

	// Run cleanup functions in reverse.
	for i := len(cle.cleanup) - 1; i >= 0; i-- {
		if err := cle.cleanup[i](ctx); err != nil {
			ers = errors.Join(ers, err)
		}
	}

	// Cleanups can be run only once.
	cle.cleanup = cle.cleanup[:0]
	return ers
}
