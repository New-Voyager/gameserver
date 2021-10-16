package caches

import (
	cc "voyager.com/caching"
)

var GameCodeCache = createCache()

func createCache() *cc.GameCodeCache {
	c, err := cc.NewCache()
	if err != nil {
		panic("Cannot initialize game code cache")
	}
	return c
}
