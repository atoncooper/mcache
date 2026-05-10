package mcache

import "sync"

// EvictionPolicy decides which key to remove when a shard is over its size limit.
// Implementations must be safe for concurrent use.
type EvictionPolicy interface {
	Name() string
	OnAccess(key string)
	OnAdd(key string)
	OnRemove(key string)
	Evict() (string, bool)
	Len() int
	Clear()
}

// PolicyFactory creates a fresh policy instance. Each shard owns one.
type PolicyFactory func() EvictionPolicy

var (
	policyRegistryMu sync.RWMutex
	policyRegistry   = map[string]PolicyFactory{
		"noop": func() EvictionPolicy { return &noopPolicy{} },
		"lru":  func() EvictionPolicy { return NewLRU() },
		"lfu":  func() EvictionPolicy { return NewLFU() },
	}
)

// RegisterPolicy adds a custom policy factory under the given name.
// Built-in names ("noop", "lru", "lfu") may be overridden.
func RegisterPolicy(name string, factory PolicyFactory) error {
	if name == "" || factory == nil {
		return ErrUnknownPolicy
	}
	policyRegistryMu.Lock()
	defer policyRegistryMu.Unlock()
	policyRegistry[name] = factory
	return nil
}

func getPolicyFactory(name string) (PolicyFactory, error) {
	policyRegistryMu.RLock()
	defer policyRegistryMu.RUnlock()
	f, ok := policyRegistry[name]
	if !ok {
		return nil, ErrUnknownPolicy
	}
	return f, nil
}

// noopPolicy is a placeholder when eviction is not desired (maxSize == 0).
type noopPolicy struct{}

func (p *noopPolicy) Name() string          { return "noop" }
func (p *noopPolicy) OnAccess(_ string)     {}
func (p *noopPolicy) OnAdd(_ string)        {}
func (p *noopPolicy) OnRemove(_ string)     {}
func (p *noopPolicy) Evict() (string, bool) { return "", false }
func (p *noopPolicy) Len() int              { return 0 }
func (p *noopPolicy) Clear()                {}
