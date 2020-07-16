package nats

import (
	"fmt"

	natsgo "github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
)

// The NATS test driver adapter functionality is to listen for new game
// created by test drivers and intialize the game.
// The NATS game subscription will start for the game and wait
// for the player to join the game.
// The test driver will send start game message after all the players sat
// in the table.
// The test driver will directly interact with the game from that point

// test driver subscription: game.testdriver2game
// test driver publish: game.game2testdriver

const testDriverToGame = "testdriver.2game"
const gameToTestDriver = "game.2testdriver"

type NatsTestDriverListener struct {
	stopped chan bool
	nc      *natsgo.Conn
}

var natsTestDriverLogger = log.With().Str("logger_name", "nats::game").Logger()

func NewNatsTestDriverListener(url string) (*NatsTestDriverListener, error) {
	nc, err := natsgo.Connect(url)
	if err != nil {
		natsTestDriverLogger.Error().Msg(fmt.Sprintf("Error connecting to NATS server, error: %v", err))
		return nil, err
	}

	natsTestDriver := &NatsTestDriverListener{
		stopped: make(chan bool),
		nc:      nc,
	}
	nc.Subscribe(testDriverToGame, natsTestDriver.listenForMessages)
	return natsTestDriver, nil
}

func (n *NatsTestDriverListener) listenForMessages(msg *natsgo.Msg) {
	fmt.Printf("msg: %s\n", string(msg.Data))
}
