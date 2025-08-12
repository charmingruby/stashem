package cache

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

func Test_Cache_New(t *testing.T) {
	t.Run("returns Cache with default values", func(t *testing.T) {
		c := New()

		assert.Equal(t, defaultTTL, c.ttl)
		assert.Empty(t, c.store)
		assert.NotNil(t, c.mu)
	})

	t.Run("returns Cache with a custom ttl duration", func(t *testing.T) {
		ttl := 10 * time.Minute

		c := New(WithTTL(
			ttl,
		))

		assert.Equal(t, ttl, c.ttl)
		assert.Empty(t, c.store)
		assert.NotNil(t, c.mu)
	})
}

func Test_Cache_Get(t *testing.T) {
	key := "dummy-key"

	value := dummy{
		ID: "id",
	}

	valueInBytes, err := json.Marshal(value)
	require.NoError(t, err)

	t.Run("returns data from a valid key", func(t *testing.T) {
		cache := New()

		key := "dummy-key"

		entry := entry{
			data:      valueInBytes,
			expiresAt: time.Now().Add(defaultTTL),
		}

		cache.store[key] = entry

		retrievedValue, err := cache.Get(key)
		require.NoError(t, err)

		var parsedValue dummy
		err = json.Unmarshal(retrievedValue, &parsedValue)
		require.NoError(t, err)
		assert.Equal(t, value.ID, parsedValue.ID)
	})

	t.Run("returns an ErrExpired error if key is expired", func(t *testing.T) {
		cache := New()

		entry := entry{
			data:      valueInBytes,
			expiresAt: time.Now().Add(-defaultTTL),
		}

		cache.store[key] = entry

		retrievedValue, err := cache.Get(key)
		require.Nil(t, retrievedValue)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrExpired)
	})

	t.Run("returns an ErrNotFound error if key does not exists", func(t *testing.T) {
		cache := New()

		retrievedValue, err := cache.Get(key)
		require.Nil(t, retrievedValue)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNotFound)
	})
}

// func Test_Cache_Set(t *testing.T) {
// 	key := "dummy-key"

// 	value := dummy{
// 		ID: "id",
// 	}

// 	valueInBytes, err := json.Marshal(value)
// 	require.NoError(t, err)

// 	t.Run("", func(t *testing.T) {})

// 	t.Run("", func(t *testing.T) {})
// }
