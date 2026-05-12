package rehash

import (
	"sync"
	"sync/atomic"
)

// BatchRehasher migrates all remaining entries in a single Step call.
// This causes a one-time latency spike but avoids prolonged double-table
// overhead. Useful when downtime or brief pauses are acceptable.
//
// The `rehashing` flag is an atomic.Bool to allow lock-free fast-path checks
// from the cache hot path. All read methods return without touching the mutex
// when no rehash is in progress.
type BatchRehasher struct {
	rehashing atomic.Bool
	mu        sync.Mutex
	oldShards []Shard
	oldMask   uint32
}

// NewBatchRehasher creates a batch rehasher.
func NewBatchRehasher() *BatchRehasher {
	return &BatchRehasher{}
}

func (r *BatchRehasher) Name() string { return "batch" }

func (r *BatchRehasher) Start(oldShards []Shard, oldMask uint32) {
	r.mu.Lock()
	r.oldShards = oldShards
	r.oldMask = oldMask
	r.mu.Unlock()
	r.rehashing.Store(true)
}

func (r *BatchRehasher) Step(newShards []Shard, indexFunc func(key string) uint32) (done bool, justCompleted bool) {
	if !r.rehashing.Load() {
		return true, false
	}
	r.mu.Lock()
	if !r.rehashing.Load() {
		r.mu.Unlock()
		return true, false
	}
	old := r.oldShards
	r.mu.Unlock()

	// Migrate all old shards in one go.
	for _, oldShard := range old {
		for {
			items := oldShard.ExtractN(1000)
			if len(items) == 0 {
				break
			}
			for _, it := range items {
				newShards[indexFunc(it.Key)].Set(it.Key, it.Value, it.TTL)
			}
		}
	}

	r.mu.Lock()
	r.rehashing.Store(false)
	r.oldShards = nil
	r.mu.Unlock()
	return true, true
}

func (r *BatchRehasher) IsRehashing() bool {
	return r.rehashing.Load()
}

func (r *BatchRehasher) OldShard(key string) Shard {
	if !r.rehashing.Load() {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.oldShards == nil {
		return nil
	}
	return r.oldShards[OldShardIndex(key, r.oldMask)]
}

func (r *BatchRehasher) OldShards() []Shard {
	if !r.rehashing.Load() {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.oldShards
}

func (r *BatchRehasher) Stop() {
	r.rehashing.Store(false)
	r.mu.Lock()
	r.oldShards = nil
	r.mu.Unlock()
}
