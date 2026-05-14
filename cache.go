package mcache

import (
	"errors"
	"hash/fnv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/atoncooper/mcache/rehash"
	"github.com/atoncooper/mcache/ds/set"
)

// shardTable is an immutable snapshot of the shard layout.
// It is stored atomically so the hot path (getShard) can read it
// without acquiring any locks.
type shardTable struct {
	shards []rehash.Shard
	mask   uint32
}

// Cache is a thread-safe in-memory cache with sharding for reduced lock contention.
// Supports pluggable rehashing strategies to resize shard count.
//
// `shards` is accessed atomically so the hot path (getShard) can look up a shard
// without acquiring a lock. `closed` is atomic so validateOpen is also lock-free.
// `mu` serialises Resize calls and guards observer swaps.
type Cache struct {
	shards atomic.Value // *shardTable
	opts   Options
	closed atomic.Bool
	mu     sync.Mutex

	rehasher rehash.Rehasher

	policyName    string
	policyFactory PolicyFactory
	policyMu      sync.RWMutex
	perShardMax   int

	observer CacheObserver
}

// New creates a Cache with the given options.
func New(opts Options) (*Cache, error) {
	if opts.shardCount <= 0 || (opts.shardCount&(opts.shardCount-1)) != 0 {
		return nil, ErrInvalidShards
	}
	if opts.maxSize < 0 {
		opts.maxSize = 0
	}
	if opts.defaultTTL < 0 {
		return nil, ErrNegativeTTL
	}
	if opts.evictionPolicy == "" {
		opts.evictionPolicy = "noop"
	}
	factory, err := getPolicyFactory(opts.evictionPolicy)
	if err != nil {
		return nil, err
	}

	if opts.rehasher == "" {
		opts.rehasher = "incremental"
	}
	rFactory, err := rehash.GetFactory(opts.rehasher)
	if err != nil {
		return nil, err
	}

	perShardMax := 0
	if opts.maxSize > 0 {
		perShardMax = opts.maxSize / opts.shardCount
		if perShardMax < 1 {
			perShardMax = 1
		}
	}

	if opts.observer == nil {
		opts.observer = &noopObserver{}
	}
	shards := make([]rehash.Shard, opts.shardCount)
	for i := range shards {
		shards[i] = newShard(factory, perShardMax, opts.observer)
	}
	c := &Cache{
		opts:          opts,
		policyName:    opts.evictionPolicy,
		policyFactory: factory,
		perShardMax:   perShardMax,
		observer:      opts.observer,
		rehasher:      rFactory(),
	}
	c.shards.Store(&shardTable{
		shards: shards,
		mask:   uint32(opts.shardCount - 1),
	})
	return c, nil
}

// SetEvictionPolicy hot-swaps the eviction strategy at runtime.
// It replaces the policy in every shard and migrates existing keys.
func (c *Cache) SetEvictionPolicy(name string) error {
	factory, err := getPolicyFactory(name)
	if err != nil {
		return err
	}
	if err := c.validateOpen(); err != nil {
		return err
	}

	c.policyMu.Lock()
	c.policyName = name
	c.policyFactory = factory
	c.policyMu.Unlock()

	table := c.loadShardTable()
	for _, s := range table.shards {
		s.(*shard).swapPolicy(factory)
	}

	for _, s := range c.rehasher.OldShards() {
		if s != nil {
			s.(*shard).swapPolicy(factory)
		}
	}
	return nil
}

// SetRehasher hot-swaps the rehashing strategy at runtime.
// If a rehash is in progress, it is aborted and the new strategy takes over
// on the next Resize.
func (c *Cache) SetRehasher(name string) error {
	factory, err := rehash.GetFactory(name)
	if err != nil {
		return err
	}
	if err := c.validateOpen(); err != nil {
		return err
	}

	c.rehasher.Stop()
	c.rehasher = factory()
	return nil
}

// Rehasher returns the current rehasher name.
func (c *Cache) Rehasher() string {
	return c.rehasher.Name()
}

// EvictionPolicy returns the current eviction policy name.
func (c *Cache) EvictionPolicy() string {
	c.policyMu.RLock()
	defer c.policyMu.RUnlock()
	return c.policyName
}

