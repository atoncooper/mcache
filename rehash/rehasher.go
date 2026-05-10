package rehash

import (
	"hash/fnv"
	"sync"
	"time"
)

// Item represents a single cache entry being migrated.
type Item struct {
	Key   string
	Value []byte
	TTL   time.Duration
}

// Shard is the minimal interface a rehasher needs to interact with cache shards.
type Shard interface {
	ExtractN(n int) []Item
	Set(key string, value []byte, ttl time.Duration) []string
	Del(key string)
	Len() int
}

// Rehasher defines a pluggable strategy for migrating data from an old shard
// layout to a new one. Implementations control the pacing and granularity of
// the migration.
type Rehasher interface {
	Name() string
	// Start begins a rehash with the old shard table and mask.
	Start(oldShards []Shard, oldMask uint32)
	// Step executes one migration step into newShards using indexFunc to locate
	// the destination shard. It returns (done, justCompleted) where done is true
	// if the rehash is fully complete, and justCompleted is true only when this
	// specific call caused the rehash to finish.
	Step(newShards []Shard, indexFunc func(key string) uint32) (done bool, justCompleted bool)
	// IsRehashing reports whether a rehash is currently in progress.
	IsRehashing() bool
	// OldShard returns the shard in the old table responsible for key, or nil.
	OldShard(key string) Shard
	// OldShards returns the full old shard table, or nil if not rehashing.
	// Callers must not modify the returned shards.
	OldShards() []Shard
	// Stop forcibly aborts the rehash and releases old state.
	Stop()
}

// RehasherFactory creates a fresh Rehasher instance.
type RehasherFactory func() Rehasher

var (
	registryMu sync.RWMutex
	registry   = map[string]RehasherFactory{
		"incremental": func() Rehasher { return NewIncrementalRehasher() },
		"batch":       func() Rehasher { return NewBatchRehasher() },
		"noop":        func() Rehasher { return NewNoopRehasher() },
	}
)

// Register adds a custom rehasher factory under the given name.
func Register(name string, factory RehasherFactory) error {
	if name == "" || factory == nil {
		return ErrUnknownRehasher
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[name] = factory
	return nil
}

// GetFactory retrieves a rehasher factory by name.
func GetFactory(name string) (RehasherFactory, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	f, ok := registry[name]
	if !ok {
		return nil, ErrUnknownRehasher
	}
	return f, nil
}

// noopRehasher is a no-op rehasher that never migrates.
type noopRehasher struct{}

// NewNoopRehasher creates a no-op rehasher.
func NewNoopRehasher() *noopRehasher {
	return &noopRehasher{}
}

func (r *noopRehasher) Name() string                                     { return "noop" }
func (r *noopRehasher) Start(oldShards []Shard, oldMask uint32)          {}
func (r *noopRehasher) Step(newShards []Shard, indexFunc func(key string) uint32) (done bool, justCompleted bool) {
	return true, false
}
func (r *noopRehasher) IsRehashing() bool                               { return false }
func (r *noopRehasher) OldShard(key string) Shard                        { return nil }
func (r *noopRehasher) OldShards() []Shard                              { return nil }
func (r *noopRehasher) Stop()                                           {}

// OldShardIndex computes the shard index for a key using the given mask.
func OldShardIndex(key string, mask uint32) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return h.Sum32() & mask
}
