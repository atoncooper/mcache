package mcache

import (
	"time"

	mnet "github.com/atoncooper/mcache/net"
)

// Client is a high-level SDK client for a single mcache server node.
type Client struct {
	transport *mnet.Client
	codec     Codec
}

// NewClient creates a client connected to the given server address.
func NewClient(addr string, opts ...Option) (*Client, error) {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}

	transport, err := mnet.NewClient(addr,
		mnet.WithPoolSize(o.PoolSize),
		mnet.WithDialTimeout(o.DialTimeout),
		mnet.WithClientReadTimeout(o.ReadTimeout),
		mnet.WithClientWriteTimeout(o.WriteTimeout),
	)
	if err != nil {
		return nil, err
	}

	return &Client{
		transport: transport,
		codec:     o.Codec,
	}, nil
}

// Get retrieves a value and unmarshals it into dest.
func (c *Client) Get(key string, dest any) error {
	if key == "" {
		return ErrKeyEmpty
	}
	if dest == nil {
		return ErrValueNil
	}

	data, err := c.transport.Get(key)
	if err != nil {
		if err.Error() == "key not found" {
			return ErrKeyNotFound
		}
		return err
	}
	return c.codec.Unmarshal(data, dest)
}

// Set marshals value and stores it under key with optional TTL.
func (c *Client) Set(key string, value any, ttl time.Duration) error {
	if key == "" {
		return ErrKeyEmpty
	}
	if value == nil {
		return ErrValueNil
	}

	data, err := c.codec.Marshal(value)
	if err != nil {
		return err
	}
	return c.transport.Set(key, data, ttl)
}

// Del removes a key from the cache.
func (c *Client) Del(key string) error {
	if key == "" {
		return ErrKeyEmpty
	}
	return c.transport.Del(key)
}

// Len returns the total number of active entries.
func (c *Client) Len() (int, error) {
	return c.transport.Len()
}

// Cleanup triggers expiration cleanup and returns count removed.
func (c *Client) Cleanup() (int, error) {
	return c.transport.Cleanup()
}

// Close closes all underlying connections.
func (c *Client) Close() error {
	return c.transport.Close()
}
