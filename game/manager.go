package game

import (
	"fmt"
)

type Manager struct {
	apiServerUrl     string
	delays           Delays
	gameStatePersist PersistGameState
	handStatePersist PersistHandState
	activeGames      map[string]*Game
}

func NewGameManager(apiServerUrl string, gamePersist PersistGameState, handPersist PersistHandState, delays Delays) *Manager {
	return &Manager{
		apiServerUrl:     apiServerUrl,
		delays:           delays,
		gameStatePersist: gamePersist,
		handStatePersist: handPersist,
		activeGames:      make(map[string]*Game),
	}
}

func (gm *Manager) InitializeGame(messageReceiver GameMessageReceiver, config *GameConfig, autoDeal bool) (*Game, uint64, error) {
	gameIDStr := fmt.Sprintf("%d", config.GameId)
	game, err := NewPokerGame(gm,
		&messageReceiver,
		config,
		gm.delays,
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
