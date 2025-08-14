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

func Test_Stash_New(t *testing.T) {
	t.Run("returns Stash with default values", func(t *testing.T) {
		s := New()
		assert.Equal(t, defaultTTL, s.ttl)
		assert.Empty(t, s.store)
	})

	t.Run("returns Stash with a custom ttl duration", func(t *testing.T) {
		ttl := 10 * time.Minute
		s := New(WithTTL(ttl))
		assert.Equal(t, ttl, s.ttl)
		assert.Empty(t, s.store)
	})
}

func Test_Stash_Get(t *testing.T) {
	key := "dummy-key"
	value := dummy{ID: "id"}
	valueInBytes, err := json.Marshal(value)
	require.NoError(t, err)

	t.Run("returns data from a valid key", func(t *testing.T) {
		stash := New()
		stash.store[key] = entry{
			data:      valueInBytes,
			expiresAt: time.Now().Add(defaultTTL),
		}

		retrievedValue, err := stash.Get(key)
		require.NoError(t, err)

		var parsedValue dummy
		err = json.Unmarshal(retrievedValue, &parsedValue)
		require.NoError(t, err)
		assert.Equal(t, value.ID, parsedValue.ID)
	})

	t.Run("returns an ErrExpired error if key is expired", func(t *testing.T) {
		stash := New()
		stash.store[key] = entry{
			data:      valueInBytes,
			expiresAt: time.Now().Add(-defaultTTL),
		}

		retrievedValue, err := stash.Get(key)
		require.Nil(t, retrievedValue)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrExpired)
	})

	t.Run("returns an ErrNotFound error if key does not exist", func(t *testing.T) {
		stash := New()
		retrievedValue, err := stash.Get(key)
		require.Nil(t, retrievedValue)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNotFound)
	})
}

func Test_Stash_Set(t *testing.T) {
	key := "dummy-key"
	value := dummy{ID: "id"}
	valueInBytes, err := json.Marshal(value)
	require.NoError(t, err)

	t.Run("updates already existent entry with different value", func(t *testing.T) {
		s := New()
		oldValue := dummy{ID: "old-id"}
		oldValueInBytes, err := json.Marshal(oldValue)
		require.NoError(t, err)

		s.store[key] = entry{
			data:      oldValueInBytes,
			expiresAt: time.Now().Add(defaultTTL),
		}

		err = s.Set(key, valueInBytes)
		require.NoError(t, err)

		updated, ok := s.store[key]
		require.True(t, ok)
		assert.Equal(t, valueInBytes, updated.data)
		assert.True(t, updated.expiresAt.After(time.Now()))
	})

	t.Run("renews expiration if value is equal", func(t *testing.T) {
		s := New()
		s.store[key] = entry{
			data:      valueInBytes,
			expiresAt: time.Now().Add(-time.Minute),
		}

		oldExpiration := s.store[key].expiresAt
		err := s.Set(key, valueInBytes)
		require.NoError(t, err)

		updated, ok := s.store[key]
		require.True(t, ok)
		assert.Equal(t, valueInBytes, updated.data)
		assert.True(t, updated.expiresAt.After(oldExpiration))
	})

	t.Run("stores a new entry", func(t *testing.T) {
		s := New()
		err := s.Set(key, valueInBytes)
		require.NoError(t, err)

		stored, ok := s.store[key]
		require.True(t, ok)
		assert.Equal(t, valueInBytes, stored.data)
		assert.True(t, stored.expiresAt.After(time.Now()))
	})

	t.Run("returns error if entry to updates exceeds memory", func(t *testing.T) {
		s := New(WithLimit(StorageLimit{entries: 10, memory: 5}))

		err := s.Set("k1", []byte("123"))
		require.NoError(t, err)

		err = s.Set("k1", []byte("123456"))
		assert.ErrorIs(t, err, ErrInsufficientStorageSize)
	})
}

func Test_Stash_Cleanup(t *testing.T) {
	key := "dummy-key"
	value := dummy{ID: "id"}
	valueInBytes, err := json.Marshal(value)
	require.NoError(t, err)

	s := New()
	s.store[key] = entry{
		data:      valueInBytes,
		expiresAt: time.Now().Add(-time.Minute),
	}

	s.cleanup()
	_, err = s.Get(key)
	assert.ErrorIs(t, err, ErrNotFound)
}

func Test_Stash_HasStorageAvailabilityForUpdate(t *testing.T) {
	t.Run("blocks adding new entry when entry limit reached", func(t *testing.T) {
		s := New(WithLimit(StorageLimit{entries: 2, memory: 1000}))

		s.store["k1"] = entry{data: []byte("1")}
		s.store["k2"] = entry{data: []byte("2")}

		canAdd := s.hasStorageAvailabilityForUpdate(nil, []byte("3"))
		assert.False(t, canAdd)
	})

	t.Run("allows updating existing entry even if limit reached", func(t *testing.T) {
		s := New(WithLimit(StorageLimit{entries: 2, memory: 1000}))

		s.store["k1"] = entry{data: []byte("old")}
		s.store["k2"] = entry{data: []byte("2")}

		canUpdate := s.hasStorageAvailabilityForUpdate([]byte("old"), []byte("new"))
		assert.True(t, canUpdate)
	})
}

func Test_Stash_Set_ErrInsufficientStorageSize(t *testing.T) {
	t.Run("returns error if entry limit exceeded for new key", func(t *testing.T) {
		s := New(WithLimit(StorageLimit{entries: 1, memory: 1000}))

		err := s.Set("k1", []byte("data"))
		require.NoError(t, err)

		err = s.Set("k2", []byte("data2"))
		assert.ErrorIs(t, err, ErrInsufficientStorageSize)
	})

	t.Run("returns error if memory limit exceeded", func(t *testing.T) {
		s := New(WithLimit(StorageLimit{entries: 10, memory: 5}))

		err := s.Set("k1", []byte("123456"))
		assert.ErrorIs(t, err, ErrInsufficientStorageSize)
	})
}

func Test_Stash_WithLimit(t *testing.T) {
	t.Run("applies custom limits", func(t *testing.T) {
		limit := StorageLimit{entries: 5, memory: 500}
		s := New(WithLimit(limit))

		assert.Equal(t, limit, s.limit)
	})
}
