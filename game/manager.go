package game

import (
	"fmt"

	"voyager.com/server/util"
)

type Manager struct {
	gameCount        uint32
	gameStatePersist PersistGameState
	handStatePersist PersistHandState
	activeGames      map[string]*Game
}

func NewGameManager() *Manager {
	var redisHost = util.GameServerEnvironment.GetRedisHost()
	var redisPort = util.GameServerEnvironment.GetRedisPort()
	var redisPW = util.GameServerEnvironment.GetRedisPW()
	var redisDB = util.GameServerEnvironment.GetRedisDB()
	var gamePersist = NewRedisGameStateTracker(fmt.Sprintf("%s:%d", redisHost, redisPort), redisPW, redisDB)
	var handPersist = NewRedisHandStateTracker(fmt.Sprintf("%s:%d", redisHost, redisPort), redisPW, redisDB)

	return &Manager{
		gameStatePersist: gamePersist,
		handStatePersist: handPersist,
		activeGames:      make(map[string]*Game),
		gameCount:        0,
	}
}

func (gm *Manager) InitializeGame(messageReceiver GameMessageReceiver,
	clubID uint32, gameNum uint32, gameType GameType,
	title string, minPlayers int, autoStart bool, autoDeal bool) (*Game, uint32) {
	if gameNum == 0 {
		gm.gameCount++
		gameNum = gm.gameCount
	}
	gameID := fmt.Sprintf("%d:%d", clubID, gameNum)
	game := NewPokerGame(gm,
		&messageReceiver,
		gameID,
		GameType_HOLDEM,
		clubID, gameNum,
		minPlayers,
		autoStart,
		autoDeal,
		gm.gameStatePersist,
		gm.handStatePersist)
	gm.activeGames[gameID] = game

	go game.runGame()
	return game, gameNum
}

func (gm *Manager) gameEnded(game *Game) {
	delete(gm.activeGames, game.gameID)
}
