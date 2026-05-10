package mcache

import (
	"container/list"
	"sync"
)

// LRUPolicy implements Least Recently Used eviction.
// Front of the list = most recently used; Back = next to evict.
// All operations are O(1).
type LRUPolicy struct {
	mu    sync.Mutex
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
	p.mu.Lock()
	defer p.mu.Unlock()
	if e, ok := p.nodes[key]; ok {
		p.order.MoveToFront(e)
	}
}

func (p *LRUPolicy) OnAdd(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if e, ok := p.nodes[key]; ok {
		p.order.MoveToFront(e)
		return
	}
	p.nodes[key] = p.order.PushFront(key)
}

func (p *LRUPolicy) OnRemove(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if e, ok := p.nodes[key]; ok {
		p.order.Remove(e)
		delete(p.nodes, key)
	}
}

// Evict removes and returns the least recently used key, if any.
func (p *LRUPolicy) Evict() (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
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
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.order.Len()
}

func (p *LRUPolicy) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.order.Init()
	p.nodes = make(map[string]*list.Element)
}
