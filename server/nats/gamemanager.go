package nats

import (
	"fmt"

	natsgo "github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"

	"voyager.com/server/game"
	"voyager.com/server/util"
)

var NatsURL string
var natsGMLogger = log.With().Str("logger_name", "nats::gamemanager").Logger()

// This game manager is similar to game.GameManager.
// However, this game manager active NatsGame objects.
// This will cleanup a NatsGame object and removes when the game ends.
type GameManager struct {
	activeGames  map[string]*NatsGame
	gameIDToCode map[string]string
	gameCodeToID map[string]string
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
	NatsURL = util.Env.GetNatsURL()
	// let us try to connect to nats server
	nc, err := natsgo.Connect(NatsURL)
	if err != nil {
		natsGMLogger.Error().Msgf("Failed to connect to nats server: %v", err)
		return nil, err
	}

	return &GameManager{
		nc:           nc,
		activeGames:  make(map[string]*NatsGame),
		gameIDToCode: make(map[string]string),
		gameCodeToID: make(map[string]string),
	}, nil
}

func (gm *GameManager) NewGame(clubID uint32, gameID uint64, config *game.GameConfig) (*NatsGame, error) {
	natsGMLogger.Info().Msgf("New game club %d game %d code %s", clubID, gameID, config.GameCode)
	gameIDStr := fmt.Sprintf("%d", gameID)
	game, err := newNatsGame(gm.nc, clubID, gameID, config)
	if err != nil {
		return nil, err
	}
	gm.activeGames[gameIDStr] = game
	gm.gameIDToCode[gameIDStr] = config.GameCode
	gm.gameCodeToID[config.GameCode] = gameIDStr
	return game, nil
}

func (gm *GameManager) CrashCleanup(gameID uint64) {
	natsGMLogger.Error().Msgf("CrashCleanup called for game ID %d", gameID)
	gm.EndNatsGame(gameID)
}

func (gm *GameManager) EndNatsGame(gameID uint64) {
	gameIDStr := fmt.Sprintf("%d", gameID)
	if game, exists := gm.activeGames[gameIDStr]; exists {
		game.gameEnded()
		game.cleanup()
		delete(gm.activeGames, gameIDStr)
		if gameCode, exists := gm.gameIDToCode[gameIDStr]; exists {
			delete(gm.gameCodeToID, gameCode)
		}
		delete(gm.gameIDToCode, gameIDStr)
	}
}

func (gm *GameManager) ResumeGame(gameID uint64) {
	gameIDStr := fmt.Sprintf("%d", gameID)
	if g, ok := gm.activeGames[gameIDStr]; ok {
		g.resumeGame()
	} else {
		natsGMLogger.Error().Uint64("gameId", gameID).Msgf("GameID: %d does not exist", gameID)
	}
}

func (gm *GameManager) SetupHand(handSetup HandSetup) {
	// first check whether the game is hosted by this game server
	gameIDStr := fmt.Sprintf("%d", handSetup.GameId)
	if handSetup.GameId == 0 {
		gameIDStr = gm.gameCodeToID[handSetup.GameCode]
	}
	var natsGame *NatsGame
	var ok bool
	if natsGame, ok = gm.activeGames[gameIDStr]; !ok {
		natsGMLogger.Error().Str("gameId", handSetup.GameCode).Msgf("Game code: %s does not exist. Aborting setup-deck.", handSetup.GameCode)
		return
	}

	// send the message to the game to setup next hand
	natsGame.setupHand(handSetup)
}

func (gm *GameManager) GetCurrentHandLog(gameID uint64) *map[string]interface{} {
	// first check whether the game is hosted by this game server
	gameIDStr := fmt.Sprintf("%d", gameID)
	var natsGame *NatsGame
	var ok bool
	if natsGame, ok = gm.activeGames[gameIDStr]; !ok {
		// lookup using game code
		var errors map[string]interface{}
		errors["errors"] = fmt.Sprintf("Cannot find game %d", gameID)
		return &errors
	}
	handLog := natsGame.getHandLog()
	return handLog
}
