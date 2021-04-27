package game

import (
	"fmt"

	"voyager.com/server/util"
)

var GameManager *Manager

func CreateGameManager(apiServerUrl string, delays Delays) *Manager {
	if GameManager != nil {
		return GameManager
	}

	var redisHost = util.GameServerEnvironment.GetRedisHost()
	var redisPort = util.GameServerEnvironment.GetRedisPort()
	var redisPW = util.GameServerEnvironment.GetRedisPW()
	var redisDB = util.GameServerEnvironment.GetRedisDB()
	handSetupPersist := NewRedisHandsSetupTracker(fmt.Sprintf("%s:%d", redisHost, redisPort), redisPW, redisDB)

	var handPersist PersistHandState
	var persistMethod = util.GameServerEnvironment.GetPersistMethod()
	if persistMethod == "redis" {
		handPersist = NewRedisHandStateTracker(fmt.Sprintf("%s:%d", redisHost, redisPort), redisPW, redisDB)
	} else {
		handPersist = NewMemoryHandStateTracker()
	}

	GameManager = NewGameManager(apiServerUrl, handPersist, handSetupPersist, delays)
	return GameManager
}
