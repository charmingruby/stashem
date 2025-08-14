package stash

import (
	"bytes"
	"errors"
	"sync"
	"time"
)

const defaultEntriesLimit = 1000

var (
	defaultTTL         = 5 * time.Minute
	defaultMemoryLimit = 5 * 1024 * 1024 // 5mb
)

var (
	ErrExpired                 = errors.New("key expired")
	ErrInsufficientStorageSize = errors.New("insufficient storage size")
	ErrNotFound                = errors.New("key not found")
)

type entry struct {
	expiresAt time.Time
	data      []byte
}

type storageLimit struct {
	memory  int
	entries int
}

type Stash struct {
	store      map[string]entry
	limit      storageLimit
	mu         sync.RWMutex
	ttl        time.Duration
	stopCh     chan struct{}
	usedMemory int
}

type Option func(*Stash)

func Default(opts ...Option) *Stash {
	s := &Stash{
		ttl: defaultTTL,
		limit: storageLimit{
			memory:  defaultMemoryLimit,
			entries: defaultEntriesLimit,
		},
		store:  make(map[string]entry),
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

func WithMemoryLimit(memory int) Option {
	return func(s *Stash) {
		if memory != 0 {
			s.limit.memory = memory
		}
	}
}

func WithEntriesLimit(entries int) Option {
	return func(s *Stash) {
		if entries != 0 {
			s.limit.entries = entries
		}
	}
}

func (s *Stash) Get(key string) ([]byte, error) {
	s.mu.RLock()
	e, ok := s.store[key]
	s.mu.RUnlock()

	if !ok {
		return nil, ErrNotFound
	}

	if e.expiresAt.Before(time.Now()) {
		s.mu.Lock()
		delete(s.store, key)
		s.usedMemory -= len(e.data)
		s.mu.Unlock()

		return nil, ErrExpired
	}

	return e.data, nil
}

func (s *Stash) Set(key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, exists := s.store[key]
	dataToBeOverwrittenSize := 0
	if exists {
		if bytes.Equal(existing.data, data) {
			existing.expiresAt = time.Now().Add(s.ttl)
			s.store[key] = existing
			return nil
		}

		dataToBeOverwrittenSize = len(existing.data)
	}

	newMemoryUsage := s.usedMemory - dataToBeOverwrittenSize + len(data)
	if newMemoryUsage > s.limit.memory {
		return ErrInsufficientStorageSize
	}

	if !exists && len(s.store) >= s.limit.entries {
		return ErrInsufficientStorageSize
	}

	s.store[key] = entry{
		data:      data,
		expiresAt: time.Now().Add(s.ttl),
	}

	s.usedMemory = newMemoryUsage

	return nil
}

func (s *Stash) Stop() {
	close(s.stopCh)
}

func (s *Stash) cleanupRoutine() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanup()
		case <-s.stopCh:
			return
		}
	}
}

func (s *Stash) cleanup() {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	for k, e := range s.store {
		if e.expiresAt.Before(now) {
			s.usedMemory -= len(e.data)
			delete(s.store, k)
		}
	}
}
