package nats

import (
	"fmt"
	"strconv"

	natsgo "github.com/nats-io/nats.go"
	cmap "github.com/orcaman/concurrent-map"
	"github.com/pkg/errors"

	"voyager.com/logging"
	"voyager.com/server/game"
	"voyager.com/server/rpc"
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
		Uint64(logging.GameIDKey, gameID).Str(logging.GameCodeKey, gameCode).
		Msgf("New game %d:%s", gameID, gameCode)
	gameIDStr := fmt.Sprintf("%d", gameID)
	game, err := newNatsGame(gm.nc, gameID, gameCode, 0, 0)
	if err != nil {
		return nil, errors.Wrap(err, "Could not create new NATS game")
	}
	gm.activeGames.Set(gameIDStr, game)
	gm.gameIDToCode.Set(gameIDStr, gameCode)
	gm.gameCodeToID.Set(gameCode, gameIDStr)
	util.Metrics.SetActiveGamesMapCount(gm.activeGames.Count())
	return game, nil
}

func (gm *GameManager) NewTournamentGame(gameCode string, tournamentID uint64, tableNo uint32) (*NatsGame, error) {
	gameID := tournamentID<<32 | uint64(tableNo)
	natsGMLogger.Info().
		Uint64(logging.GameIDKey, gameID).
		Str(logging.GameCodeKey, gameCode).
		Msgf("New Tournament game %d:%s", gameID, gameCode)
	gameIDStr := fmt.Sprintf("%d", gameID)
	game, err := newNatsGame(gm.nc, gameID, gameCode, tournamentID, tableNo)
	if err != nil {
		return nil, errors.Wrap(err, "Could not create new NATS game")
	}
	gm.activeGames.Set(gameIDStr, game)
	gm.gameIDToCode.Set(gameIDStr, gameCode)
	gm.gameCodeToID.Set(gameCode, gameIDStr)
	util.Metrics.SetActiveGamesMapCount(gm.activeGames.Count())
	return game, nil
}

func (gm *GameManager) GetGames() ([]GameListItem, error) {
	games := make([]GameListItem, 0)
	for item := range gm.activeGames.IterBuffered() {
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

func (gm *GameManager) CrashCleanup(gameID uint64, gameCode string) {
	natsGMLogger.Error().
		Uint64(logging.GameIDKey, gameID).
		Str(logging.GameCodeKey, gameCode).
		Msgf("CrashCleanup called")
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
		natsGMLogger.Error().
			Uint64(logging.GameIDKey, gameID).
			Msgf("Game ID does not exist")
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
		natsGMLogger.Error().
			Str(logging.GameCodeKey, handSetup.GameCode).
			Msgf("Game code does not exist. Aborting setup-deck.")
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

func (gm *GameManager) DealTournamentHand(gameCode string, in *rpc.HandInfo) error {
	// first check whether the game is hosted by this game server
	v, _ := gm.gameCodeToID.Get(gameCode)
	gameIDStr := v.(string)
	var natsGame *NatsGame
	v, ok := gm.activeGames.Get(gameIDStr)
	if ok {
		natsGame = v.(*NatsGame)
	} else {
		// lookup using game code
		var errors map[string]interface{}
		errors["errors"] = fmt.Sprintf("Cannot find game %s", gameCode)
		return fmt.Errorf("Cannot find game %s", gameCode)
	}

	/*
		GameID               uint64 `json:"gameId"`
		GameCode             string
		GameType             GameType
		MaxPlayers           uint32
		SmallBlind           float64
		BigBlind             float64
		Ante                 float64
		ButtonPos            uint32
		HandNum              uint32
		ActionTime           uint32
		StraddleBet          float64
		ChipUnit             ChipUnit
		RakePercentage       float64
		RakeCap              float64
		AnnounceGameType     bool
		PlayersInSeats       []SeatPlayer
		GameStatus           GameStatus
		TableStatus          TableStatus
		SbPos                uint32
		BbPos                uint32
		ResultPauseTime      uint32
		BombPot              bool
		DoubleBoard          bool
		BombPotBet           float64
		BringIn              float64
		RunItTwiceTimeout    uint32
		HighHandRank         uint32
		HighHandTracked      bool
		MandatoryStraddle    bool
		TotalHands           int
		StraightFlushCount   int
		FourKindCount        int
		StraightFlushAllowed bool
		FourKindAllowed      bool
		Tournament           bool
	*/
	var hand game.NewHandInfo
	hand.GameCode = in.GameCode
	hand.GameID = in.GameId
	hand.Tournament = true
	hand.HandNum = in.HandDetails.HandNum
	hand.GameType = game.GameType(in.HandDetails.GameType)
	hand.MaxPlayers = in.HandDetails.MaxPlayers
	hand.SbPos = in.HandDetails.SbPos
	hand.BbPos = in.HandDetails.BbPos
	hand.ButtonPos = in.HandDetails.ButtonPos
	hand.SmallBlind = in.HandDetails.Sb
	hand.BigBlind = in.HandDetails.Bb
	hand.Ante = in.HandDetails.Ante
	hand.ActionTime = in.HandDetails.ActionTime
	hand.ResultPauseTime = in.HandDetails.ResultPauseTime
	hand.PlayersInSeats = make([]game.SeatPlayer, 0)
	for _, seat := range in.Seats {
		var sp game.SeatPlayer
		sp.SeatNo = seat.SeatNo
		sp.Name = seat.Name
		sp.PlayerID = seat.PlayerId
		sp.PlayerUUID = seat.PlayerUuid
		sp.Stack = seat.Stack
		sp.Inhand = seat.Inhand
		sp.EncryptionKey = seat.EncryptionKey
		sp.Status = game.PlayerStatus_PLAYING
		hand.PlayersInSeats = append(hand.PlayersInSeats, sp)
	}
	hand.TournamentURL = in.TournamentUrl
	go natsGame.serverGame.DealTournamentHand(&hand)
	return nil
}
