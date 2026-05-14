package mcache

// CacheObserver receives lifecycle events from the cache.
// Implementations must be safe for concurrent use.
type CacheObserver interface {
	OnHit(key string)
	OnMiss(key string)
	OnSet(key string)
	OnDel(key string)
	OnEvict(key string)
	OnRehashStart(oldShards, newShards int)
	OnRehashDone()
}

// noopObserver is the default no-op observer.
type noopObserver struct{}

func (o *noopObserver) OnHit(key string)                      {}
func (o *noopObserver) OnMiss(key string)                     {}
func (o *noopObserver) OnSet(key string)                      {}
func (o *noopObserver) OnDel(key string)                      {}
func (o *noopObserver) OnEvict(key string)                    {}
func (o *noopObserver) OnRehashStart(oldShards, newShards int) {}
func (o *noopObserver) OnRehashDone()                          {}

// MultiObserver fans out lifecycle events to multiple observers.
// Use it when multiple consumers need to observe cache events
// (e.g. infra metrics + MBR decision engine).
type MultiObserver struct {
	observers []CacheObserver
}

// NewMultiObserver creates a fan-out observer. Nil observers are silently
// skipped, and nested MultiObservers are flattened.
func NewMultiObserver(observers ...CacheObserver) *MultiObserver {
	filtered := make([]CacheObserver, 0, len(observers))
	for _, obs := range observers {
		if obs == nil {
			continue
		}
		if mo, ok := obs.(*MultiObserver); ok {
			filtered = append(filtered, mo.observers...)
		} else if _, isNoop := obs.(*noopObserver); !isNoop {
			filtered = append(filtered, obs)
		}
	}
	return &MultiObserver{observers: filtered}
}

func (m *MultiObserver) OnHit(key string) {
	for _, obs := range m.observers {
		obs.OnHit(key)
	}
}

func (m *MultiObserver) OnMiss(key string) {
	for _, obs := range m.observers {
		obs.OnMiss(key)
	}
}

func (m *MultiObserver) OnSet(key string) {
	for _, obs := range m.observers {
		obs.OnSet(key)
	}
}

func (m *MultiObserver) OnDel(key string) {
	for _, obs := range m.observers {
		obs.OnDel(key)
	}
}

func (m *MultiObserver) OnEvict(key string) {
	for _, obs := range m.observers {
		obs.OnEvict(key)
	}
}

func (m *MultiObserver) OnRehashStart(oldShards, newShards int) {
	for _, obs := range m.observers {
		obs.OnRehashStart(oldShards, newShards)
	}
}

func (m *MultiObserver) OnRehashDone() {
	for _, obs := range m.observers {
		obs.OnRehashDone()
	}
}
