package stash

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dummy struct {
	ID string `json:"id"`
}

func Test_Stash_Default(t *testing.T) {
	t.Run("returns Stash with default values", func(t *testing.T) {
		s := Default()

		assert.Equal(t, defaultTTL, s.ttl)
		assert.Empty(t, s.store)
	})

	t.Run("applies custom ttl duration", func(t *testing.T) {
		customTTL := 10 * time.Minute

		s := Default(WithTTL(customTTL))

		assert.Equal(t, customTTL, s.ttl)
		assert.Empty(t, s.store)
	})

	t.Run("applies custom limits", func(t *testing.T) {
		entries := 5
		memory := 500

		s := Default(WithMemoryLimit(memory), WithEntriesLimit(entries))
		assert.Equal(t, entries, s.limit.entries)
		assert.Equal(t, memory, s.limit.memory)
	})
}

func Test_Stash_Get(t *testing.T) {
	key := "dummy-key"
	value := dummy{ID: "id"}
	valueInBytes, err := json.Marshal(value)
	require.NoError(t, err)

	t.Run("returns data from a valid key", func(t *testing.T) {
		stash := Default()

		stash.store[key] = entry{data: valueInBytes, expiresAt: time.Now().Add(defaultTTL)}
		stash.usedMemory += len(valueInBytes)

		data, err := stash.Get(key)
		require.NoError(t, err)

		var parsed dummy
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Equal(t, value.ID, parsed.ID)
	})

	t.Run("returns error if key is expired", func(t *testing.T) {
		stash := Default()
		stash.store[key] = entry{data: valueInBytes, expiresAt: time.Now().Add(-time.Minute)}
		stash.usedMemory += len(valueInBytes)

		data, err := stash.Get(key)
		assert.ErrorIs(t, err, ErrExpired)
		assert.Nil(t, data)
	})

	t.Run("returns error if key does not exist", func(t *testing.T) {
		stash := Default()

		data, err := stash.Get(key)
		assert.ErrorIs(t, err, ErrNotFound)
		assert.Nil(t, data)
	})
}

func Test_Stash_Set(t *testing.T) {
	key := "dummy-key"
	value := dummy{ID: "id"}
	valueInBytes, err := json.Marshal(value)
	require.NoError(t, err)

	t.Run("stores a new entry", func(t *testing.T) {
		s := Default()

		err := s.Set(key, valueInBytes)
		require.NoError(t, err)

		stored, exists := s.store[key]
		require.True(t, exists)
		assert.Equal(t, valueInBytes, stored.data)
		assert.True(t, stored.expiresAt.After(time.Now()))
	})

	t.Run("updates existing entry with different value", func(t *testing.T) {
		s := Default()
		oldValue := dummy{ID: "old-id"}
		oldBytes, _ := json.Marshal(oldValue)
		s.store[key] = entry{data: oldBytes, expiresAt: time.Now().Add(defaultTTL)}
		s.usedMemory += len(oldBytes)

		err := s.Set(key, valueInBytes)
		require.NoError(t, err)

		updated, exists := s.store[key]
		require.True(t, exists)
		assert.Equal(t, valueInBytes, updated.data)
		assert.True(t, updated.expiresAt.After(time.Now()))
	})

	t.Run("resets expiration if value is equal", func(t *testing.T) {
		s := Default()
		s.store[key] = entry{data: valueInBytes, expiresAt: time.Now().Add(-time.Minute)}
		s.usedMemory += len(valueInBytes)

		oldExpiration := s.store[key].expiresAt
		err := s.Set(key, valueInBytes)
		require.NoError(t, err)

		updated := s.store[key]
		assert.Equal(t, valueInBytes, updated.data)
		assert.True(t, updated.expiresAt.After(oldExpiration))
	})

	t.Run("returns error if entry exceeds memory limit", func(t *testing.T) {
		data := []byte("123")
		maxMemory := len(data)

		s := Default(WithMemoryLimit(maxMemory), WithEntriesLimit(5))
		err := s.Set("k1", data)
		require.NoError(t, err)

		err = s.Set("k1", []byte("123456"))
		assert.ErrorIs(t, err, ErrInsufficientStorageSize)
	})

	t.Run("returns error if entry limit exceeded", func(t *testing.T) {
		s := Default(WithMemoryLimit(100), WithEntriesLimit(1))
		err := s.Set("k1", []byte("data"))
		require.NoError(t, err)

		err = s.Set("k2", []byte("data2"))
		assert.ErrorIs(t, err, ErrInsufficientStorageSize)
	})
}

func Test_Stash_Cleanup(t *testing.T) {
	key := "dummy-key"
	value := dummy{ID: "id"}
	valueInBytes, _ := json.Marshal(value)

	s := Default()
	s.store[key] = entry{data: valueInBytes, expiresAt: time.Now().Add(-time.Minute)}
	s.usedMemory += len(valueInBytes)

	s.cleanup()

	data, err := s.Get(key)
	assert.ErrorIs(t, err, ErrNotFound)
	assert.Nil(t, data)
}
