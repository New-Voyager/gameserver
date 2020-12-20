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

func (gm *Manager) InitializeGame(messageReceiver GameMessageReceiver, config *GameConfig, autoDeal bool) (*Game, uint64, error) {
	if config.GameId == 0 {
		gm.gameCount++
		config.GameId = gm.gameCount
	}
	gameIDStr := fmt.Sprintf("%d", config.GameId)
	game, err := NewPokerGame(gm,
		&messageReceiver,
		config,
		autoDeal,
		gm.gameStatePersist,
		gm.handStatePersist,
		gm.apiServerUrl)
	gm.activeGames[gameIDStr] = game

	if err != nil {
		return nil, 0, err
	}
	go game.runGame()
	return game, config.GameId, nil
}

func (gm *Manager) gameEnded(game *Game) {
	gameIDStr := fmt.Sprintf("%d", game.config.GameId)
	delete(gm.activeGames, gameIDStr)
}
