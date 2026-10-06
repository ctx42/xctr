// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctr

import (
	"context"
	"errors"
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
	col map[string]Container // Maps container names to their implementations.
	mx  sync.RWMutex         // Guards struct fields.
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
