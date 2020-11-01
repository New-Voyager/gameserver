package game

import (
	"fmt"
)

type Manager struct {
	gameCount        uint32
	gameStatePersist PersistGameState
	handStatePersist PersistHandState
	activeGames      map[string]*Game
}

func NewGameManager(gamePersist PersistGameState, handPersist PersistHandState) *Manager {
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
