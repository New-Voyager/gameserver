package internal

import (
	"fmt"

	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
)

type GameCodeCacheStruct struct {
	gameIDToCode *lru.Cache
	gameCodeToID *lru.Cache
}

var GameCodeCache = createCache()

func createCache() *GameCodeCacheStruct {
	c, err := NewCache()
	if err != nil {
		panic("Cannot initialize game code cache")
	}
	return c
}

func NewCache() (*GameCodeCacheStruct, error) {
	size := 100000
	gameIDToCode, err := lru.New(size)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to initialize gameIDToCode cache")
	}
	gameCodeToID, err := lru.New(size)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to initialize gameCodeToID cache")
	}
	return &GameCodeCacheStruct{
		gameIDToCode: gameIDToCode,
		gameCodeToID: gameCodeToID,
	}, nil
}

func (c *GameCodeCacheStruct) Add(gameID uint64, gameCode string) error {
	if gameID == 0 {
		return fmt.Errorf("Invalid game ID [%d]", gameID)
	} else if gameCode == "" {
		return fmt.Errorf("Invalid game Code [%s]", gameCode)
	}

	c.gameIDToCode.Add(gameID, gameCode)
	c.gameCodeToID.Add(gameCode, gameID)
	return nil
}

func (c *GameCodeCacheStruct) GameIDToCode(gameID uint64) (string, bool) {
	v, exists := c.gameIDToCode.Get(gameID)
	if !exists {
		return "", false
	}
	return v.(string), true
}

func (c *GameCodeCacheStruct) GameCodeToID(gameCode string) (uint64, bool) {
	v, exists := c.gameCodeToID.Get(gameCode)
	if !exists {
		return 0, false
	}
	return v.(uint64), true
}
