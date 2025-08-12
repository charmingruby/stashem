package stash

import (
	"errors"
	"sync"
	"time"
)

var defaultTTL = 5 * time.Minute

var (
	ErrNotFound = errors.New("key not found")
	ErrExpired  = errors.New("key expired")
)

type entry struct {
	expiresAt time.Time
	data      []byte
}

type Stash struct {
	store map[string]entry
	mu    *sync.RWMutex
	ttl   time.Duration
}

type Option func(*Stash)

func New(opts ...Option) *Stash {
	c := &Stash{
		mu:    &sync.RWMutex{},
		store: make(map[string]entry),
		ttl:   defaultTTL,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

func WithTTL(ttl time.Duration) Option {
	return func(c *Stash) {
		c.ttl = ttl
	}
}

func (c *Stash) Get(key string) ([]byte, error) {
	c.mu.RLock()
	entry, ok := c.store[key]
	c.mu.RUnlock()

	if !ok {
		return nil, ErrNotFound
	}

	now := time.Now()

	isExpired := entry.expiresAt.Before(now)

	if isExpired {
		c.mu.Lock()
		delete(c.store, key)
		c.mu.Unlock()

		return nil, ErrExpired
	}

	return entry.data, nil
}

func (c *Stash) Set(key string, value []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	if entry, ok := c.store[key]; ok {
		entry.expiresAt = now.Add(c.ttl)
		c.store[key] = entry

		return nil
	}

	entry := entry{
		data:      value,
		expiresAt: now.Add(c.ttl),
	}

	c.store[key] = entry

	return nil
}
