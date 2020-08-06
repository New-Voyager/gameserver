package nats

import (
	"fmt"

	"voyager.com/server/game"

	jsoniter "github.com/json-iterator/go"

	natsgo "github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"voyager.com/server/bot"
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

type NatsDriverBotListener struct {
	stopped chan bool
	nc      *natsgo.Conn
}

var natsTestDriverLogger = log.With().Str("logger_name", "nats::game").Logger()

func NewNatsDriverBotListener(url string) (*NatsDriverBotListener, error) {
	nc, err := natsgo.Connect(url)
	if err != nil {
		natsTestDriverLogger.Error().Msg(fmt.Sprintf("Error connecting to NATS server, error: %v", err))
		return nil, err
	}

	natsTestDriver := &NatsDriverBotListener{
		stopped: make(chan bool),
		nc:      nc,
	}

	natsTestDriverLogger.Info().Msg(fmt.Sprintf("Listenting nats subject: %s for bot messages", bot.BotDriverToGame))
	nc.Subscribe(bot.BotDriverToGame, natsTestDriver.listenForMessages)
	return natsTestDriver, nil
}

func (n *NatsDriverBotListener) listenForMessages(msg *natsgo.Msg) {
	fmt.Printf("msg: %s\n", string(msg.Data))
	// unmarshal message
	var botDriverMessage bot.DriverBotMessage
	err := jsoniter.Unmarshal(msg.Data, &botDriverMessage)
	if err != nil {
		// log the error
		natsTestDriverLogger.Error().Msg(fmt.Sprintf("Invalid driver bot message: %s", string(msg.Data)))
		return
	}

	switch botDriverMessage.MessageType {
	case bot.BotDriverInitializeGame:
		n.initializeGame(&botDriverMessage)
	default:
		natsTestDriverLogger.Warn().Msg(fmt.Sprintf("Unhandled bot driver message: %s", string(msg.Data)))
	}

}

func (n *NatsDriverBotListener) initializeGame(botDriverMessage *bot.DriverBotMessage) {
	gameConfig := botDriverMessage.GameConfig
	gameType := game.GameType(game.GameType_value[gameConfig.GameType])
	clubID := uint32(1)
	gameNum := uint32(1)

	// initialize nats game
	natsGame, _ := initializeNatsGame(clubID, gameNum)

	serverGame, gameNum := gameManager.InitializeGame(*natsGame, clubID,
		gameNum,
		gameType,
		gameConfig.Title,
		int(gameConfig.MinPlayers),
		gameConfig.AutoStart, false)

	natsGame.serverGame = serverGame

	// respond to the driver bot with the game num
	response := &bot.DriverBotMessage{
		ClubId:      clubID,
		BotId:       botDriverMessage.BotId,
		GameNum:     gameNum,
		MessageType: bot.GameInitialized,
	}
	data, _ := jsoniter.Marshal(response)
	err := n.nc.Publish(bot.GameToBotDriver, data)
	if err != nil {
		natsTestDriverLogger.Error().Msg(fmt.Sprintf("Failed to deliver message to driver bot"))
		return
	}
}