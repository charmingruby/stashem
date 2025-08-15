package stash

import (
	"bytes"
	"container/list"
	"errors"
	"sync"
	"time"
)

const (
	defaultEntriesLimit = 1000
	defaultMemoryLimit  = 5 * 1024 * 1024 // 5MB
	defaultTTL          = 5 * time.Minute
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

type config struct {
	maxMemory  int
	maxEntries int
}

type Stash struct {
	entries       map[string]*list.Element
	shutdownChan  chan struct{}
	accessList    *list.List
	ttl           time.Duration
	config        config
	currentMemory int
	mu            sync.RWMutex
}

type Option func(*Stash)

func Default(opts ...Option) *Stash {
	c := &Stash{
		ttl:          defaultTTL,
		config:       config{maxMemory: defaultMemoryLimit, maxEntries: defaultEntriesLimit},
		accessList:   list.New(),
		entries:      make(map[string]*list.Element),
		shutdownChan: make(chan struct{}),
	}

	for _, opt := range opts {
		opt(c)
	}

	if c.ttl > 0 {
		go c.cleanupRoutine()
	}

	return c
}

func WithTTL(ttl time.Duration) Option {
	return func(c *Stash) {
		if ttl > 0 {
			c.ttl = ttl
		}
	}
}

func WithMemoryLimit(memory int) Option {
	return func(c *Stash) {
		if memory > 0 {
			c.config.maxMemory = memory
		}
	}
}

func WithEntryLimit(entries int) Option {
	return func(c *Stash) {
		if entries > 0 {
			c.config.maxEntries = entries
		}
	}
}

func (c *Stash) Get(key string) ([]byte, error) {
	c.mu.RLock()
	element, exists := c.entries[key]
	c.mu.RUnlock()

	if !exists {
		return nil, ErrNotFound
	}

	entry := c.getElementEntry(element)
	if entry.expiresAt.Before(time.Now()) {
		c.evictElement(element)
		return nil, ErrExpired
	}

	c.updateAccess(element, entry)

	return entry.data, nil
}

func (c *Stash) Set(key string, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if element, exists := c.entries[key]; exists {
		return c.updateExistingEntry(element, data)
	}

	if len(data) > c.config.maxMemory {
		return ErrInsufficientStorageSize
	}

	needsSpace := c.currentMemory+len(data) > c.config.maxMemory ||
		len(c.entries) >= c.config.maxEntries

	if needsSpace {
		if !c.ensureSpace(len(data)) {
			return ErrInsufficientStorageSize
		}
	}

	c.addNewEntry(key, data)

	return nil
}

func (c *Stash) Shutdown() {
	close(c.shutdownChan)
}

func (c *Stash) getElementEntry(element *list.Element) *entry {
	entry, _ := element.Value.(*entry)
	return entry
}

func (c *Stash) updateAccess(el *list.Element, en *entry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	en.expiresAt = time.Now().Add(c.ttl)
	c.accessList.MoveToFront(el)
}

func (c *Stash) evictElement(element *list.Element) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry := c.getElementEntry(element)
	delete(c.entries, entry.key)
	c.accessList.Remove(element)
	c.currentMemory -= len(entry.data)
}

func (c *Stash) ensureSpace(requiredSpace int) bool {
	if requiredSpace > c.config.maxMemory {
		return false
	}

	for c.currentMemory+requiredSpace <= c.config.maxMemory || len(c.entries) >= c.config.maxEntries {
		if oldest := c.accessList.Back(); oldest != nil {
			c.evictElement(oldest)
			continue
		}

		return false
	}
	return true
}

func (c *Stash) updateExistingEntry(element *list.Element, newData []byte) error {
	entry := c.getElementEntry(element)

	if bytes.Equal(entry.data, newData) {
		entry.expiresAt = time.Now().Add(c.ttl)
		c.accessList.MoveToFront(element)

		return nil
	}

	newSize := c.currentMemory - len(entry.data) + len(newData)
	if newSize > c.config.maxMemory {
		return ErrInsufficientStorageSize
	}

	entry.data = newData
	entry.expiresAt = time.Now().Add(c.ttl)
	c.currentMemory = newSize
	c.accessList.MoveToFront(element)

	return nil
}

func (c *Stash) addNewEntry(key string, data []byte) {
	entry := &entry{
		key:       key,
		data:      data,
		expiresAt: time.Now().Add(c.ttl),
	}

	element := c.accessList.PushFront(entry)
	c.entries[key] = element
	c.currentMemory += len(data)
}

func (c *Stash) cleanupRoutine() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.removeExpiredEntries()
		case <-c.shutdownChan:
			return
		}
	}
}

func (c *Stash) removeExpiredEntries() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for element := c.accessList.Back(); element != nil; {
		entry := c.getElementEntry(element)
		next := element.Prev()

		if entry.expiresAt.Before(now) {
			delete(c.entries, entry.key)
			c.accessList.Remove(element)
			c.currentMemory -= len(entry.data)
		}

		element = next
	}
}
