package game

import (
	"fmt"
)

type Manager struct {
	gameCount        uint64
	apiServerUrl     string
	gameStatePersist PersistGameState
	handStatePersist PersistHandState
	activeGames      map[string]*Game
}

func NewGameManager(apiServerUrl string, gamePersist PersistGameState, handPersist PersistHandState) *Manager {
	return &Manager{
		apiServerUrl:     apiServerUrl,
		gameStatePersist: gamePersist,
		handStatePersist: handPersist,
		activeGames:      make(map[string]*Game),
		gameCount:        0,
	}
}

func (gm *Manager) InitializeGame(messageReceiver GameMessageReceiver,
	clubID uint32, gameID uint64, gameType GameType, gameCode string,
	title string, minPlayers int, autoStart bool, autoDeal bool, actionTime uint32) (*Game, uint64) {
	if gameID == 0 {
		gm.gameCount++
		gameID = gm.gameCount
	}
	gameIDStr := fmt.Sprintf("%d", gameID)
	game := NewPokerGame(gm,
		&messageReceiver,
		gameCode,
		GameType_HOLDEM,
		clubID, gameID,
		minPlayers,
		autoStart,
		autoDeal,
		actionTime,
		gm.gameStatePersist,
		gm.handStatePersist,
		gm.apiServerUrl)
	gm.activeGames[gameIDStr] = game

	go game.runGame()
	return game, gameID
}

func (gm *Manager) gameEnded(game *Game) {
	gameIDStr := fmt.Sprintf("%d", game.gameID)
	delete(gm.activeGames, gameIDStr)
}
