package game

import (
	"fmt"
)

type Manager struct {
	gameCount        uint64
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
	clubID uint32, gameID uint64, gameType GameType,
	title string, minPlayers int, autoStart bool, autoDeal bool, actionTime uint32) (*Game, uint64) {
	if gameID == 0 {
		gm.gameCount++
		gameID = gm.gameCount
	}
	gameIDStr := fmt.Sprintf("%d", gameID)
	game := NewPokerGame(gm,
		&messageReceiver,
		gameIDStr,
		GameType_HOLDEM,
		clubID, gameID,
		minPlayers,
		autoStart,
		autoDeal,
		actionTime,
		gm.gameStatePersist,
		gm.handStatePersist)
	gm.activeGames[gameIDStr] = game

	go game.runGame()
	return game, gameID
}

func (gm *Manager) gameEnded(game *Game) {
	gameIDStr := fmt.Sprintf("%d", game.gameID)
	delete(gm.activeGames, gameIDStr)
}
