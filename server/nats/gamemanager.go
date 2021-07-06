package nats

import (
	"fmt"

	natsgo "github.com/nats-io/nats.go"

	"voyager.com/server/game"
	"voyager.com/server/util"
)

var NatsURL string

// This game manager is similar to game.GameManager.
// However, this game manager active NatsGame objects.
// This will cleanup a NatsGame object and removes when the game ends.
type GameManager struct {
	activeGames  map[string]*NatsGame
	gameCodes    map[string]string
	gameCodeToId map[string]string
	nc           *natsgo.Conn
}

const (
	GAMESTATUS_UNKNOWN    = 0
	GAMESTATUS_CONFIGURED = 1
	GAMESTATUS_ACTIVE     = 2
	GAMESTATUS_PAUSED     = 3
	GAMESTATUS_ENDED      = 4
)

func NewGameManager(nc *natsgo.Conn) (*GameManager, error) {
	NatsURL = util.GameServerEnvironment.GetNatsURL()
	// let us try to connect to nats server
	nc, err := natsgo.Connect(NatsURL)
	if err != nil {
		natsLogger.Error().Msg(fmt.Sprintf("Failed to connect to nats server: %v", err))
		return nil, err
	}

	return &GameManager{
		nc:           nc,
		activeGames:  make(map[string]*NatsGame),
		gameCodes:    make(map[string]string),
		gameCodeToId: make(map[string]string),
	}, nil
}

func (gm *GameManager) NewGame(clubID uint32, gameID uint64, config *game.GameConfig) (*NatsGame, error) {
	natsLogger.Info().Msgf("New game club %d game %d code %s", clubID, gameID, config.GameCode)
	gameIDStr := fmt.Sprintf("%d", gameID)
	game, err := newNatsGame(gm.nc, clubID, gameID, config)
	if err != nil {
		return nil, err
	}
	gm.activeGames[gameIDStr] = game
	gm.gameCodes[gameIDStr] = config.GameCode
	gm.gameCodeToId[config.GameCode] = gameIDStr
	return game, nil
}

func (gm *GameManager) EndNatsGame(clubID uint32, gameID uint64) {
	gameIDStr := fmt.Sprintf("%d", gameID)
	if game, ok := gm.activeGames[gameIDStr]; ok {
		game.cleanup()
		delete(gm.activeGames, gameIDStr)
		delete(gm.gameCodes, gameIDStr)
	}
}

func (gm *GameManager) GameStatusChanged(gameID uint64, newStatus GameStatus) {
	gameIDStr := fmt.Sprintf("%d", gameID)
	if game, ok := gm.activeGames[gameIDStr]; ok {
		// if game ended, remove natsgame and game
		if newStatus.GameStatus == GAMESTATUS_ENDED {
			delete(gm.activeGames, gameIDStr)
			delete(gm.gameCodes, gameIDStr)
			game.gameEnded()
		} else {
			game.gameStatusChanged(gameID, newStatus)
		}
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

func (gm *GameManager) PendingUpdatesDone(gameID uint64, gameStatusInt uint64, tableStatusInt uint64) {
	gameIDStr := fmt.Sprintf("%d", gameID)
	if g, ok := gm.activeGames[gameIDStr]; ok {
		gameStatus := game.GameStatus(gameStatusInt)
		tableStatus := game.TableStatus(tableStatusInt)

		g.pendingUpdatesDone(gameStatus, tableStatus)
	} else {
		natsLogger.Error().Uint64("gameId", gameID).Msg(fmt.Sprintf("GameID: %d does not exist", gameID))
	}
}

func (gm *GameManager) SetupHand(handSetup HandSetup) {
	// first check whether the game is hosted by this game server
	gameIDStr := fmt.Sprintf("%d", handSetup.GameId)
	if handSetup.GameId == 0 {
		gameIDStr = gm.gameCodeToId[handSetup.GameCode]
	}
	var natsGame *NatsGame
	var ok bool
	if natsGame, ok = gm.activeGames[gameIDStr]; !ok {
		// lookup using game code
		gameIDStr, ok = gm.gameCodes[handSetup.GameCode]
		if !ok {
			natsLogger.Error().Str("gameId", handSetup.GameCode).Msg(fmt.Sprintf("Game code: %s does not exist. Aborting setup-deck.", handSetup.GameCode))
			return
		}
	}

	// send the message to the game to setup next hand
	natsGame.setupHand(handSetup)
}

func (gm *GameManager) GetCurrentHandLog(gameID uint64, gameCode string) *map[string]interface{} {
	// first check whether the game is hosted by this game server
	gameIDStr := fmt.Sprintf("%d", gameID)
	if gameID == 0 {
		gameIDStr = gm.gameCodeToId[gameCode]
	}
	var natsGame *NatsGame
	var ok bool
	if natsGame, ok = gm.activeGames[gameIDStr]; !ok {
		// lookup using game code
		gameIDStr, ok = gm.gameCodes[gameCode]
		if !ok {
			var errors map[string]interface{}
			errors["errors"] = fmt.Sprintf("Cannot find game %d", gameID)
			return &errors
		}
		natsGame = gm.activeGames[gameIDStr]
	}
	handLog := natsGame.getHandLog()
	return handLog
}

// TableUpdate used for sending table updates to the players
func (gm *GameManager) TableUpdate(gameID uint64, tableUpdate *TableUpdate) {
	gameIDStr := fmt.Sprintf("%d", gameID)
	if game, ok := gm.activeGames[gameIDStr]; ok {
		game.tableUpdate(gameID, tableUpdate)
	} else {
		natsLogger.Error().Uint64("gameId", gameID).Msg(fmt.Sprintf("GameID: %d does not exist", gameID))
	}
}

// PlayerConfigUpdate used for sending player config updates (muckLosingHand, runItTwicePrompt) to the game
func (gm *GameManager) PlayerConfigUpdate(gameID uint64, playerConfigUpdate *PlayerConfigUpdate) {
	gameIDStr := fmt.Sprintf("%d", gameID)
	if game, ok := gm.activeGames[gameIDStr]; ok {
		game.playerConfigUpdate(playerConfigUpdate)
	} else {
		natsLogger.Error().Uint64("gameId", gameID).Msg(fmt.Sprintf("GameID: %d does not exist", gameID))
	}
}
