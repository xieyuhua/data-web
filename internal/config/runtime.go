package config

import (
	"sync/atomic"
)

// Runtime holds the live configuration. Services read snapshots from it so that
// admin changes take effect without restarting the process.
type Runtime struct {
	ptr atomic.Pointer[Config]
	// OnChange listeners run after a successful update (e.g. rebuild caches).
	listeners []func(*Config)
}

func NewRuntime(cfg *Config) *Runtime {
	rt := &Runtime{}
	rt.ptr.Store(cfg)
	return rt
}

// Get returns the current configuration snapshot. Callers must treat it as
// read-only; mutate through Update.
func (r *Runtime) Get() *Config {
	return r.ptr.Load()
}

// Update applies fn to a copy, stores it, then notifies listeners.
func (r *Runtime) Update(fn func(*Config)) *Config {
	cur := r.ptr.Load()
	next := *cur // shallow copy; slices/maps are replaced, never mutated in place

	fn(&next)

	r.ptr.Store(&next)
	for _, l := range r.listeners {
		l(&next)
	}
	return &next
}

// OnChange registers a listener invoked after each update.
func (r *Runtime) OnChange(fn func(*Config)) {
	r.listeners = append(r.listeners, fn)
}
