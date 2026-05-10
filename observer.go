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
