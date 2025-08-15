package stash

import (
	"bytes"
	"container/list"
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
	ErrInvalidEntryType        = errors.New("invalid entry type")
	ErrInsufficientStorageSize = errors.New("insufficient storage size")
	ErrNotFound                = errors.New("key not found")
)

type entry struct {
	key       string
	expiresAt time.Time
	data      []byte
}

type storageLimit struct {
	memory  int
	entries int
}

type Stash struct {
	store      map[string]*list.Element
	stopCh     chan struct{}
	order      *list.List
	limit      storageLimit
	mu         sync.RWMutex
	ttl        time.Duration
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
		order:  list.New(),
		store:  make(map[string]*list.Element),
		stopCh: make(chan struct{}),
	}

	for _, opt := range opts {
		opt(s)
	}

	if s.ttl > 0 {
		go s.cleanupRoutine()
	}

	return s
}

func WithTTL(ttl time.Duration) Option {
	return func(s *Stash) {
		s.ttl = ttl
	}
}

func WithMemoryLimit(memory int) Option {
	return func(s *Stash) {
		if memory > 0 {
			s.limit.memory = memory
		}
	}
}

func WithEntriesLimit(entries int) Option {
	return func(s *Stash) {
		if entries > 0 {
			s.limit.entries = entries
		}
	}
}

func (s *Stash) Get(key string) ([]byte, error) {
	s.mu.RLock()
	element, ok := s.store[key]
	s.mu.RUnlock()

	if !ok {
		return nil, ErrNotFound
	}

	entry, ok := element.Value.(*entry)
	if !ok {
		return nil, ErrInvalidEntryType
	}

	if entry.expiresAt.Before(time.Now()) {
		s.mu.Lock()
		s.order.Remove(element)
		delete(s.store, key)
		s.usedMemory -= len(entry.data)
		s.mu.Unlock()

		return nil, ErrExpired
	}

	s.mu.Lock()
	s.order.MoveToFront(element)
	s.mu.Unlock()

	return entry.data, nil
}

func (s *Stash) Set(key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if element, exists := s.store[key]; exists {
		existing, ok := element.Value.(*entry)
		if !ok {
			return ErrInvalidEntryType
		}

		if bytes.Equal(existing.data, data) {
			existing.expiresAt = time.Now().Add(s.ttl)
			s.order.MoveToFront(element)

			return nil
		}

		newMemoryUsage := s.usedMemory - len(existing.data) + len(data)
		if newMemoryUsage > s.limit.memory {
			return ErrInsufficientStorageSize
		}

		s.usedMemory = newMemoryUsage
		existing.data = data
		existing.expiresAt = time.Now().Add(s.ttl)
		s.order.MoveToFront(element)
		return nil
	}

	if len(data) > s.limit.memory {
		return ErrInsufficientStorageSize
	}

	if s.usedMemory+len(data) > s.limit.memory {
		return ErrInsufficientStorageSize
	}

	if len(s.store) >= s.limit.entries {
		return ErrInsufficientStorageSize
	}

	newEntry := &entry{
		key:       key,
		data:      data,
		expiresAt: time.Now().Add(s.ttl),
	}

	element := s.order.PushFront(newEntry)
	s.store[key] = element
	s.usedMemory += len(data)

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

	for element := s.order.Back(); element != nil; {
		entry, ok := element.Value.(*entry)

		if !ok {
			continue
		}

		next := element.Prev()

		if entry.expiresAt.Before(now) {
			s.usedMemory -= len(entry.data)
			delete(s.store, entry.key)
			s.order.Remove(element)
		}

		element = next
	}
}
