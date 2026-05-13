package mcache

import "container/list"

// LRUPolicy implements Least Recently Used eviction.
// Front of the list = most recently used; Back = next to evict.
// All operations are O(1).
// The shard lock protects all policy calls; no internal mutex is needed.
type LRUPolicy struct {
	order *list.List
	nodes map[string]*list.Element
}

// NewLRU returns an empty LRU policy.
func NewLRU() *LRUPolicy {
	return &LRUPolicy{
		order: list.New(),
		nodes: make(map[string]*list.Element),
	}
}

func (p *LRUPolicy) Name() string { return "lru" }

func (p *LRUPolicy) OnAccess(key string) {
	if e, ok := p.nodes[key]; ok {
		p.order.MoveToFront(e)
	}
}

func (p *LRUPolicy) OnAdd(key string) {
	if e, ok := p.nodes[key]; ok {
		p.order.MoveToFront(e)
		return
	}
	p.nodes[key] = p.order.PushFront(key)
}

func (p *LRUPolicy) OnRemove(key string) {
	if e, ok := p.nodes[key]; ok {
		p.order.Remove(e)
		delete(p.nodes, key)
	}
}

// Evict removes and returns the least recently used key, if any.
func (p *LRUPolicy) Evict() (string, bool) {
	e := p.order.Back()
	if e == nil {
		return "", false
	}
	key := e.Value.(string)
	p.order.Remove(e)
	delete(p.nodes, key)
	return key, true
}

func (p *LRUPolicy) Len() int {
	return p.order.Len()
}

func (p *LRUPolicy) Clear() {
	p.order.Init()
	p.nodes = make(map[string]*list.Element)
}
