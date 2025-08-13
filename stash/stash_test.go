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
		assert.NotNil(t, s.mu)
	})

	t.Run("returns Stash with a custom ttl duration", func(t *testing.T) {
		ttl := 10 * time.Minute

		s := New(WithTTL(
			ttl,
		))

		assert.Equal(t, ttl, s.ttl)
		assert.Empty(t, s.store)
		assert.NotNil(t, s.mu)
	})
}

func Test_Stash_Get(t *testing.T) {
	key := "dummy-key"

	value := dummy{
		ID: "id",
	}

	valueInBytes, err := json.Marshal(value)
	require.NoError(t, err)

	t.Run("returns data from a valid key", func(t *testing.T) {
		stash := New()

		key := "dummy-key"

		entry := entry{
			data:      valueInBytes,
			expiresAt: time.Now().Add(defaultTTL),
		}

		stash.store[key] = entry

		retrievedValue, err := stash.Get(key)
		require.NoError(t, err)

		var parsedValue dummy
		err = json.Unmarshal(retrievedValue, &parsedValue)
		require.NoError(t, err)
		assert.Equal(t, value.ID, parsedValue.ID)
	})

	t.Run("returns an ErrExpired error if key is expired", func(t *testing.T) {
		stash := New()

		entry := entry{
			data:      valueInBytes,
			expiresAt: time.Now().Add(-defaultTTL),
		}

		stash.store[key] = entry

		retrievedValue, err := stash.Get(key)
		require.Nil(t, retrievedValue)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrExpired)
	})

	t.Run("returns an ErrNotFound error if key does not exists", func(t *testing.T) {
		stash := New()

		retrievedValue, err := stash.Get(key)
		require.Nil(t, retrievedValue)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNotFound)
	})
}

func Test_Stash_Set(t *testing.T) {
	key := "dummy-key"

	value := dummy{
		ID: "id",
	}

	valueInBytes, err := json.Marshal(value)
	require.NoError(t, err)

	t.Run("updates already existent entry", func(t *testing.T) {
		s := New()

		oldValue := dummy{ID: "old-id"}
		oldValueInBytes, err := json.Marshal(oldValue)
		require.NoError(t, err)

		entry := entry{
			data:      oldValueInBytes,
			expiresAt: time.Now().Add(defaultTTL),
		}

		s.store[key] = entry

		err = s.Set(key, valueInBytes)
		require.NoError(t, err)

		updated, ok := s.store[key]
		require.True(t, ok)
		assert.True(t, updated.expiresAt.After(time.Now()))
		assert.Equal(t, oldValueInBytes, updated.data)
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
}
