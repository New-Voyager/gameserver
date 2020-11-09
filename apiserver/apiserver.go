package apiserver

import (
	"fmt"

	jsoniter "github.com/json-iterator/go"
	natsgo "github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
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

type GameInfo struct {
	ClubId             int     `json:"clubId"`
	GameId             int     `json:"gameId"`
	GameType           int     `json:"gameType"`
	ClubCode           string  `json:"clubCode"`
	GameCode           string  `json:"gameCode"`
	Title              string  `json:"title"`
	SmallBlind         float64 `json:"smallBlind"`
	BigBlind           float64 `json:"bigBlind"`
	StraddleBet        float64 `json:"straddleBet"`
	MinPlayers         float64 `json:"minPlayers"`
	MaxPlayers         float64 `json:"maxPlayers"`
	GameLength         int     `json:"gameLength"`
	RakePercentage     float64 `json:"rakePercentage"`
	RakeCap            float64 `json:"rakeCap"`
	BuyInMin           float64 `json:"buyInMin"`
	BuyInMax           float64 `json:"buyInMax"`
	ActionTime         int     `json:"actionTime"`
	StartedBy          string  `json:"startedBy"`
	StartedByUuid      string  `json:"startedByUuid"`
	BreakLength        int     `json:"breakLength"`
	AutoKickAfterBreak bool    `json:"autoKickAfterBreak"`
}

func handleNewGame(data []byte) {
	var gameInfo GameInfo
	err := jsoniter.Unmarshal(data, &gameInfo)
	if err != nil {
		log.Error().Msg(fmt.Sprintf("New game message cannot be unmarshalled. Error: %s", err.Error()))
		return
	}

	log.Info().Msg(fmt.Sprintf("New game is started. ClubId: %d, gameId: %d", gameInfo.ClubId, gameInfo.GameId))

	// get game configuration

	// host the game
}
