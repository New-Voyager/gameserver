package scheduler

import (
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

func (c *Cache) Exists(gameID uint64) bool {
	_, exists := c.cache.Get(gameID)
	return exists
}

func (c *Cache) Add(gameID uint64) {
	// Value doesn't matter. We only care about a set of keys (game IDs).
	c.cache.Add(gameID, nil)
}
