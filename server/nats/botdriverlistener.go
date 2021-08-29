package nats

import (
	"fmt"

	jsoniter "github.com/json-iterator/go"
	"voyager.com/server/game"

	natsgo "github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
)

const BotDriverToGame = "driverbot.game"
const GameToBotDriver = "game.driverpot"
const botPlayerID = 0xFFFFFFFF

// bot driver messages to game
const (
	BotDriverStartGame = "B2GStartGame"
	BotDriverSetupDeck = "B2GSetupDeck"
)

type PlayerCard struct {
	Seat  uint32   `json:"seat"`
	Cards []string `json:"cards"`
}

type HandSetup struct {
	Num                  uint32       `json:"num"`
	MessageType          string       `json:"message-type"`
	GameCode             string       `json:"game-code"`
	GameId               uint64       `json:"game-id"`
	ButtonPos            uint32       `json:"button-pos"`
	Board                []string     `json:"board"`
	Board2               []string     `json:"board2"`
	Flop                 []string     `json:"flop"`
	Turn                 string       `json:"turn"`
	River                string       `json:"river"`
	PlayerCards          []PlayerCard `json:"player-cards"`
	Pause                uint32       `json:"pause"` // pauses before dealing next hand
	BombPot              bool         `json:"bomb-pot"`
	BombPotBet           float32      `json:"bomb-pot-bet"`
	DoubleBoard          bool         `json:"double-board"`
	IncludeStatsInResult bool         `json:"include-stats"`
	ResultPauseTime      uint32       `json:"result-pause-time"`
}

// The NATS test driver adapter functionality is to listen for new game
// created by test drivers and intialize the game.
// The NATS game subscription will start for the game and wait
// for the player to join the game.
// The test driver will send start game message after all the players sat
// in the table.
// The test driver will directly interact with the game from that point

// test driver subscription: game.testdriver2game
// test driver publish: game.game2testdriver

type NatsDriverBotListener struct {
	stopped     chan bool
	nc          *natsgo.Conn
	gameManager *GameManager
}

// game to bot driver messages
const (
	GameInitialized = "G2BGameInitialized"
)

type DriverBotMessage struct {
	BotId       string               `json:"bot-id"`
	MessageType string               `json:"message-type"`
	GameConfig  *game.TestGameConfig `json:"game-config"`
	ClubId      uint32               `json:"club-id"`
	GameId      uint64               `json:"game-id"`
	GameCode    string               `json:"game-code"`
}

var natsTestDriverLogger = log.With().Str("logger_name", "nats::game").Logger()

func NewNatsDriverBotListener(nc *natsgo.Conn, gameManager *GameManager) (*NatsDriverBotListener, error) {
	natsTestDriver := &NatsDriverBotListener{
		stopped:     make(chan bool),
		gameManager: gameManager,
		nc:          nc,
	}

	natsTestDriverLogger.Info().Msgf("Listenting nats subject: %s for bot messages", BotDriverToGame)
	nc.Subscribe(BotDriverToGame, natsTestDriver.listenForMessages)
	return natsTestDriver, nil
}

func (n *NatsDriverBotListener) listenForMessages(msg *natsgo.Msg) {
	fmt.Printf("msg: %s\n", string(msg.Data))

	var data map[string]interface{}
	err := jsoniter.Unmarshal(msg.Data, &data)
	if err != nil {
		return
	}
	messageType := data["message-type"].(string)
	gameCode := data["game-code"].(string)
	log.Debug().Msgf("Message type: %s Game code:- %s", messageType, gameCode)

	switch messageType {
	case BotDriverSetupDeck:
		var handSetup HandSetup
		fmt.Printf("Received setup deck message: %s", string(msg.Data))
		err := jsoniter.Unmarshal(msg.Data, &handSetup)
		if err != nil {
			natsTestDriverLogger.Error().Msgf("Invalid setup deck message. %s", string(msg.Data))
			return
		}
		n.gameManager.SetupHand(handSetup)
	default:
		natsTestDriverLogger.Warn().Msgf("Unhandled bot driver message: %s", string(msg.Data))
	}

}
