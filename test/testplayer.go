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
	playerInfo   GamePlayer
	player       *game.Player
	seatNo       uint32
	testObserver bool
	observerCh   chan int

	// we preserve the last message
	lastHandMessage *game.HandMessage
	lastGameMessage *game.GameMessage

	// current hand message
	currentHand *game.HandMessage

	// platers cards
	cards []uint32

	// preserve last received message

	// preserve last received table state
	lastTableState *game.GameTableStateMessage
}

func NewTestPlayer(playerInfo GamePlayer) *TestPlayer {
	return &TestPlayer{
		playerInfo: playerInfo,
	}
}

func NewTestPlayerAsObserver(playerInfo GamePlayer, observerCh chan int) *TestPlayer {
	return &TestPlayer{
		playerInfo:   playerInfo,
		testObserver: true,
		observerCh:   observerCh,
	}
}

func (t *TestPlayer) setPlayer(player *game.Player) {
	t.player = player
}

func (t *TestPlayer) HandMessageFromGame(handMessage *game.HandMessage, jsonb []byte) {

	if handMessage.MessageType == game.HandNewHand {
		t.currentHand = handMessage
	} else if handMessage.MessageType == "DEAL" {
		t.cards = handMessage.GetDealCards().Cards
	}
	t.lastHandMessage = handMessage

	logged := false
	if t.testObserver {
		if handMessage.MessageType == game.HandPlayerAction ||
			handMessage.MessageType == game.HandNextAction ||
			handMessage.MessageType == game.HandNewHand ||
			handMessage.MessageType == game.HandResultMessage {
			testPlayerLogger.Info().
				Uint32("club", t.player.ClubID).
				Uint32("game", t.player.GameNum).
				Uint32("playerid", t.player.PlayerID).
				Uint32("seatNo", t.player.SeatNo).
				Str("player", t.player.PlayerName).
				Msg(fmt.Sprintf("HAND MESSAGE Json: %s", string(jsonb)))
			logged = true
			// indicate we received a message
			t.observerCh <- 1
			_ = 1
		}
	}

	if !logged {
		testPlayerLogger.Trace().
			Uint32("club", t.player.ClubID).
			Uint32("game", t.player.GameNum).
			Uint32("playerid", t.player.PlayerID).
			Uint32("seatNo", t.player.SeatNo).
			Str("player", t.player.PlayerName).
			Msg(fmt.Sprintf("HAND MESSAGE Json: %s", string(jsonb)))
	}
}

func (t *TestPlayer) GameMessageFromGame(gameMessage *game.GameMessage, jsonb []byte) {
	testPlayerLogger.Trace().
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
	if messageType, ok := message["messageType"]; ok {
		var messageTypeStr string
		jsoniter.Unmarshal(messageType, &messageTypeStr)
		// determine message type

		if messageTypeStr == "TABLE_STATE" {
			t.lastTableState = gameMessage.GetTableState()
		} else if messageTypeStr == "PLAYER_SAT" {
			if gameMessage.GetPlayerSat().PlayerId == t.player.PlayerID {
				t.seatNo = gameMessage.GetPlayerSat().SeatNo
			}
		} else {
			t.lastGameMessage = gameMessage
		}
	}
}
