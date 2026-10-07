// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctr

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
)

// once is the package-level [Once] used by the Once* helpers. It is a
// deliberate convenience singleton — like http.DefaultClient — letting
// callers track containers without threading an [Once] value through
// their tests. Use a [NewOnce] instance directly to avoid shared state.
var once = NewOnce()

// Once keeps track of started [CTR] instances. The zero value is ready to use.
type Once struct {
	col   map[string]Container   // Containers by name.
	locks map[string]*sync.Mutex // Serialize [Once.Start] calls per name.
	mx    sync.RWMutex           // Guards struct fields.
}

// NewOnce returns a new instance of [Once].
func NewOnce() *Once { return &Once{col: make(map[string]Container)} }

// Add adds a container to the collection. If the container with the same name
// already exists in the collection, it will not be overridden. Returns true if
// the container has been added to the collection, false otherwise.
func (onc *Once) Add(ctr Container) bool {
	onc.mx.Lock()
	defer onc.mx.Unlock()
	if _, ok := onc.col[ctr.Name()]; ok {
		return false
	}
	if onc.col == nil {
		onc.col = make(map[string]Container)
	}
	onc.col[ctr.Name()] = ctr
	return true
}

// OnceAdd adds the container to the package-level collection. See [Once.Add].
func OnceAdd(ctr Container) bool { return once.Add(ctr) }

// AddNamed adds a container with a custom name to the collection. If the
// container with the same name already exists in the collection, it will not be
// overridden. Returns true if the container has been added to the collection,
// false otherwise.
func (onc *Once) AddNamed(name string, ctr Container) bool {
	onc.mx.Lock()
	defer onc.mx.Unlock()
	if _, ok := onc.col[name]; ok {
		return false
	}
	if onc.col == nil {
		onc.col = make(map[string]Container)
	}
	onc.col[name] = ctr
	return true
}

// OnceAddNamed adds the container to the package-level collection under name.
// See [Once.AddNamed].
func OnceAddNamed(name string, ctr Container) bool {
	return once.AddNamed(name, ctr)
}

// Get returns the container tracked by its name. Returns nil if it doesn't
// exist in the collection.
func (onc *Once) Get(name string) Container {
	onc.mx.RLock()
	defer onc.mx.RUnlock()
	if ctr, ok := onc.col[name]; ok {
		return ctr
	}
	return nil
}

// OnceGet returns the container tracked in the package-level collection by
// name or nil. See [Once.Get].
func OnceGet(name string) Container { return once.Get(name) }

// Start returns the container registered under name. On first use it creates
// the container with newFn, starts it with env, and registers it; concurrent
// calls for the same name wait for that start instead of starting another. A
// failed start is cleaned up and not registered, so a later call tries again.
func (onc *Once) Start(
	ctx context.Context,
	env []string,
	name string,
	newFn func(name string) Container,
) (Container, error) {

	lck := onc.lock(name)
	lck.Lock()
	defer lck.Unlock()

	if ctr := onc.Get(name); ctr != nil {
		return ctr, nil
	}
	ctr := newFn(name)
	if err := ctr.Start(ctx, env); err != nil {
		err = fmt.Errorf("start %s: %w", name, err)
		return nil, errors.Join(err, ctr.Cleanup(context.WithoutCancel(ctx)))
	}
	if !onc.AddNamed(name, ctr) {
		// Registered by Add or AddNamed while this one was starting.
		_ = ctr.Cleanup(context.WithoutCancel(ctx))
		if ctr = onc.Get(name); ctr == nil {
			return nil, fmt.Errorf("start %s: removed while starting", name)
		}
	}
	return ctr, nil
}

// lock returns the mutex serializing [Once.Start] calls for name.
func (onc *Once) lock(name string) *sync.Mutex {
	onc.mx.Lock()
	defer onc.mx.Unlock()
	if onc.locks == nil {
		onc.locks = make(map[string]*sync.Mutex)
	}
	lck, ok := onc.locks[name]
	if !ok {
		lck = &sync.Mutex{}
		onc.locks[name] = lck
	}
	return lck
}

// OnceStart returns the container registered under name in the package-level
// collection, creating and starting it with newFn on first use. It returns an
// error when a container of another type is registered under name. See
// [Once.Start].
func OnceStart[T Container](
	ctx context.Context,
	env []string,
	name string,
	newFn func(name string) T,
) (T, error) {

	var zero T
	ctr, err := once.Start(ctx, env, name, func(name string) Container {
		return newFn(name)
	})
	if err != nil {
		return zero, err
	}
	typed, ok := ctr.(T)
	if !ok {
		format := "start %s: registered container is %T, not %T"
		return zero, fmt.Errorf(format, name, ctr, zero)
	}
	return typed, nil
}

// Stop stops the container with the given name and removes it from the
// collection.
func (onc *Once) Stop(ctx context.Context, name string) error {
	onc.mx.Lock()
	ctr, ok := onc.col[name]
	if ok {
		delete(onc.col, name)
	}
	onc.mx.Unlock()
	if !ok {
		return nil
	}
	return ctr.Cleanup(ctx)
}

// OnceStop stops the container with the given name and removes it from the
// package-level collection. See [Once.Stop].
func OnceStop(ctx context.Context, name string) error {
	return once.Stop(ctx, name)
}

// StopAll calls [Container.Cleanup] on all tracked containers and removes them
// from the collection. It returns the number of containers that were tracked
// and a joined error from any cleanup failures.
func (onc *Once) StopAll(ctx context.Context) (int, error) {
	onc.mx.Lock()
	names := slices.Collect(maps.Keys(onc.col))
	ctrs := make([]Container, 0, len(names))
	for _, name := range names {
		ctrs = append(ctrs, onc.col[name])
		delete(onc.col, name)
	}
	onc.mx.Unlock()

	var ers error
	for _, ctr := range ctrs {
		if err := ctr.Cleanup(ctx); err != nil {
			ers = errors.Join(ers, err)
		}
	}
	return len(ctrs), ers
}

// OnceStopAll stops all containers in the package-level collection. See
// [Once.StopAll].
func OnceStopAll(ctx context.Context) (int, error) {
	return once.StopAll(ctx)
}

// Names returns a sorted slice of container names in the collection.
func (onc *Once) Names() []string {
	onc.mx.RLock()
	defer onc.mx.RUnlock()
	keys := make([]string, 0, len(onc.col))
	for name := range onc.col {
		keys = append(keys, name)
	}
	slices.Sort(keys)
	return keys
}

// OnceNames returns a sorted slice of container names in the package-level
// collection. See [Once.Names].
func OnceNames() []string { return once.Names() }
