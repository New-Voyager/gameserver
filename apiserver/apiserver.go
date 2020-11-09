package apiserver

import (
	"fmt"

	jsoniter "github.com/json-iterator/go"
	natsgo "github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"voyager.com/server/game"
)

var logger = log.With().Str("logger_name", "server::apiserver").Logger()

// Subscribes to messages coming from apiserver and act on the messages
// that is sent to this game server.
var gameServerId = 1
var topic = "apiserver.gameserver"
var apiServerUrl = ""

// RegisterGameServer registers game server with the API server
func RegisterGameServer(url string) {
	apiServerUrl = url
}

func SubscribeToNats(nc *natsgo.Conn) {
	log.Info().Msg(fmt.Sprintf("Subscribing to nats topic %s", topic))
	nc.Subscribe(topic, handleApiServerMessages)
}

func handleApiServerMessages(msg *natsgo.Msg) {
	// unmarshal the message
	var data map[string]interface{}
	err := jsoniter.Unmarshal(msg.Data, &data)
	if err != nil {
		return
	}
	var targetGameServer int
	targetGameServer = int(data["gameServer"].(float64))
	if gameServerId != targetGameServer {
		return
	}
	log.Info().Msg(fmt.Sprintf("Message received :- %s", string(msg.Data)))

	messageType := data["type"].(string)
	switch messageType {
	case "NewGame":
		handleNewGame(msg.Data)
	}
}

func handleNewGame(data []byte) {
	var gameInfo game.GameConfig
	err := jsoniter.Unmarshal(data, &gameInfo)
	if err != nil {
		log.Error().Msg(fmt.Sprintf("New game message cannot be unmarshalled. Error: %s", err.Error()))
		return
	}

	log.Info().Msg(fmt.Sprintf("New game is started. ClubId: %d, gameId: %d", gameInfo.ClubId, gameInfo.GameId))

	// get game configuration

	// host the game
}
