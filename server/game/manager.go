package game

import (
	"fmt"

	"github.com/jmoiron/sqlx"
)

type Manager struct {
	isScriptTest     bool
	apiServerURL     string
	delays           Delays
	handStatePersist PersistHandState
	handSetupPersist *RedisHandsSetupTracker
	activeGames      map[string]*Game
	crashdb          *sqlx.DB
	userdb           *sqlx.DB
}

func NewGameManager(isScriptTest bool, apiServerURL string, handPersist PersistHandState, handSetupPersist *RedisHandsSetupTracker, userdb *sqlx.DB, crashdb *sqlx.DB, delays Delays) (*Manager, error) {
	return &Manager{
		isScriptTest:     isScriptTest,
		apiServerURL:     apiServerURL,
		delays:           delays,
		handStatePersist: handPersist,
		handSetupPersist: handSetupPersist,
		activeGames:      make(map[string]*Game),
		userdb:           userdb,
		crashdb:          crashdb,
	}, nil
}

func (gm *Manager) InitializeGame(messageSender MessageSender, config *GameConfig) (*Game, uint64, error) {
	gameIDStr := fmt.Sprintf("%d", config.GameId)
	game, err := NewPokerGame(
		gm.isScriptTest,
		gm,
		&messageSender,
		config,
		gm.delays,
		gm.handStatePersist,
		gm.handSetupPersist,
		gm.apiServerURL,
		gm.crashdb,
		gm.userdb)
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
