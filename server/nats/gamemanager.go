package nats

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	natsgo "github.com/nats-io/nats.go"
	"github.com/pkg/errors"

	"voyager.com/server/game"
	"voyager.com/server/internal"
	"voyager.com/server/poker"
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
	db           *sqlx.DB
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

	db, err := sqlx.Open("postgres", internal.GetConnStr())
	if err != nil {
		return nil, errors.Wrap(err, "Unable to create sqlx handle to postgres")
	}
	err = db.Ping()
	if err != nil {
		return nil, errors.Wrap(err, "Unable to verify postgres connection")
	}

	return &GameManager{
		nc:           nc,
		db:           db,
		activeGames:  make(map[string]*NatsGame),
		gameCodes:    make(map[string]string),
		gameCodeToId: make(map[string]string),
	}, nil
}

func (gm *GameManager) NewGame(clubID uint32, gameID uint64, config *game.GameConfig) (*NatsGame, error) {
	natsLogger.Info().Msgf("New game club %d game %d code %s", clubID, gameID, config.GameCode)
	gameIDStr := fmt.Sprintf("%d", gameID)
	game, err := newNatsGame(gm.nc, gm.db, clubID, gameID, config)
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

func (gm *GameManager) SetupDeck(setupDeck SetupDeck) {
	// first check whether the game is hosted by this game server
	gameIDStr := fmt.Sprintf("%d", setupDeck.GameId)
	if setupDeck.GameId == 0 {
		gameIDStr = gm.gameCodeToId[setupDeck.GameCode]
	}
	var natsGame *NatsGame
	var ok bool
	if natsGame, ok = gm.activeGames[gameIDStr]; !ok {
		// lookup using game code
		gameIDStr, ok = gm.gameCodes[setupDeck.GameCode]
		if !ok {
			natsLogger.Error().Str("gameId", setupDeck.GameCode).Msg(fmt.Sprintf("Game code: %s does not exist. Aborting setup-deck.", setupDeck.GameCode))
			return
		}
	}

	// send the message to the game to setup deck for next hand

	if setupDeck.PlayerCards != nil {
		playerCards := make([]poker.CardsInAscii, 0)
		for _, cards := range setupDeck.PlayerCards {
			playerCards = append(playerCards, cards.Cards)
		}

		var deck *poker.Deck
		if setupDeck.Board != nil {
			deck = poker.DeckFromBoard(playerCards, setupDeck.Board, setupDeck.Board2, false)
		} else {
			// arrange deck
			deck = poker.DeckFromScript(playerCards,
				setupDeck.Flop,
				poker.NewCard(setupDeck.Turn),
				poker.NewCard(setupDeck.River),
				false /* burn card */)
		}
		natsGame.setupDeck(deck.GetBytes(), setupDeck.ButtonPos, setupDeck.Pause)
	} else {
		//deck := poker.NewDeck(nil)
		natsGame.setupDeck(nil, setupDeck.ButtonPos, setupDeck.Pause)
	}
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
