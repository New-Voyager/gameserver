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

func NewGameManager() *Manager {
	var gamePersist = NewMemoryGameStateTracker()
	var handPersist = NewMemoryHandStateTracker()

	return &Manager{
		gameStatePersist: gamePersist,
		handStatePersist: handPersist,
		activeGames:      make(map[string]*Game),
		gameCount:        0,
	}
}

func (gm *Manager) InitializeGame(clubID uint32, gameType GameType,
	title string, minPlayers int, autoStart bool, autoDeal bool) uint32 {
	gm.gameCount++
	gameID := fmt.Sprintf("%d:%d", clubID, gm.gameCount)
	game := NewPokerGame(gm, gameID,
		GameType_HOLDEM,
		clubID, gm.gameCount,
		minPlayers,
		autoStart,
		autoDeal,
		gm.gameStatePersist,
		gm.handStatePersist)
	gm.activeGames[gameID] = game

	go game.runGame()
	return gm.gameCount
}

func (gm *Manager) gameEnded(game *Game) {
	delete(gm.activeGames, game.gameID)
}
