package encryptionkey

import (
	lru "github.com/hashicorp/golang-lru"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

type Cache struct {
	cache *lru.Cache
	db    *sqlx.DB
}

func NewCache(size int, db *sqlx.DB) (*Cache, error) {
	c, err := lru.New(size)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to initialize cache")
	}
	return &Cache{
		cache: c,
		db:    db,
	}, nil
}

func (c *Cache) Get(playerID uint64) (string, error) {
	var err error
	v, exists := c.cache.Get(playerID)
	if !exists {
		v, err = c.fetch(playerID)
		if err != nil {
			return "", errors.Wrapf(err, "Unable to fetch encryption key for player %d from database", playerID)
		}
		c.cache.Add(playerID, v)
	}
	return v.(string), nil
}

func (c *Cache) fetch(playerID uint64) (string, error) {
	var encryptionKey string
	err := c.db.Get(&encryptionKey, "SELECT encryption_key FROM player WHERE id = $1", playerID)
	if err != nil {
		return "", errors.Wrap(err, "sqlx Get returned an error")
	}

	return encryptionKey, nil
}