// Resize starts rehashing to a new shard count (must be power of two).
func (c *Cache) Resize(shardCount int) error {
	if err := c.validateOpen(); err != nil {
		return err
	}
	if shardCount <= 0 || (shardCount&(shardCount-1)) != 0 {
		return ErrInvalidShards
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed.Load() {
		return ErrCacheClosed
	}
	if c.rehasher.IsRehashing() {
		return errors.New("rehash already in progress")
	}

	c.policyMu.RLock()
	factory := c.policyFactory
	c.policyMu.RUnlock()

	perShardMax := 0
	if c.opts.maxSize > 0 {
		perShardMax = c.opts.maxSize / shardCount
		if perShardMax < 1 {
			perShardMax = 1
		}
	}

	oldTable := c.loadShardTable()
	c.observer.OnRehashStart(len(oldTable.shards), shardCount)
	c.rehasher.Start(oldTable.shards, oldTable.mask)
	newShards := make([]rehash.Shard, shardCount)
	for i := range newShards {
		newShards[i] = newShard(factory, perShardMax, c.observer)
	}
	c.shards.Store(&shardTable{
		shards: newShards,
		mask:   uint32(shardCount - 1),
	})
	return nil
}

// IsRehashing reports whether a rehash is in progress.
func (c *Cache) IsRehashing() bool {
	return c.rehasher.IsRehashing()
}

// loadShardTable returns the current shard table atomically.
func (c *Cache) loadShardTable() *shardTable {
	return c.shards.Load().(*shardTable)
}

func (c *Cache) shardIndex(key string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return h.Sum32() & c.loadShardTable().mask
}

func (c *Cache) getShard(key string) *shard {
	table := c.loadShardTable()
	return table.shards[shardIndexFor(key, table.mask)].(*shard)
}

func shardIndexFor(key string, mask uint32) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return h.Sum32() & mask
}

// Set stores value under key with optional TTL override (0 = use default, <0 = no expiry).
func (c *Cache) Set(key string, value []byte, ttl ...time.Duration) error {
	if err := c.validateOpen(); err != nil {
		return err
	}
	if err := validateKeyValue(key, value); err != nil {
		return err
	}
	var d time.Duration
	if len(ttl) > 0 {
		d = max(ttl[0], 0)
	} else {
		d = c.opts.defaultTTL
	}
	shard := c.getShard(key)
	evicted := shard.Set(key, value, d)
	c.observer.OnSet(key)
	for _, ek := range evicted {
		c.observer.OnEvict(ek)
	}

	if c.rehasher.IsRehashing() {
		if oldShard := c.rehasher.OldShard(key); oldShard != nil {
			oldShard.Del(key)
		}
		table := c.loadShardTable()
		_, justCompleted := c.rehasher.Step(table.shards, c.shardIndex)
		if justCompleted {
			c.observer.OnRehashDone()
		}
	}
	return nil
}

// Get retrieves a value by key.
func (c *Cache) Get(key string) ([]byte, error) {
	if err := c.validateOpen(); err != nil {
		return nil, err
	}
	if key == "" {
		return nil, ErrKeyEmpty
	}
	val, ok := c.getShard(key).get(key)
	if ok {
		c.observer.OnHit(key)
		if c.rehasher.IsRehashing() {
			table := c.loadShardTable()
			_, justCompleted := c.rehasher.Step(table.shards, c.shardIndex)
			if justCompleted {
				c.observer.OnRehashDone()
			}
		}
		return val, nil
	}

	if c.rehasher.IsRehashing() {
		if oldShard := c.rehasher.OldShard(key); oldShard != nil {
			val, ok = oldShard.(*shard).get(key)
			if ok {
				c.observer.OnHit(key)
				table := c.loadShardTable()
				_, justCompleted := c.rehasher.Step(table.shards, c.shardIndex)
				if justCompleted {
					c.observer.OnRehashDone()
				}
				return val, nil
			}
		}
	}

	c.observer.OnMiss(key)
	if c.rehasher.IsRehashing() {
		table := c.loadShardTable()
		_, justCompleted := c.rehasher.Step(table.shards, c.shardIndex)
		if justCompleted {
			c.observer.OnRehashDone()
		}
	}
	return nil, ErrKeyNotFound
}

// Del removes a key from the cache.
func (c *Cache) Del(key string) error {
	if err := c.validateOpen(); err != nil {
		return err
	}
	if key == "" {
		return ErrKeyEmpty
	}
	c.getShard(key).Del(key)
	c.observer.OnDel(key)

	if c.rehasher.IsRehashing() {
		if oldShard := c.rehasher.OldShard(key); oldShard != nil {
			oldShard.Del(key)
		}
		table := c.loadShardTable()
		_, justCompleted := c.rehasher.Step(table.shards, c.shardIndex)
		if justCompleted {
			c.observer.OnRehashDone()
		}
	}
	return nil
}

// Len returns the total number of active entries across all shards.
func (c *Cache) Len() int {
	if c.closed.Load() {
		return 0
	}
	table := c.loadShardTable()
	total := 0
	for _, s := range table.shards {
		total += s.Len()
	}
	for _, s := range c.rehasher.OldShards() {
		total += s.Len()
	}
	return total
}

// ShardCount returns the current number of shards.
func (c *Cache) ShardCount() int {
	if c.closed.Load() {
		return 0
	}
	return len(c.loadShardTable().shards)
}

// Close shuts down the cache.
func (c *Cache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed.Load() {
		return ErrCacheClosed
	}
	c.closed.Store(true)
	c.shards.Store(&shardTable{})
	c.rehasher.Stop()
	return nil
}

// SetObserver replaces the current CacheObserver at runtime.
func (c *Cache) SetObserver(obs CacheObserver) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if obs == nil {
		obs = &noopObserver{}
	}
	c.observer = obs
	table := c.loadShardTable()
	for _, s := range table.shards {
		s.(*shard).observer = obs
	}
	for _, s := range c.rehasher.OldShards() {
		if s != nil {
			s.(*shard).observer = obs
		}
	}
}

