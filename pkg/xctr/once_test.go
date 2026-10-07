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
	"github.com/ctx42/testkit/pkg/dkrkit"

	"github.com/ctx42/xctr/pkg/xctr/xctrtest"
)

func Test_NewOnce(t *testing.T) {
	// --- When ---
	one := NewOnce()

	// --- Then ---
	assert.NotNil(t, one.col)
	assert.Len(t, 0, one.col)
}

func Test_Once_Add(t *testing.T) {
	t.Run("zero value", func(t *testing.T) {
		// --- Given ---
		var one Once
		ctr := NewCTR("ctr0", xctrtest.ImageReq())

		// --- When ---
		have := one.Add(ctr)

		// --- Then ---
		assert.True(t, have)
		assert.Equal(t, []string{"ctr0"}, one.Names())
	})

	t.Run("add to empty", func(t *testing.T) {
		// --- Given ---
		one := NewOnce()
		ctr := NewCTR("ctr0", xctrtest.ImageReq())

		// --- When ---
		have := one.Add(ctr)

		// --- Then ---
		assert.True(t, have)
		assert.Equal(t, []string{"ctr0"}, one.Names())
	})

	t.Run("add not existing", func(t *testing.T) {
		// --- Given ---
		one := NewOnce()
		one.Add(NewCTR("ctr0", xctrtest.ImageReq()))
		ctr := NewCTR("ctr1", xctrtest.ImageReq())

		// --- When ---
		have := one.Add(ctr)

		// --- Then ---
		assert.True(t, have)
		assert.Equal(t, []string{"ctr0", "ctr1"}, one.Names())
	})

	t.Run("add existing", func(t *testing.T) {
		// --- Given ---
		one := NewOnce()
		ctr := NewCTR("ctr0", xctrtest.ImageReq())
		one.Add(ctr)

		// --- When ---
		have := one.Add(NewCTR("ctr0", xctrtest.ImageReq()))

		// --- Then ---
		assert.False(t, have)
		assert.Equal(t, []string{"ctr0"}, one.Names())
		assert.Same(t, ctr, one.Get("ctr0"))
	})
}

func Test_Once_AddNamed(t *testing.T) {
	t.Run("zero value", func(t *testing.T) {
		// --- Given ---
		var one Once
		ctr := NewCTR("ctr0", xctrtest.ImageReq())

		// --- When ---
		have := one.AddNamed("name", ctr)

		// --- Then ---
		assert.True(t, have)
		assert.Equal(t, []string{"name"}, one.Names())
	})

	t.Run("add to empty", func(t *testing.T) {
		// --- Given ---
		one := NewOnce()
		ctr := NewCTR("ctr0", xctrtest.ImageReq())

		// --- When ---
		have := one.AddNamed("my-name", ctr)

		// --- Then ---
		assert.True(t, have)
		assert.Equal(t, []string{"my-name"}, one.Names())
	})

	t.Run("add under different name", func(t *testing.T) {
		// --- Given ---
		one := NewOnce()
		ctr := NewCTR("ctr0", xctrtest.ImageReq())
		one.Add(ctr)

		// --- When ---
		have := one.AddNamed("my-name", ctr)

		// --- Then ---
		assert.True(t, have)
		assert.Equal(t, []string{"ctr0", "my-name"}, one.Names())
		assert.Same(t, ctr, one.Get("ctr0"))
		assert.Same(t, ctr, one.Get("my-name"))
	})

	t.Run("add existing", func(t *testing.T) {
		// --- Given ---
		one := NewOnce()
		ctr := NewCTR("ctr0", xctrtest.ImageReq())
		one.Add(ctr)

		// --- When ---
		have := one.AddNamed("ctr0", NewCTR("ctr1", xctrtest.ImageReq()))

		// --- Then ---
		assert.False(t, have)
		assert.Equal(t, []string{"ctr0"}, one.Names())
		assert.Equal(t, "ctr0", one.Get("ctr0").Name())
	})
}

