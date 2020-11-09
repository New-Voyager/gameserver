package game

import (
	"fmt"

	"voyager.com/server/util"
)

var GameManager *Manager = CreateGameManager()

func CreateGameManager() *Manager {
	var gamePersist PersistGameState
	var handPersist PersistHandState
	var persistMethod = util.GameServerEnvironment.GetPersistMethod()
	if persistMethod == "redis" {
		var redisHost = util.GameServerEnvironment.GetRedisHost()
		var redisPort = util.GameServerEnvironment.GetRedisPort()
		var redisPW = util.GameServerEnvironment.GetRedisPW()
		var redisDB = util.GameServerEnvironment.GetRedisDB()
		gamePersist = NewRedisGameStateTracker(fmt.Sprintf("%s:%d", redisHost, redisPort), redisPW, redisDB)
		handPersist = NewRedisHandStateTracker(fmt.Sprintf("%s:%d", redisHost, redisPort), redisPW, redisDB)
	} else {
		gamePersist = NewMemoryGameStateTracker()
		handPersist = NewMemoryHandStateTracker()
	}
	return NewGameManager(gamePersist, handPersist)
}