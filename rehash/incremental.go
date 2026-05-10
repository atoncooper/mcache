package rehash

import "sync"

// IncrementalRehasher performs migration gradually, moving a fixed batch of
// entries on each Step call (Redis-style). This minimizes latency spikes but
// spreads migration overhead across normal operations.
type IncrementalRehasher struct {
	mu        sync.Mutex
	oldShards []Shard
	oldMask   uint32
	rehashing bool
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
	defer r.mu.Unlock()
	r.oldShards = oldShards
	r.oldMask = oldMask
	r.rehashing = true
	r.rehashIdx = 0
}

func (r *IncrementalRehasher) Step(newShards []Shard, indexFunc func(key string) uint32) (done bool, justCompleted bool) {
	r.mu.Lock()
	if !r.rehashing {
		r.mu.Unlock()
		return true, false
	}
	idx := r.rehashIdx
	r.mu.Unlock()

	items := r.oldShards[idx].ExtractN(r.batchSize)
	for _, it := range items {
		newShards[indexFunc(it.Key)].Set(it.Key, it.Value, it.TTL)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(items) == 0 && r.rehashing && r.rehashIdx == idx {
		r.rehashIdx++
		if r.rehashIdx >= len(r.oldShards) {
			r.rehashing = false
			r.oldShards = nil
			return true, true
		}
	}

	return !r.rehashing, false
}

func (r *IncrementalRehasher) IsRehashing() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rehashing
}

func (r *IncrementalRehasher) OldShard(key string) Shard {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.rehashing || r.oldShards == nil {
		return nil
	}
	return r.oldShards[OldShardIndex(key, r.oldMask)]
}

func (r *IncrementalRehasher) OldShards() []Shard {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.rehashing {
		return nil
	}
	return r.oldShards
}

func (r *IncrementalRehasher) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rehashing = false
	r.oldShards = nil
	r.rehashIdx = 0
}
