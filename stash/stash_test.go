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
		assert.NotNil(t, s.accessList)
		assert.Empty(t, s.entries)
	})

	t.Run("applies custom ttl duration", func(t *testing.T) {
		customTTL := 10 * time.Minute

		s := Default(WithTTL(customTTL))

		assert.Equal(t, customTTL, s.ttl)
		assert.NotNil(t, s.accessList)
		assert.Empty(t, s.entries)
	})

	t.Run("applies custom limits", func(t *testing.T) {
		entries := 5
		memory := 500

		s := Default(WithMemoryLimit(memory), WithEntryLimit(entries))
		assert.Equal(t, entries, s.config.maxEntries)
		assert.Equal(t, memory, s.config.maxMemory)
	})
}

func Test_Stash_Get(t *testing.T) {
	key := "dummy-key"
	value := dummy{ID: "id"}
	valueInBytes, err := json.Marshal(value)
	require.NoError(t, err)

	t.Run("returns data from a valid key", func(t *testing.T) {
		stash := Default()

		e := &entry{
			key:       key,
			data:      valueInBytes,
			expiresAt: time.Now().Add(defaultTTL),
		}
		element := stash.accessList.PushFront(e)
		stash.entries[key] = element
		stash.currentMemory += len(valueInBytes)

		data, err := stash.Get(key)
		require.NoError(t, err)

		var parsed dummy
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Equal(t, value.ID, parsed.ID)
	})

	t.Run("returns data from a valid key and updates expiration time on accessList and store", func(t *testing.T) {
		stash := Default()

		e := &entry{
			key:       key,
			data:      valueInBytes,
			expiresAt: time.Now().Add(defaultTTL),
		}

		entryCopy := *e

		el := stash.accessList.PushFront(e)
		stash.entries[key] = el
		stash.currentMemory += len(valueInBytes)

		data, err := stash.Get(key)
		require.NoError(t, err)

		var parsed dummy
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Equal(t, value.ID, parsed.ID)

		mostRecentEl := stash.accessList.Front()
		en := stash.getElementEntry(mostRecentEl)
		assert.True(t, en.expiresAt.After(entryCopy.expiresAt))
	})

	t.Run("returns error if key is expired", func(t *testing.T) {
		stash := Default()

		e := &entry{
			key:       key,
			data:      valueInBytes,
			expiresAt: time.Now().Add(-time.Minute),
		}
		element := stash.accessList.PushFront(e)
		stash.entries[key] = element
		stash.currentMemory += len(valueInBytes)

		data, err := stash.Get(key)
		require.ErrorIs(t, err, ErrExpired)
		assert.Nil(t, data)
	})

	t.Run("returns error if key does not exist", func(t *testing.T) {
		stash := Default()

		data, err := stash.Get(key)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrNotFound)
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

		element, exists := s.entries[key]
		require.True(t, exists)
		stored, ok := element.Value.(*entry)
		assert.True(t, ok)
		assert.Equal(t, valueInBytes, stored.data)
		assert.True(t, stored.expiresAt.After(time.Now()))
	})

	t.Run("updates existing entry with different value", func(t *testing.T) {
		s := Default()
		oldValue := dummy{ID: "old-id"}
		oldBytes, _ := json.Marshal(oldValue)

		e := &entry{
			key:       key,
			data:      oldBytes,
			expiresAt: time.Now().Add(defaultTTL),
		}
		element := s.accessList.PushFront(e)
		s.entries[key] = element
		s.currentMemory += len(oldBytes)

		err := s.Set(key, valueInBytes)
		require.NoError(t, err)

		updatedElement, exists := s.entries[key]
		require.True(t, exists)
		updated, ok := updatedElement.Value.(*entry)
		assert.True(t, ok)
		assert.Equal(t, valueInBytes, updated.data)
		assert.True(t, updated.expiresAt.After(time.Now()))
	})

	t.Run("resets expiration if value is equal", func(t *testing.T) {
		s := Default()

		oldExpiration := time.Now().Add(-time.Minute)
		e := &entry{
			key:       key,
			data:      valueInBytes,
			expiresAt: oldExpiration,
		}
		element := s.accessList.PushFront(e)
		s.entries[key] = element
		s.currentMemory += len(valueInBytes)

		err := s.Set(key, valueInBytes)
		require.NoError(t, err)

		updatedElement := s.entries[key]
		updated, ok := updatedElement.Value.(*entry)
		assert.True(t, ok)
		assert.Equal(t, valueInBytes, updated.data)
		assert.True(t, updated.expiresAt.After(oldExpiration))
	})

	t.Run("returns error if entry exceeds memory limit", func(t *testing.T) {
		data := []byte("123")
		maxMemory := len(data)

		s := Default(WithMemoryLimit(maxMemory), WithEntryLimit(5))
		err := s.Set("k1", data)
		require.NoError(t, err)

		err = s.Set("k1", []byte("123456"))
		require.ErrorIs(t, err, ErrInsufficientStorageSize)
	})

	t.Run("ensures space needed when memory limit is reached", func(t *testing.T) {
		data := []byte("data")
		dataSize := len(data)

		s := Default(WithMemoryLimit(dataSize), WithEntryLimit(10))
		err := s.Set("k1", data)
		require.NoError(t, err)

		newKey := "k2"
		err = s.Set(newKey, data)
		require.NoError(t, err)

		e := s.getElementEntry(s.accessList.Front())
		assert.Equal(t, e.key, newKey)
	})
}
