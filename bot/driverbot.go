package bot

import (
	"fmt"
	"io/ioutil"

	jsoniter "github.com/json-iterator/go"

	natsgo "github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
	"voyager.com/server/test"
)

var driverBotLogger = log.With().Str("logger_name", "server::driverbot").Logger()

const NatsURL = "nats://localhost:4222"
const testDriverToGame = "driverbot.2game"
const gameToTestDriver = "game.2driverpot"

// bot driver messages to game
const (
	BotDriverInitializeGame = "BotInitializeGame"
	BotDriverStartGame      = "BotStartGame"
)

type DriveBotMessage struct {
	MessageType string           `json:"message-type"`
	GameConfig  *test.GameConfig `json:"game-config"`
}

type DriverBot struct {
	stopped chan bool
	nc      *natsgo.Conn
}

func NewDriverBot(url string) (*DriverBot, error) {
	nc, err := natsgo.Connect(url)
	if err != nil {
		driverBotLogger.Error().Msg(fmt.Sprintf("Error connecting to NATS server, error: %v", err))
		return nil, err
	}

	driverBot := &DriverBot{
		stopped: make(chan bool),
		nc:      nc,
	}
	nc.Subscribe(gameToTestDriver, driverBot.listenForMessages)
	return driverBot, nil
}

func (b *DriverBot) Cleanup() {
	b.nc.Close()
}

func (b *DriverBot) listenForMessages(msg *natsgo.Msg) {
	fmt.Printf("Message to bot")
}

func (b *DriverBot) RunGameScript(filename string) error {
	fmt.Printf("Running game script: %s\n", filename)

	// load game script
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		// failed to load game script file
		fmt.Printf("Failed to load file: %s\n", filename)
		return err
	}

	var gameScript test.GameScript
	err = yaml.Unmarshal(data, &gameScript)
	if err != nil {
		// failed to load game script file
		fmt.Printf("Loading json failed: %s, err: %v\n", filename, err)
		return err
	}
	if gameScript.Disabled {
		return nil
	}

	b.run(&gameScript)

	return nil
}

func (b *DriverBot) run(gameScript *test.GameScript) error {
	// initialize game by sending the message to game server
	data, _ := jsoniter.Marshal(gameScript.GameConfig)

	// send to game server
	e := b.nc.Publish(testDriverToGame, data)
	if e != nil {
		return e
	}

	// wait for the response

	return nil
}