func Test_once_global(t *testing.T) {
	ctr0 := NewCTR("ctr0", xctrtest.ImageReq())
	ctr1 := NewCTR("ctr1", xctrtest.ImageReq())
	ctr2 := NewCTR("ctr2", xctrtest.ImageReq())

	t.Run("empty", func(t *testing.T) {
		n, err := OnceStopAll(t.Context())
		assert.NoError(t, err)
		assert.Equal(t, 0, n)
		assert.Empty(t, OnceNames())
	})

	t.Run("add first container", func(t *testing.T) {
		// --- When ---
		have := OnceAdd(ctr0)

		// --- Then ---
		assert.True(t, have)
		assert.Same(t, ctr0, OnceGet("ctr0"))
		assert.Equal(t, []string{"ctr0"}, OnceNames())
	})

	t.Run("add second container", func(t *testing.T) {
		// --- When ---
		have := OnceAdd(ctr1)

		// --- Then ---
		assert.True(t, have)
		assert.Same(t, ctr1, OnceGet("ctr1"))
		assert.Equal(t, []string{"ctr0", "ctr1"}, OnceNames())
	})

	t.Run("add named container", func(t *testing.T) {
		// --- When ---
		have := OnceAddNamed("my-name", ctr2)

		// --- Then ---
		assert.True(t, have)
		assert.Same(t, ctr2, OnceGet("my-name"))
		assert.Equal(t, []string{"ctr0", "ctr1", "my-name"}, OnceNames())
	})

	t.Run("get not existing", func(t *testing.T) {
		// --- When ---
		have := OnceGet("not")

		// --- Then ---
		assert.Nil(t, have)
	})

	t.Run("stop not existing", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()

		// --- When ---
		err := OnceStop(ctx, "not")

		// --- Then ---
		assert.NoError(t, err)
	})

	t.Run("add existing container", func(t *testing.T) {
		// --- When ---
		have := OnceAdd(ctr0)

		// --- Then ---
		assert.False(t, have)
		assert.Equal(t, []string{"ctr0", "ctr1", "my-name"}, OnceNames())
	})

	t.Run("add existing named container", func(t *testing.T) {
		// --- When ---
		have := OnceAddNamed("my-name", ctr2)

		// --- Then ---
		assert.False(t, have)
		assert.Equal(t, []string{"ctr0", "ctr1", "my-name"}, OnceNames())
	})

	t.Run("stop existing", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()

		// --- When ---
		err := OnceStop(ctx, "ctr0")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, []string{"ctr1", "my-name"}, OnceNames())
	})

	assert.True(t, OnceAdd(ctr0)) // Add the container 0 back.

	t.Run("stop all", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr1.RegisterCleanup(func(ctx context.Context) error { return errTest })

		// --- When ---
		have, err := OnceStopAll(ctx)

		// --- Then ---
		assert.Equal(t, 3, have)
		assert.ErrorIs(t, errTest, err)
		assert.Empty(t, OnceNames())
	})
}

func Test_Once_Get(t *testing.T) {
	t.Run("not existing", func(t *testing.T) {
		// --- Given ---
		one := NewOnce()

		// --- When ---
		have := one.Get("name")

		// --- Then ---
		assert.Nil(t, have)
	})

	t.Run("get existing", func(t *testing.T) {
		// --- Given ---
		one := NewOnce()
		ctr := NewCTR("ctr0", xctrtest.ImageReq())
		one.Add(ctr)

		// --- When ---
		have := one.Get("ctr0")

		// --- Then ---
		assert.Same(t, ctr, have)
	})
}

// fakeCtr is a [Container] whose Start and Cleanup only count their calls.
type fakeCtr struct {
	*CTR
	startErr error        // Returned by Start.
	starts   atomic.Int32 // Number of Start calls.
	cleanups atomic.Int32 // Number of Cleanup calls.
}

// newFake returns a function creating a [fakeCtr] whose Start returns err.
func newFake(err error) func(name string) *fakeCtr {
	return func(name string) *fakeCtr {
		return &fakeCtr{CTR: NewCTR(name, xctrtest.ImageReq()), startErr: err}
	}
}

func (fak *fakeCtr) Start(context.Context, []string) error {
	fak.starts.Add(1)
	return fak.startErr
}

func (fak *fakeCtr) Cleanup(context.Context) error {
	fak.cleanups.Add(1)
	return nil
}

