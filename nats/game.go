package nats

import (
	"fmt"

	natsgo "github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"voyager.com/server/game"
)

// NatsGame is an adapter that interacts with the NATS server and
// passes the information to the game using the channels

// protocols supported
// StartGame
// PauseGame
// EndGame
// JoinGame
//

var natsLogger = log.With().Str("logger_name", "nats::game").Logger()

const NatsURL = "nats://localhost:4222"

var gameManager = game.NewGameManager()

// id: clubId.gameNum
/**
For each game, we are going to listen in two subjects for incoming messages from players.
game.<id>.main
game.<id>.hand
game.<id>.heartbeat
game.<id>.driver2game : used by test driver bot to send message to the game
game.<id>.game2driver: used by game to send messages to driver bot

The only message comes from the player for the game is PLAYER_ACTED.
The heartbeat helps us tracking the connectivity of the player.

The gamestate tracks all the active players in the table.

Test driver scenario:
1. Test driver initializes game with game configuration.
2. Launches players to join the game.
3. Waits for all players took the seats.
4. Signals the game to start the game <game>.<id>.game
5. Monitors the players/actions.
*/

type NatsGame struct {
	clubID    uint32
	gameNum   uint32
	chEndGame chan bool
	nc        *natsgo.Conn
}

func NewGame(clubID uint32, gameNum uint32) (*NatsGame, error) {
	// let us try to connect to nats server
	nc, err := natsgo.Connect(NatsURL)
	if err != nil {
		natsLogger.Error().Msg(fmt.Sprintf("Failed to connect to nats server: %v", err))
		return nil, err
	}

	// hard code the game here

	// we need to use the API to get the game configuration
	game := &NatsGame{
		clubID:    clubID,
		gameNum:   gameNum,
		chEndGame: make(chan bool),
		nc:        nc,
	}

	started := make(chan bool)
	go game.runGame(started)
	<-started

	return game, nil
}

func (n *NatsGame) runGame(started chan bool) {
	// subscribe to topics
	started <- true
}
