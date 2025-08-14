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
	store  map[string]entry
	mu     *sync.RWMutex
	ttl    time.Duration
	stopCh chan struct{}
}

type Option func(*Stash)

func New(opts ...Option) *Stash {
	s := &Stash{
		mu:     &sync.RWMutex{},
		store:  make(map[string]entry),
		ttl:    defaultTTL,
		stopCh: make(chan struct{}),
	}

	for _, opt := range opts {
		opt(s)
	}

	go s.cleanupRoutine()

	return s
}

func WithTTL(ttl time.Duration) Option {
	return func(s *Stash) {
		s.ttl = ttl
	}
}

func (s *Stash) Get(key string) ([]byte, error) {
	s.mu.RLock()
	entry, ok := s.store[key]
	s.mu.RUnlock()

	if !ok {
		return nil, ErrNotFound
	}

	now := time.Now()

	isExpired := entry.expiresAt.Before(now)

	if isExpired {
		s.mu.Lock()
		delete(s.store, key)
		s.mu.Unlock()

		return nil, ErrExpired
	}

	return entry.data, nil
}

func (s *Stash) Set(key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	if entry, ok := s.store[key]; ok {
		entry.expiresAt = now.Add(s.ttl)
		s.store[key] = entry

		return nil
	}

	entry := entry{
		data:      value,
		expiresAt: now.Add(s.ttl),
	}

	s.store[key] = entry

	return nil
}

func (s *Stash) Stop() {
	close(s.stopCh)
}

func (s *Stash) cleanupRoutine() {
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	for k, e := range s.store {
		if now.After(e.expiresAt) {
			delete(s.store, k)
		}
	}
}
