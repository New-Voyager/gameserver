package game

import (
	"fmt"

	jsoniter "github.com/json-iterator/go"
	"github.com/rs/zerolog/log"
)

var testPlayerLogger = log.With().Str("logger_name", "test::testplayer").Logger()

// TestPlayer is a receiver for game and hand messages
// it also sends messages to game and hand via player object
type TestPlayer struct {
	playerInfo   GamePlayer
	player       *Player
	seatNo       uint32
	testObserver bool
	observerCh   chan []byte

	// we preserve the last message
	lastHandMessage *HandMessage
	lastGameMessage *GameMessage
	actionChange    *HandMessage
	noMoreActions   *HandMessage
	// current hand message
	currentHand *HandMessage

	// players cards
	cards []uint32

	// preserve different stages of the messages
	flop     *Flop
	turn     *Turn
	river    *River
	showdown *Showdown

	// preserve last received message

	// preserve last received table state
	lastTableState *GameTableStateMessage
}

func NewTestPlayer(playerInfo GamePlayer) *TestPlayer {
	return &TestPlayer{
		playerInfo: playerInfo,
	}
}

func NewTestPlayerAsObserver(playerInfo GamePlayer, observerCh chan []byte) *TestPlayer {
	return &TestPlayer{
		playerInfo:   playerInfo,
		testObserver: true,
		observerCh:   observerCh,
	}
}

func (t *TestPlayer) setPlayer(player *Player) {
	t.player = player
}

func (t *TestPlayer) HandMessageFromGame(messageBytes []byte, handMessage *HandMessage, jsonb []byte) {

	if handMessage.MessageType == HandNewHand {
		t.currentHand = handMessage
		t.flop = nil
		t.cards = nil
	} else if handMessage.MessageType == "DEAL" {
		t.cards = handMessage.GetDealCards().Cards
	} else if handMessage.MessageType == "FLOP" {
		t.flop = handMessage.GetFlop()
	} else if handMessage.MessageType == "TURN" {
		t.turn = handMessage.GetTurn()
	} else if handMessage.MessageType == "RIVER" {
		t.river = handMessage.GetRiver()
	}
	t.lastHandMessage = handMessage

	logged := false
	if t.testObserver {
		if handMessage.MessageType == HandPlayerAction ||
			handMessage.MessageType == HandNextAction ||
			handMessage.MessageType == HandNewHand ||
			handMessage.MessageType == HandResultMessage ||
			handMessage.MessageType == HandFlop ||
			handMessage.MessageType == HandTurn ||
			handMessage.MessageType == HandRiver ||
			handMessage.MessageType == HandNoMoreActions {

			if handMessage.MessageType != HandNextAction {
				testPlayerLogger.Info().
					Uint32("club", t.player.ClubID).
					Uint32("game", t.player.GameNum).
					Uint32("playerid", t.player.PlayerID).
					Uint32("seatNo", t.player.SeatNo).
					Str("player", t.player.PlayerName).
					Msg(fmt.Sprintf("%s", string(jsonb)))
			}

			// save next action information
			// used for pot validation
			if handMessage.MessageType == HandNextAction {
				t.actionChange = handMessage
			}

			if handMessage.MessageType == HandNoMoreActions {
				t.noMoreActions = handMessage
			}

			logged = true
			// signal the observer to consume this message
			t.observerCh <- messageBytes
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

func (t *TestPlayer) GameMessageFromGame(messageBytes []byte, gameMessage *GameMessage, jsonb []byte) {
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
