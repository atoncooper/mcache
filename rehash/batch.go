package rehash

import "sync"

// BatchRehasher migrates all remaining entries in a single Step call.
// This causes a one-time latency spike but avoids prolonged double-table
// overhead. Useful when downtime or brief pauses are acceptable.
type BatchRehasher struct {
	mu        sync.Mutex
	oldShards []Shard
	oldMask   uint32
	rehashing bool
}

// NewBatchRehasher creates a batch rehasher.
func NewBatchRehasher() *BatchRehasher {
	return &BatchRehasher{}
}

func (r *BatchRehasher) Name() string { return "batch" }

func (r *BatchRehasher) Start(oldShards []Shard, oldMask uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.oldShards = oldShards
	r.oldMask = oldMask
	r.rehashing = true
}

func (r *BatchRehasher) Step(newShards []Shard, indexFunc func(key string) uint32) (done bool, justCompleted bool) {
	r.mu.Lock()
	if !r.rehashing {
		r.mu.Unlock()
		return true, false
	}
	r.mu.Unlock()

	// Migrate all old shards in one go.
	for _, oldShard := range r.oldShards {
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
	r.rehashing = false
	r.oldShards = nil
	r.mu.Unlock()
	return true, true
}

func (r *BatchRehasher) IsRehashing() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rehashing
}

func (r *BatchRehasher) OldShard(key string) Shard {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.rehashing || r.oldShards == nil {
		return nil
	}
	return r.oldShards[OldShardIndex(key, r.oldMask)]
}

func (r *BatchRehasher) OldShards() []Shard {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.rehashing {
		return nil
	}
	return r.oldShards
}

func (r *BatchRehasher) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rehashing = false
	r.oldShards = nil
}
