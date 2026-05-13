package mcache

import "container/list"

// lfuItem is the node payload in the per-frequency lists.
type lfuItem struct {
	key  string
	freq int
}

// LFUPolicy implements an O(1) Least Frequently Used eviction (Ketan Shah et al., 2010).
// State:
//   - items:    key  -> list element pointer (across freqLists)
//   - freqLists: freq -> doubly linked list of lfuItem (front = oldest at that freq)
//   - minFreq:  smallest frequency that currently has any items
//
// All operations are amortized O(1) except Clear (O(n)).
// The shard lock protects all policy calls; no internal mutex is needed.
type LFUPolicy struct {
	items     map[string]*list.Element
	freqLists map[int]*list.List
	minFreq   int
}

// NewLFU returns an empty LFU policy.
func NewLFU() *LFUPolicy {
	return &LFUPolicy{
		items:     make(map[string]*list.Element),
		freqLists: make(map[int]*list.List),
		minFreq:   0,
	}
}

func (p *LFUPolicy) Name() string { return "lfu" }

func (p *LFUPolicy) OnAccess(key string) {
	p.bump(key)
}

func (p *LFUPolicy) OnAdd(key string) {
	if _, ok := p.items[key]; ok {
		p.bump(key)
		return
	}
	item := &lfuItem{key: key, freq: 1}
	bucket, ok := p.freqLists[1]
	if !ok {
		bucket = list.New()
		p.freqLists[1] = bucket
	}
	p.items[key] = bucket.PushBack(item)
	p.minFreq = 1
}

func (p *LFUPolicy) OnRemove(key string) {
	elem, ok := p.items[key]
	if !ok {
		return
	}
	item := elem.Value.(*lfuItem)
	p.removeFromBucket(elem, item.freq)
	delete(p.items, key)
}

// Evict removes and returns the key with the lowest frequency.
// When multiple keys share the lowest frequency, the oldest one wins.
func (p *LFUPolicy) Evict() (string, bool) {
	if len(p.items) == 0 {
		return "", false
	}
	bucket, ok := p.freqLists[p.minFreq]
	if !ok || bucket.Len() == 0 {
		p.minFreq = p.scanMinFreq()
		if p.minFreq == 0 {
			return "", false
		}
		bucket = p.freqLists[p.minFreq]
	}
	elem := bucket.Front()
	item := elem.Value.(*lfuItem)
	p.removeFromBucket(elem, item.freq)
	delete(p.items, item.key)
	return item.key, true
}

func (p *LFUPolicy) Len() int {
	return len(p.items)
}

func (p *LFUPolicy) Clear() {
	p.items = make(map[string]*list.Element)
	p.freqLists = make(map[int]*list.List)
	p.minFreq = 0
}

// bump increments the frequency of an existing key. Caller must hold shard lock.
func (p *LFUPolicy) bump(key string) {
	elem, ok := p.items[key]
	if !ok {
		return
	}
	item := elem.Value.(*lfuItem)
	oldFreq := item.freq
	p.removeFromBucket(elem, oldFreq)
	item.freq++
	bucket, ok := p.freqLists[item.freq]
	if !ok {
		bucket = list.New()
		p.freqLists[item.freq] = bucket
	}
	p.items[key] = bucket.PushBack(item)
}

// removeFromBucket removes elem from freqLists[freq] and adjusts minFreq if the
// bucket becomes empty. Caller must hold shard lock.
func (p *LFUPolicy) removeFromBucket(elem *list.Element, freq int) {
	bucket := p.freqLists[freq]
	bucket.Remove(elem)
	if bucket.Len() == 0 {
		delete(p.freqLists, freq)
		if p.minFreq == freq {
			p.minFreq = p.scanMinFreq()
		}
	}
}

// scanMinFreq finds the smallest frequency with a non-empty bucket.
// Returns 0 if no buckets remain. Caller must hold shard lock.
// O(k) where k is the number of distinct frequencies, which is bounded by Len().
func (p *LFUPolicy) scanMinFreq() int {
	if len(p.freqLists) == 0 {
		return 0
	}
	min := -1
	for f := range p.freqLists {
		if min < 0 || f < min {
			min = f
		}
	}
	if min < 0 {
		return 0
	}
	return min
}
