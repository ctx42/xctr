// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctr

import (
	"context"
	"errors"
	"sync"
)

// CleanupFn is a cleanup callback function run by [CleanupTrait.Cleanup].
type CleanupFn func(context.Context) error

// CleanupTrait provides registration and execution of cleanup callback
// functions. Embed it in another struct to give that struct cleanup
// capabilities. It is safe for concurrent use and must not be copied after
// first use.
type CleanupTrait struct {
	// Slice of cleanup functions called from Cleanup method.
	cleanup []CleanupFn

	// Guards cleanup.
	mx sync.Mutex
}

// RegisterCleanup registers a custom cleanup function to be called by the
// Cleanup method. Implements a fluent interface.
func (cle *CleanupTrait) RegisterCleanup(fns ...CleanupFn) *CleanupTrait {
	cle.mx.Lock()
	defer cle.mx.Unlock()
	cle.cleanup = append(cle.cleanup, fns...)
	return cle
}

// Cleanup runs all registered cleanup functions in the reverse order.
func (cle *CleanupTrait) Cleanup(ctx context.Context) error {
	// Cleanups can be run only once. The lock is not held while they run, so
	// a cleanup function may register another one.
	cle.mx.Lock()
	fns := cle.cleanup
	cle.cleanup = nil
	cle.mx.Unlock()

	// Run cleanup functions in reverse.
	var ers error
	for i := len(fns) - 1; i >= 0; i-- {
		if err := fns[i](ctx); err != nil {
			ers = errors.Join(ers, err)
		}
	}
	return ers
}