// asContainer adapts newFn to the signature [Once.Start] takes.
func asContainer(newFn func(string) *fakeCtr) func(string) Container {
	return func(name string) Container { return newFn(name) }
}

func Test_Once_Start(t *testing.T) {
	t.Run("starts and registers", func(t *testing.T) {
		// --- Given ---
		var one Once
		newFn := asContainer(newFake(nil))

		// --- When ---
		have, err := one.Start(t.Context(), nil, "ctr0", newFn)

		// --- Then ---
		assert.NoError(t, err)
		assert.Same(t, have, one.Get("ctr0"))
		assert.Equal(t, int32(1), have.(*fakeCtr).starts.Load())
	})

	t.Run("reuses registered", func(t *testing.T) {
		// --- Given ---
		var one Once
		newFn := asContainer(newFake(nil))
		first := must.Value(one.Start(t.Context(), nil, "ctr0", newFn))

		// --- When ---
		have, err := one.Start(t.Context(), nil, "ctr0", newFn)

		// --- Then ---
		assert.NoError(t, err)
		assert.Same(t, first, have)
		assert.Equal(t, int32(1), have.(*fakeCtr).starts.Load())
	})

	t.Run("concurrent calls start once", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		var one Once
		var created atomic.Int32
		newFn := func(name string) Container {
			created.Add(1)
			return newFake(nil)(name)
		}

		// --- When ---
		var wg sync.WaitGroup
		haves := make([]Container, 10)
		for i := range haves {
			wg.Go(func() {
				haves[i] = must.Value(one.Start(ctx, nil, "ctr0", newFn))
			})
		}
		wg.Wait()

		// --- Then ---
		assert.Equal(t, int32(1), created.Load())
		for _, have := range haves {
			assert.Same(t, haves[0], have)
		}
	})

	t.Run("error - start fails", func(t *testing.T) {
		// --- Given ---
		var one Once
		var fak *fakeCtr
		newFn := func(name string) Container {
			fak = newFake(errTest)(name)
			return fak
		}

		// --- When ---
		have, err := one.Start(t.Context(), nil, "ctr0", newFn)

		// --- Then ---
		assert.ErrorIs(t, errTest, err)
		assert.ErrorContain(t, "start ctr0: ", err)
		assert.Nil(t, have)
		assert.Equal(t, int32(1), fak.cleanups.Load())
		assert.Nil(t, one.Get("ctr0"))
	})

	t.Run("retries after a failed start", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		var one Once
		_, _ = one.Start(ctx, nil, "ctr0", asContainer(newFake(errTest)))

		// --- When ---
		have, err := one.Start(ctx, nil, "ctr0", asContainer(newFake(nil)))

		// --- Then ---
		assert.NoError(t, err)
		assert.Same(t, have, one.Get("ctr0"))
	})

	t.Run("registered while starting", func(t *testing.T) {
		// --- Given ---
		var one Once
		other := newFake(nil)("other")
		var fak *fakeCtr
		newFn := func(name string) Container {
			fak = newFake(nil)(name)
			one.AddNamed(name, other)
			return fak
		}

		// --- When ---
		have, err := one.Start(t.Context(), nil, "ctr0", newFn)

		// --- Then ---
		assert.NoError(t, err)
		assert.Same(t, other, have)
		assert.Equal(t, int32(1), fak.cleanups.Load())
	})
}

func Test_OnceStart(t *testing.T) {
	t.Run("typed container", func(t *testing.T) {
		// --- Given ---
		name := t.Name()
		t.Cleanup(func() { _ = OnceStop(context.Background(), name) })

		// --- When ---
		have, err := OnceStart(t.Context(), nil, name, newFake(nil))

		// --- Then ---
		assert.NoError(t, err)
		assert.Same(t, have, OnceGet(name))
		assert.Equal(t, int32(1), have.starts.Load())
	})

	t.Run("error - start fails", func(t *testing.T) {
		// --- Given ---
		name := t.Name()

		// --- When ---
		have, err := OnceStart(t.Context(), nil, name, newFake(errTest))

		// --- Then ---
		assert.ErrorIs(t, errTest, err)
		assert.Nil(t, have)
	})

	t.Run("error - registered with another type", func(t *testing.T) {
		// --- Given ---
		name := t.Name()
		OnceAddNamed(name, NewCTR(name, xctrtest.ImageReq()))
		t.Cleanup(func() { _ = OnceStop(context.Background(), name) })

		// --- When ---
		have, err := OnceStart(t.Context(), nil, name, newFake(nil))

		// --- Then ---
		wMsg := "start " + name + ": registered container is *xctr.CTR, " +
			"not *xctr.fakeCtr"
		assert.ErrorEqual(t, wMsg, err)
		assert.Nil(t, have)
	})
}

