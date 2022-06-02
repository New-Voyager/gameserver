package game

import (
	"fmt"

	"github.com/pkg/errors"
	"voyager.com/server/internal/encryptionkey"
)

type Manager struct {
	isScriptTest       bool
	apiServerURL       string
	delays             Delays
	handStatePersist   PersistHandState
	handSetupPersist   *RedisHandsSetupTracker
	activeGames        map[string]*Game
	crashHandler       func(uint64, string)
	encryptionKeyCache *encryptionkey.Cache
}

func NewGameManager(isScriptTest bool, apiServerURL string, handPersist PersistHandState, handSetupPersist *RedisHandsSetupTracker, delays Delays) (*Manager, error) {

	cache, err := encryptionkey.NewCache(100000, apiServerURL)
	if err != nil || cache == nil {
		return nil, errors.Wrap(err, "Unable to instantiate encryption key cache")
	}

	return &Manager{
		isScriptTest:       isScriptTest,
		apiServerURL:       apiServerURL,
		delays:             delays,
		handStatePersist:   handPersist,
		handSetupPersist:   handSetupPersist,
		activeGames:        make(map[string]*Game),
		encryptionKeyCache: cache,
	}, nil
}

func (gm *Manager) InitializeGame(messageSender MessageSender, gameID uint64, gameCode string, tournamentID uint64, tableNo uint32) (*Game, uint64, error) {
	gameIDStr := fmt.Sprintf("%d", gameID)
	game, err := NewPokerGame(
		gameID,
		gameCode,
		tournamentID,
		tableNo,
		gm.isScriptTest,
		gm,
		&messageSender,
		gm.delays,
		gm.handStatePersist,
		gm.handSetupPersist,
		gm.encryptionKeyCache,
		gm.apiServerURL)
	gm.activeGames[gameIDStr] = game

	if err != nil {
		return nil, 0, err
	}
	return game, gameID, nil
}

func (gm *Manager) InitializeTournamentGame(messageSender MessageSender, tournamentID uint64, tableNo uint64, gameID uint64, gameCode string) (*Game, uint64, error) {
	gameIDStr := fmt.Sprintf("%d", gameID)
	game, err := NewPokerGame(
		gameID,
		gameCode,
		tournamentID,
		uint32(tableNo),
		gm.isScriptTest,
		gm,
		&messageSender,
		gm.delays,
		gm.handStatePersist,
		gm.handSetupPersist,
		gm.encryptionKeyCache,
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
		gm.encryptionKeyCache,
		gm.apiServerURL)
	gm.activeGames[gameIDStr] = game

	if err != nil {
		return nil, 0, err
	}
	return game, config.GameId, nil
}

func (gm *Manager) SetCrashHandler(handler func(uint64, string)) {
	gm.crashHandler = handler
}

func (gm *Manager) OnGameCrash(gameID uint64, gameCode string) {
	gm.crashHandler(gameID, gameCode)
}

func (gm *Manager) gameEnded(game *Game) {
	gameIDStr := fmt.Sprintf("%d", game.gameID)
	delete(gm.activeGames, gameIDStr)
}
