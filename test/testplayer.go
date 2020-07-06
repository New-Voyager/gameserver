package test

import (
	"fmt"

	jsoniter "github.com/json-iterator/go"
	"github.com/rs/zerolog/log"
	"voyager.com/server/game"
)

var testPlayerLogger = log.With().Str("logger_name", "test::testplayer").Logger()

// TestPlayer is a receiver for game and hand messages
// it also sends messages to game and hand via player object
type TestPlayer struct {
	playerInfo GamePlayer
	player     *game.Player

	// we preserve the last message
	lastHandMessage *game.HandMessage
	lastGameMessage *game.GameMessage

	// current hand message
	currentHand *game.HandMessage

	// preserve last received message

	// preserve last received table state
	lastTableState *game.GameTableStateMessage
}

func NewTestPlayer(playerInfo GamePlayer) *TestPlayer {
	return &TestPlayer{
		playerInfo: playerInfo,
	}
}

func (t *TestPlayer) setPlayer(player *game.Player) {
	t.player = player
}

func (t *TestPlayer) HandMessageFromGame(handMessage *game.HandMessage, jsonb []byte) {
	testPlayerLogger.Info().
		Uint32("club", t.player.ClubID).
		Uint32("game", t.player.GameNum).
		Uint32("playerid", t.player.PlayerID).
		Uint32("seatNo", t.player.SeatNo).
		Str("player", t.player.PlayerName).
		Msg(fmt.Sprintf("HAND MESSAGE Json: %s", string(jsonb)))

	if handMessage.MessageType == "NEW_HAND" {
		t.currentHand = handMessage
	}
	t.lastHandMessage = handMessage
}

func (t *TestPlayer) GameMessageFromGame(gameMessage *game.GameMessage, jsonb []byte) {
	testPlayerLogger.Info().
		Uint32("club", t.player.ClubID).
		Uint32("game", t.player.GameNum).
		Uint32("playerid", t.player.PlayerID).
		Uint32("seatNo", t.player.SeatNo).
		Str("player", t.player.PlayerName).
		Msg(fmt.Sprintf("GAME MESSAGE Json: %s", string(jsonb)))

	// parse json message
	var message map[string]jsoniter.RawMessage
	err := jsoniter.Unmarshal(jsonb, &message)
	if err != nil {
		// preserve error
	}
	fmt.Printf("%s\n", string(jsonb))
	if messageType, ok := message["messageType"]; ok {
		var messageTypeStr string
		jsoniter.Unmarshal(messageType, &messageTypeStr)
		// determine message type

		if messageTypeStr == "TABLE_STATE" {
			t.lastTableState = gameMessage.GetTableState()
		} else {
			t.lastGameMessage = gameMessage
		}
	}
}
