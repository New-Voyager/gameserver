package game

import (
	"fmt"

	"github.com/jmoiron/sqlx"
)

type Manager struct {
	isScriptTest     bool
	apiServerUrl     string
	delays           Delays
	handStatePersist PersistHandState
	handSetupPersist *RedisHandsSetupTracker
	activeGames      map[string]*Game
	db               *sqlx.DB
}

func NewGameManager(isScriptTest bool, apiServerUrl string, handPersist PersistHandState, handSetupPersist *RedisHandsSetupTracker, db *sqlx.DB, delays Delays) (*Manager, error) {
	return &Manager{
		isScriptTest:     isScriptTest,
		apiServerUrl:     apiServerUrl,
		delays:           delays,
		handStatePersist: handPersist,
		handSetupPersist: handSetupPersist,
		activeGames:      make(map[string]*Game),
		db:               db,
	}, nil
}

func (gm *Manager) InitializeGame(messageReceiver GameMessageReceiver, config *GameConfig, autoDeal bool) (*Game, uint64, error) {
	gameIDStr := fmt.Sprintf("%d", config.GameId)
	game, err := NewPokerGame(
		gm.isScriptTest,
		gm,
		&messageReceiver,
		config,
		gm.delays,
		autoDeal,
		gm.handStatePersist,
		gm.handSetupPersist,
		gm.apiServerUrl,
		gm.db)
	gm.activeGames[gameIDStr] = game

	if err != nil {
		return nil, 0, err
	}
	return game, config.GameId, nil
}

func (gm *Manager) gameEnded(game *Game) {
	gameIDStr := fmt.Sprintf("%d", game.config.GameId)
	delete(gm.activeGames, gameIDStr)
}
