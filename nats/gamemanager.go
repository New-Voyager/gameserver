package nats

import (
	"fmt"

	natsgo "github.com/nats-io/nats.go"

	"voyager.com/server/game"
	"voyager.com/server/util"
)

var NatsURL = util.GameServerEnvironment.GetNatsClientConnURL()

// This game manager is similar to game.GameManager.
// However, this game manager active NatsGame objects.
// This will cleanup a NatsGame object and removes when the game ends.
type GameManager struct {
	activeGames map[string]*NatsGame
	nc          *natsgo.Conn
}

func NewGameManager(nc *natsgo.Conn) (*GameManager, error) {
	// let us try to connect to nats server
	nc, err := natsgo.Connect(NatsURL)
	if err != nil {
		natsLogger.Error().Msg(fmt.Sprintf("Failed to connect to nats server: %v", err))
		return nil, err
	}

	return &GameManager{
		nc:          nc,
		activeGames: make(map[string]*NatsGame),
	}, nil
}

func (gm *GameManager) NewGame(clubID uint32, gameID uint64, config *game.GameConfig) (*NatsGame, error) {
	gameIDStr := fmt.Sprintf("%d", gameID)
	game, err := newNatsGame(gm.nc, clubID, gameID, config)
	if err != nil {
		return nil, err
	}
	gm.activeGames[gameIDStr] = game
	return game, nil
}

func (gm *GameManager) EndNatsGame(clubID uint32, gameID uint64) {
	gameIDStr := fmt.Sprintf("%d", gameID)
	if game, ok := gm.activeGames[gameIDStr]; ok {
		game.cleanup()
		delete(gm.activeGames, gameIDStr)
	}
}

func (gm *GameManager) GameStatusChanged(gameID uint64, newStatus game.GameStatus) {
	gameIDStr := fmt.Sprintf("%d", gameID)
	if game, ok := gm.activeGames[gameIDStr]; ok {
		game.gameStatusChanged(gameID, newStatus)
	} else {
		natsLogger.Error().Uint64("gameId", gameID).Msg(fmt.Sprintf("GameID: %d does not exist", gameID))
	}
}

func (gm *GameManager) PlayerUpdate(gameID uint64, playerUpdate *PlayerUpdate) {
	gameIDStr := fmt.Sprintf("%d", gameID)
	if game, ok := gm.activeGames[gameIDStr]; ok {
		game.playerUpdate(gameID, playerUpdate)
	} else {
		natsLogger.Error().Uint64("gameId", gameID).Msg(fmt.Sprintf("GameID: %d does not exist", gameID))
	}
}
