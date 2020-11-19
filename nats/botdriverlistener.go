package nats

import (
	"fmt"

	jsoniter "github.com/json-iterator/go"

	natsgo "github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"voyager.com/server/bot"
)

type PlayerCard struct {
	Cards  []string `json:"cards"`
}

type SetupDeck struct {
	MessageType string       `json:"message-type"`
	GameCode    string       `json:"game-code"`
	GameId      uint64       `json:"game-id"`
	ButtonPos   uint32       `json:"button-pos"`
	Flop        []string     `json:"flop"`
	Turn        string       `json:"turn"`
	River       string       `json:"river"`
	PlayerCards []PlayerCard `json:"player-cards"`
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

var natsTestDriverLogger = log.With().Str("logger_name", "nats::game").Logger()

func NewNatsDriverBotListener(nc *natsgo.Conn, gameManager *GameManager) (*NatsDriverBotListener, error) {
	natsTestDriver := &NatsDriverBotListener{
		stopped: make(chan bool),
		gameManager: gameManager,
		nc:      nc,
	}

	natsTestDriverLogger.Info().Msg(fmt.Sprintf("Listenting nats subject: %s for bot messages", bot.BotDriverToGame))
	nc.Subscribe(bot.BotDriverToGame, natsTestDriver.listenForMessages)
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
	log.Info().Msg(fmt.Sprintf("Message type: %s Game code:- %s", messageType, gameCode))

	switch messageType {
	case bot.BotDriverInitializeGame:
		// unmarshal message
		var botDriverMessage bot.DriverBotMessage
		err := jsoniter.Unmarshal(msg.Data, &botDriverMessage)
		if err != nil {
			// log the error
			natsTestDriverLogger.Error().Msg(fmt.Sprintf("Invalid driver bot message: %s", string(msg.Data)))
			return
		}

		n.initializeGame(&botDriverMessage)
	case bot.BotDriverSetupDeck:
		var setupDeck SetupDeck
		err := jsoniter.Unmarshal(msg.Data, &setupDeck)
		if err != nil {
			natsTestDriverLogger.Error().Msg(fmt.Sprintf("Invalid setup deck message. %s", string(msg.Data)))
			return
		}
		n.gameManager.SetupDeck(setupDeck)
	default:
		natsTestDriverLogger.Warn().Msg(fmt.Sprintf("Unhandled bot driver message: %s", string(msg.Data)))
	}

}

func (n *NatsDriverBotListener) initializeGame(botDriverMessage *bot.DriverBotMessage) {
	gameConfig := botDriverMessage.GameConfig
	clubID := uint32(1)
	gameID := uint64(1)

	// initialize nats game
	_, err := n.gameManager.NewGame(clubID, gameID, gameConfig)
	if err != nil {
		msg := fmt.Sprintf("Unable to initialize nats game: %v", err)
		natsTestDriverLogger.Error().Msg(msg)
		panic(msg)
	}

	// respond to the driver bot with the game num
	response := &bot.DriverBotMessage{
		ClubId:      clubID,
		BotId:       botDriverMessage.BotId,
		GameId:      gameID,
		MessageType: bot.GameInitialized,
	}
	data, _ := jsoniter.Marshal(response)
	err = n.nc.Publish(bot.GameToBotDriver, data)
	if err != nil {
		natsTestDriverLogger.Error().Msg(fmt.Sprintf("Failed to deliver message to driver bot"))
		return
	}
}