func Test_Once_Stop(t *testing.T) {
	t.Run("stop existing", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr0 := NewCTR("ctr0", xctrtest.ImageReq())
		assert.NoError(t, ctr0.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr0.Cleanup(context.WithoutCancel(ctx)))
		})

		ctr1 := NewCTR("ctr1", xctrtest.ImageReq())
		assert.NoError(t, ctr1.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr1.Cleanup(context.WithoutCancel(ctx)))
		})

		assert.NotNil(t, must.Value(dkrkit.NewT(t).CtrPs().FindByID(ctr0.ID())))
		assert.NotNil(t, must.Value(dkrkit.NewT(t).CtrPs().FindByID(ctr1.ID())))
		t.Cleanup(func() {
			assert.NoError(t, ctr0.Terminate(context.WithoutCancel(ctx)))
		})
		t.Cleanup(func() {
			assert.NoError(t, ctr1.Terminate(context.WithoutCancel(ctx)))
		})

		one := NewOnce()
		one.Add(ctr0)
		one.Add(ctr1)

		// --- When ---
		err := one.Stop(ctx, "ctr0")

		// --- Then ---
		assert.NoError(t, err)
		assert.Len(t, 1, one.col)
		ctrGone(t, ctr0.ID())
		assert.NotNil(t, must.Value(dkrkit.NewT(t).CtrPs().FindByID(ctr1.ID())))
	})

	t.Run("stop not existing", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		one := NewOnce()

		// --- When ---
		err := one.Stop(ctx, "ctr0")

		// --- Then ---
		assert.NoError(t, err)
	})
}

func Test_Once_StopAll(t *testing.T) {
	t.Run("all", func(t *testing.T) {
		// --- Given ---
		ctx := t.Context()
		ctr0 := NewCTR("ctr0", xctrtest.ImageReq())
		assert.NoError(t, ctr0.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr0.Cleanup(context.WithoutCancel(ctx)))
		})

		ctr1 := NewCTR("ctr1", xctrtest.ImageReq())
		assert.NoError(t, ctr1.Start(ctx, nil))
		t.Cleanup(func() {
			assert.NoError(t, ctr1.Cleanup(context.WithoutCancel(ctx)))
		})

		assert.NotNil(t, must.Value(dkrkit.NewT(t).CtrPs().FindByID(ctr0.ID())))
		assert.NotNil(t, must.Value(dkrkit.NewT(t).CtrPs().FindByID(ctr1.ID())))
		t.Cleanup(func() {
			assert.NoError(t, ctr0.Terminate(context.WithoutCancel(ctx)))
		})
		t.Cleanup(func() {
			assert.NoError(t, ctr1.Terminate(context.WithoutCancel(ctx)))
		})

		one := NewOnce()
		one.Add(ctr0)
		one.Add(ctr1)

		// --- When ---
		n, err := one.StopAll(ctx)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, 2, n)
		assert.Len(t, 0, one.col)
		ctrGone(t, ctr0.ID())
		ctrGone(t, ctr1.ID())
	})
}

func Test_Once_Names(t *testing.T) {
	t.Run("with containers", func(t *testing.T) {
		// --- Given ---
		col := map[string]Container{
			"ctr3": nil,
			"ctr0": nil,
			"ctr1": nil,
		}
		one := &Once{col: col}

		// --- When ---
		have := one.Names()

		// --- Then ---
		assert.Equal(t, []string{"ctr0", "ctr1", "ctr3"}, have)
	})

	t.Run("without container", func(t *testing.T) {
		// --- Given ---
		one := &Once{}

		// --- When ---
		have := one.Names()

		// --- Then ---
		assert.Len(t, 0, have)
		assert.NotNil(t, have)
	})
}
