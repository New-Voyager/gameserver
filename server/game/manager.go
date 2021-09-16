package game

import (
	"fmt"
)

type Manager struct {
	isScriptTest     bool
	apiServerURL     string
	delays           Delays
	handStatePersist PersistHandState
	handSetupPersist *RedisHandsSetupTracker
	activeGames      map[string]*Game
	crashHandler     func(uint64)
}

func NewGameManager(isScriptTest bool, apiServerURL string, handPersist PersistHandState, handSetupPersist *RedisHandsSetupTracker, delays Delays) (*Manager, error) {
	return &Manager{
		isScriptTest:     isScriptTest,
		apiServerURL:     apiServerURL,
		delays:           delays,
		handStatePersist: handPersist,
		handSetupPersist: handSetupPersist,
		activeGames:      make(map[string]*Game),
	}, nil
}

func (gm *Manager) InitializeGame(messageSender MessageSender, gameID uint64, gameCode string) (*Game, uint64, error) {
	gameIDStr := fmt.Sprintf("%d", gameID)
	game, err := NewPokerGame(
		gameID,
		gameCode,
		gm.isScriptTest,
		gm,
		&messageSender,
		gm.delays,
		gm.handStatePersist,
		gm.handSetupPersist,
		gm.apiServerURL)
	gm.activeGames[gameIDStr] = game

	if err != nil {
		return nil, 0, err
	}
	return game, gameID, nil
}

func (gm *Manager) InitializeTestGame(messageSender MessageSender, gameID uint64, gameCode string, config *TestGameConfig) (*Game, uint64, error) {
	gameIDStr := fmt.Sprintf("%d", config.GameId)
	game, err := NewTestPokerGame(
		gameID,
		gameCode,
		gm.isScriptTest,
		gm,
		&messageSender,
		config,
		gm.delays,
		gm.handStatePersist,
		gm.handSetupPersist,
		gm.apiServerURL)
	gm.activeGames[gameIDStr] = game

	if err != nil {
		return nil, 0, err
	}
	return game, config.GameId, nil
}

func (gm *Manager) SetCrashHandler(handler func(uint64)) {
	gm.crashHandler = handler
}

func (gm *Manager) OnGameCrash(gameID uint64) {
	gm.crashHandler(gameID)
}

func (gm *Manager) gameEnded(game *Game) {
	gameIDStr := fmt.Sprintf("%d", game.gameID)
	delete(gm.activeGames, gameIDStr)
}
