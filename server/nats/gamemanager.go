package nats

import (
	"fmt"
	"strconv"

	natsgo "github.com/nats-io/nats.go"
	cmap "github.com/orcaman/concurrent-map"
	"github.com/pkg/errors"

	"voyager.com/logging"
	"voyager.com/server/util"
)

var NatsURL string
var natsGMLogger = logging.GetZeroLogger("nats::gamemanager", nil)

// This game manager is similar to game.GameManager.
// However, this game manager active NatsGame objects.
// This will cleanup a NatsGame object and removes when the game ends.
type GameManager struct {
	activeGames  cmap.ConcurrentMap
	gameIDToCode cmap.ConcurrentMap
	gameCodeToID cmap.ConcurrentMap
	nc           *natsgo.Conn
}

type GameListItem struct {
	GameID   uint64 `json:"gameId"`
	GameCode string `json:"gameCode"`
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
		activeGames:  cmap.New(),
		gameIDToCode: cmap.New(),
		gameCodeToID: cmap.New(),
	}, nil
}

func (gm *GameManager) NewGame(gameID uint64, gameCode string) (*NatsGame, error) {
	natsGMLogger.Info().
		Uint64("gameID", gameID).Str("gameCode", gameCode).
		Msgf("New game %d:%s", gameID, gameCode)
	gameIDStr := fmt.Sprintf("%d", gameID)
	game, err := newNatsGame(gm.nc, gameID, gameCode)
	if err != nil {
		return nil, errors.Wrap(err, "Could create new NATS game")
	}
	gm.activeGames.Set(gameIDStr, game)
	gm.gameIDToCode.Set(gameIDStr, gameCode)
	gm.gameCodeToID.Set(gameCode, gameIDStr)
	util.Metrics.SetActiveGamesMapCount(gm.activeGames.Count())
	return game, nil
}

func (gm *GameManager) GetGames() ([]GameListItem, error) {
	games := make([]GameListItem, 0)
	for item := range gm.activeGames.Iter() {
		gameIDStr := item.Key
		game, ok := item.Val.(*NatsGame)
		if !ok {
			msg := fmt.Sprintf("GetGames unable to convert activeGames map value to *NatsGame. Value: %+v", item.Val)
			natsGMLogger.Error().Msg(msg)
			return nil, fmt.Errorf(msg)
		}
		gameID, err := strconv.ParseUint(gameIDStr, 10, 64)
		if err != nil {
			msg := fmt.Sprintf("GetGames unable to parse gameIDStr as uint64. Value: %s", gameIDStr)
			natsGMLogger.Error().Msg(msg)
			return nil, fmt.Errorf(msg)
		}
		gameCode := game.gameCode
		games = append(games, GameListItem{
			GameID:   gameID,
			GameCode: gameCode,
		})
	}
	return games, nil
}

func (gm *GameManager) CrashCleanup(gameID uint64) {
	natsGMLogger.Error().Uint64("gameID", gameID).Msgf("CrashCleanup called", gameID)
	gm.EndNatsGame(gameID)
}

func (gm *GameManager) EndNatsGame(gameID uint64) {
	gameIDStr := fmt.Sprintf("%d", gameID)
	if v, exists := gm.activeGames.Get(gameIDStr); exists {
		game := v.(*NatsGame)
		game.gameEnded()
		game.cleanup()
		gm.activeGames.Remove(gameIDStr)
		if gameCode, exists := gm.gameIDToCode.Get(gameIDStr); exists {
			gm.gameCodeToID.Remove(gameCode.(string))

		}
		gm.gameIDToCode.Remove(gameIDStr)
		util.Metrics.SetActiveGamesMapCount(gm.activeGames.Count())
	}
}

func (gm *GameManager) ResumeGame(gameID uint64) {
	gameIDStr := fmt.Sprintf("%d", gameID)
	if g, ok := gm.activeGames.Get(gameIDStr); ok {
		g.(*NatsGame).resumeGame()
	} else {
		natsGMLogger.Error().Uint64("gameId", gameID).Msgf("GameID: %d does not exist", gameID)
	}
}

func (gm *GameManager) SetupHand(handSetup HandSetup) {
	// first check whether the game is hosted by this game server
	gameIDStr := fmt.Sprintf("%d", handSetup.GameId)
	if handSetup.GameId == 0 {
		v, _ := gm.gameCodeToID.Get(handSetup.GameCode)
		gameIDStr = v.(string)
	}
	var natsGame *NatsGame
	v, ok := gm.activeGames.Get(gameIDStr)
	if ok {
		natsGame = v.(*NatsGame)
	} else {
		natsGMLogger.Error().Str("gameId", handSetup.GameCode).Msgf("Game code: %s does not exist. Aborting setup-deck.", handSetup.GameCode)
		return
	}

	// send the message to the game to setup next hand
	natsGame.setupHand(handSetup)
}

func (gm *GameManager) GetCurrentHandLog(gameID uint64) (*map[string]interface{}, bool /*success*/) {
	// first check whether the game is hosted by this game server
	gameIDStr := fmt.Sprintf("%d", gameID)
	var natsGame *NatsGame
	v, ok := gm.activeGames.Get(gameIDStr)
	if ok {
		natsGame = v.(*NatsGame)
	} else {
		// lookup using game code
		var errors map[string]interface{}
		errors["errors"] = fmt.Sprintf("Cannot find game %d", gameID)
		return &errors, false
	}
	handLog := natsGame.getHandLog()
	return handLog, true
}
