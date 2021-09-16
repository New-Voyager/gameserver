package encryptionkey

import (
	"fmt"

	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
)

type Cache struct {
	cache *lru.Cache
}

func NewCache(size int) (*Cache, error) {
	c, err := lru.New(size)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to initialize cache")
	}
	return &Cache{
		cache: c,
	}, nil
}

func (c *Cache) Get(playerID uint64) (string, error) {
	v, exists := c.cache.Get(playerID)
	if !exists {
		return "", fmt.Errorf("Encryption key for player %d is not in cache", playerID)
	}
	return v.(string), nil
}

func (c *Cache) Add(playerID uint64, encryptionKey string) {
	c.cache.Add(playerID, encryptionKey)
}