// Cleanup removes all expired entries and returns count removed.
func (c *Cache) Cleanup() int {
	if c.closed.Load() {
		return 0
	}
	table := c.loadShardTable()
	total := 0
	for _, s := range table.shards {
		total += s.(*shard).cleanup()
	}
	for _, s := range c.rehasher.OldShards() {
		total += s.(*shard).cleanup()
	}
	return total
}

// --- Set operations ---

// SAdd adds elements to the set at key. Creates the set if it doesn't exist.
func (c *Cache) SAdd(key string, elems ...string) (int, error) {
	if err := c.validateOpen(); err != nil {
		return 0, err
	}
	if key == "" {
		return 0, ErrKeyEmpty
	}
	return c.getShard(key).sAdd(key, elems...), nil
}

// SRem removes elements from the set at key.
func (c *Cache) SRem(key string, elems ...string) (int, error) {
	if err := c.validateOpen(); err != nil {
		return 0, err
	}
	if key == "" {
		return 0, ErrKeyEmpty
	}
	return c.getShard(key).sRem(key, elems...), nil
}

// SIsMember tests whether elem is in the set at key.
func (c *Cache) SIsMember(key, elem string) (bool, error) {
	if err := c.validateOpen(); err != nil {
		return false, err
	}
	if key == "" {
		return false, ErrKeyEmpty
	}
	return c.getShard(key).sIsMember(key, elem), nil
}

// SMembers returns all elements in the set at key.
func (c *Cache) SMembers(key string) ([]string, error) {
	if err := c.validateOpen(); err != nil {
		return nil, err
	}
	if key == "" {
		return nil, ErrKeyEmpty
	}
	return c.getShard(key).sMembers(key), nil
}

// SCard returns the number of elements in the set.
func (c *Cache) SCard(key string) (int, error) {
	if err := c.validateOpen(); err != nil {
		return 0, err
	}
	if key == "" {
		return 0, ErrKeyEmpty
	}
	return c.getShard(key).sCard(key), nil
}

// SPop removes and returns a random element from the set.
func (c *Cache) SPop(key string) (string, error) {
	if err := c.validateOpen(); err != nil {
		return "", err
	}
	if key == "" {
		return "", ErrKeyEmpty
	}
	elem, ok := c.getShard(key).sPop(key)
	if !ok {
		return "", ErrKeyNotFound
	}
	return elem, nil
}

// SRandMember returns random elements from the set.
func (c *Cache) SRandMember(key string, count int) ([]string, error) {
	if err := c.validateOpen(); err != nil {
		return nil, err
	}
	if key == "" {
		return nil, ErrKeyEmpty
	}
	return c.getShard(key).sRandMember(key, count), nil
}

// SUnion returns the union of multiple sets across shards.
func (c *Cache) SUnion(keys ...string) ([]string, error) {
	if err := c.validateOpen(); err != nil {
		return nil, err
	}
	sets := make([]*set.Set, 0, len(keys))
	for _, key := range keys {
		for _, s := range c.loadShardTable().shards {
			if ss := s.(*shard).getSet(key); ss != nil {
				sets = append(sets, ss)
				break
			}
		}
	}
	if len(sets) == 0 {
		return nil, nil
	}
	return set.Union(sets...).Members(), nil
}

// SInter returns the intersection of multiple sets.
func (c *Cache) SInter(keys ...string) ([]string, error) {
	if err := c.validateOpen(); err != nil {
		return nil, err
	}
	sets := make([]*set.Set, 0, len(keys))
	for _, key := range keys {
		found := false
		for _, s := range c.loadShardTable().shards {
			if ss := s.(*shard).getSet(key); ss != nil {
				sets = append(sets, ss)
				found = true
				break
			}
		}
		if !found {
			return nil, nil // one missing → empty intersection
		}
	}
	return set.Inter(sets...).Members(), nil
}

// SDiff returns elements in first key not in any other key.
func (c *Cache) SDiff(keys ...string) ([]string, error) {
	if err := c.validateOpen(); err != nil {
		return nil, err
	}
	sets := make([]*set.Set, 0, len(keys))
	for _, key := range keys {
		found := false
		for _, s := range c.loadShardTable().shards {
			if ss := s.(*shard).getSet(key); ss != nil {
				sets = append(sets, ss)
				found = true
				break
			}
		}
		if !found {
			sets = append(sets, set.New()) // missing set = empty
		}
	}
	return set.Diff(sets...).Members(), nil
}

// validateOpen is on the cache hot path (called for every Set/Get/Del). It
// must avoid taking any locks; use the atomic flag instead. The RWMutex is
// only used for shard-table swaps in Resize().
func (c *Cache) validateOpen() error {
	if c.closed.Load() {
		return ErrCacheClosed
	}
	return nil
}

func validateKeyValue(key string, value []byte) error {
	if key == "" {
		return ErrKeyEmpty
	}
	if value == nil {
		return ErrValueNil
	}
	return nil
}
