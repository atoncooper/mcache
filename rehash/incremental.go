package rehash

import (
	"sync"
	"sync/atomic"
)

// IncrementalRehasher performs migration gradually, moving a fixed batch of
// entries on each Step call (Redis-style). This minimizes latency spikes but
// spreads migration overhead across normal operations.
//
// The `rehashing` flag is an atomic.Bool to allow lock-free fast-path checks
// from the cache hot path. Step/IsRehashing/OldShard/OldShards all return
// without touching the mutex when no rehash is in progress.
type IncrementalRehasher struct {
	rehashing atomic.Bool
	mu        sync.Mutex
	oldShards []Shard
	oldMask   uint32
	rehashIdx int
	batchSize int
}

// NewIncrementalRehasher creates an incremental rehasher with a default batch
// size of 16 entries per step.
func NewIncrementalRehasher() *IncrementalRehasher {
	return &IncrementalRehasher{batchSize: 16}
}

func (r *IncrementalRehasher) Name() string { return "incremental" }

func (r *IncrementalRehasher) Start(oldShards []Shard, oldMask uint32) {
	r.mu.Lock()
	r.oldShards = oldShards
	r.oldMask = oldMask
	r.rehashIdx = 0
	r.mu.Unlock()
	r.rehashing.Store(true)
}

func (r *IncrementalRehasher) Step(newShards []Shard, indexFunc func(key string) uint32) (done bool, justCompleted bool) {
	if !r.rehashing.Load() {
		return true, false
	}
	r.mu.Lock()
	if !r.rehashing.Load() {
		r.mu.Unlock()
		return true, false
	}
	idx := r.rehashIdx
	oldShard := r.oldShards[idx]
	r.mu.Unlock()

	items := oldShard.ExtractN(r.batchSize)
	for _, it := range items {
		newShards[indexFunc(it.Key)].Set(it.Key, it.Value, it.TTL)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(items) == 0 && r.rehashing.Load() && r.rehashIdx == idx {
		r.rehashIdx++
		if r.rehashIdx >= len(r.oldShards) {
			r.rehashing.Store(false)
			r.oldShards = nil
			return true, true
		}
	}

	return !r.rehashing.Load(), false
}

func (r *IncrementalRehasher) IsRehashing() bool {
	return r.rehashing.Load()
}

func (r *IncrementalRehasher) OldShard(key string) Shard {
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

func (r *IncrementalRehasher) OldShards() []Shard {
	if !r.rehashing.Load() {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.oldShards
}

func (r *IncrementalRehasher) Stop() {
	r.rehashing.Store(false)
	r.mu.Lock()
	r.oldShards = nil
	r.rehashIdx = 0
	r.mu.Unlock()
}
