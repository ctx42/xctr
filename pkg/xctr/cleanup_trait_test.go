// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctr

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ctx42/testing/pkg/assert"
	"github.com/ctx42/testing/pkg/must"
)

func Test_CleanupTrait_RegisterCleanup(t *testing.T) {
	t.Run("concurrent registration", func(t *testing.T) {
		// --- Given ---
		cln := &CleanupTrait{}
		var calls atomic.Int32
		fn := func(context.Context) error {
			calls.Add(1)
			return nil
		}

		// --- When ---
		var wg sync.WaitGroup
		for range 10 {
			wg.Go(func() { cln.RegisterCleanup(fn) })
		}
		wg.Wait()

		// --- Then ---
		assert.NoError(t, cln.Cleanup(t.Context()))
		assert.Equal(t, int32(10), calls.Load())
	})
}

func Test_CleanupTrait_Cleanup(t *testing.T) {
	t.Run("no cleanups registered", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		cln := &CleanupTrait{}

		// --- When ---
		err := cln.Cleanup(ctx)

		// --- Then ---
		assert.NoError(t, err)
	})

	t.Run("registered cleanups run in reverse order", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		cln := &CleanupTrait{}

		var order []string
		cb0 := func(context.Context) error {
			order = append(order, "0")
			return nil
		}
		cb1 := func(context.Context) error {
			order = append(order, "1")
			return nil
		}

		cln.RegisterCleanup(cb0, cb1)

		// --- When ---
		err := cln.Cleanup(ctx)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, []string{"1", "0"}, order)
	})

	t.Run("errors are collected", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		cln := &CleanupTrait{}

		var order []string

		cb0 := func(context.Context) error {
			order = append(order, "0")
			return nil
		}
		cb1 := func(context.Context) error {
			order = append(order, "1")
			return errTest
		}
		cb2 := func(context.Context) error {
			order = append(order, "2")
			return nil
		}
		cb3 := func(context.Context) error {
			order = append(order, "3")
			return errTestOther
		}

		cln.RegisterCleanup(cb0, cb1, cb2, cb3)

		// --- When ---
		err := cln.Cleanup(ctx)

		// --- Then ---
		assert.ErrorIs(t, errTest, err)
		assert.ErrorIs(t, errTestOther, err)

		assert.Equal(t, []string{"3", "2", "1", "0"}, order)
	})

	t.Run("cleanup registers another cleanup", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		cln := &CleanupTrait{}

		var called bool
		cln.RegisterCleanup(func(context.Context) error {
			cln.RegisterCleanup(func(context.Context) error {
				called = true
				return nil
			})
			return nil
		})

		// --- When ---
		err := cln.Cleanup(ctx)

		// --- Then ---
		assert.NoError(t, err)
		assert.False(t, called)
		assert.NoError(t, cln.Cleanup(ctx))
		assert.True(t, called)
	})

	t.Run("registered cleanups run only once", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		cln := &CleanupTrait{}

		var order []string
		cb0 := func(context.Context) error {
			order = append(order, "0")
			return nil
		}
		cb1 := func(context.Context) error {
			order = append(order, "1")
			return nil
		}

		cln.RegisterCleanup(cb0, cb1)
		must.Nil(cln.Cleanup(ctx))

		// --- When ---
		err := cln.Cleanup(ctx)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, []string{"1", "0"}, order)
	})
}
