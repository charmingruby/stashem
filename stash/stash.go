package stash

import (
	"bytes"
	"errors"
	"sync"
	"time"
)

var defaultTTL = 5 * time.Minute

var (
	ErrExpired                 = errors.New("key expired")
	ErrInsufficientStorageSize = errors.New("insufficient storage size")
	ErrNotFound                = errors.New("key not found")
)

type entry struct {
	expiresAt time.Time
	data      []byte
}

type StorageLimit struct {
	memory  int
	entries int
}

type Stash struct {
	stopCh chan struct{}
	store  map[string]entry
	limit  StorageLimit
	mu     sync.RWMutex
	ttl    time.Duration
}

type Option func(*Stash)

func New(opts ...Option) *Stash {
	s := &Stash{
		store:  make(map[string]entry),
		ttl:    defaultTTL,
		stopCh: make(chan struct{}),
		limit: StorageLimit{
			memory:  1000,
			entries: 1000,
		},
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

func WithLimit(sl StorageLimit) Option {
	return func(s *Stash) {
		s.limit = sl
	}
}

func (s *Stash) Get(key string) ([]byte, error) {
	s.mu.RLock()
	entry, ok := s.store[key]
	s.mu.RUnlock()

	if !ok {
		return nil, ErrNotFound
	}

	if s.isEntryExpired(entry) {
		s.mu.Lock()
		delete(s.store, key)
		s.mu.Unlock()
		return nil, ErrExpired
	}

	return entry.data, nil
}

func (s *Stash) Set(key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, ok := s.store[key]; ok {
		if bytes.Equal(entry.data, data) {
			entry.expiresAt = time.Now().Add(s.ttl)
			s.store[key] = entry

			return nil
		}

		if !s.hasStorageAvailabilityForUpdate(entry.data, data) {
			return ErrInsufficientStorageSize
		}
	} else if !s.hasStorageAvailabilityForUpdate(nil, data) {
		return ErrInsufficientStorageSize
	}

	s.store[key] = entry{
		data:      data,
		expiresAt: time.Now().Add(s.ttl),
	}
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
	expiredKeys := make([]string, 0)

	s.mu.RLock()
	for k, e := range s.store {
		if time.Now().After(e.expiresAt) {
			expiredKeys = append(expiredKeys, k)
		}
	}
	s.mu.RUnlock()

	if len(expiredKeys) > 0 {
		s.mu.Lock()
		for _, k := range expiredKeys {
			delete(s.store, k)
		}
		s.mu.Unlock()
	}
}

func (s *Stash) hasStorageAvailabilityForUpdate(oldData, newData []byte) bool {
	if len(s.store) >= s.limit.entries && oldData == nil {
		return false
	}

	totalMemory := 0
	for _, v := range s.store {
		totalMemory += len(v.data)
	}

	if oldData != nil {
		totalMemory -= len(oldData)
	}
	totalMemory += len(newData)

	return totalMemory <= s.limit.memory
}

func (s *Stash) isEntryExpired(e entry) bool {
	return e.expiresAt.Before(time.Now())
}
