package mcache

import "github.com/atoncooper/mcache/rehash"

// RehasherFactory is an alias for backward compatibility.
type RehasherFactory = rehash.RehasherFactory

// RegisterRehasher adds a custom rehasher factory under the given name.
func RegisterRehasher(name string, factory RehasherFactory) error {
	return rehash.Register(name, factory)
}
