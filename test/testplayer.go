package test

import (
	"encoding/binary"
	"fmt"
	"strconv"

	jsoniter "github.com/json-iterator/go"
	"github.com/rs/zerolog/log"
	"voyager.com/server/game"
)

var testPlayerLogger = log.With().Str("logger_name", "test::testplayer").Logger()

// TestPlayer is a receiver for game and hand messages
// it also sends messages to game and hand via player object
type TestPlayer struct {
	playerInfo   game.GamePlayer
	player       *game.Player
	seatNo       uint32
	testObserver bool
	observerCh   chan []byte

	// we preserve the last message
	lastHandMessage *game.HandMessage
	lastGameMessage *game.GameMessage
	actionChange    *game.HandMessage
	noMoreActions   *game.HandMessage
	// current hand message
	currentHand *game.HandMessage

	// players cards
	cards []uint32

	// preserve different stages of the messages
	flop     *game.Flop
	turn     *game.Turn
	river    *game.River
	showdown *game.Showdown

	// preserve last received message

	// preserve last received table state
	lastTableState *game.GameTableStateMessage
}

func NewTestPlayer(playerInfo game.GamePlayer) *TestPlayer {
	return &TestPlayer{
		playerInfo: playerInfo,
	}
}

func NewTestPlayerAsObserver(playerInfo game.GamePlayer, observerCh chan []byte) *TestPlayer {
	return &TestPlayer{
		playerInfo:   playerInfo,
		testObserver: true,
		observerCh:   observerCh,
	}
}

func (t *TestPlayer) setPlayer(player *game.Player) {
	t.player = player
}

func (t *TestPlayer) HandMessageFromGame(messageBytes []byte, handMessage *game.HandMessage, jsonb []byte) {

	if handMessage.MessageType == game.HandNewHand {
		t.currentHand = handMessage
		t.flop = nil

		if t.seatNo > 0 {
			// unscramble cards
			maskedCards := t.currentHand.GetNewHand().PlayerCards[t.seatNo]
			c, _ := strconv.ParseInt(maskedCards, 10, 64)
			b := make([]byte, 8)
			binary.LittleEndian.PutUint64(b, uint64(c))
			cards := make([]uint32, 0)
			i := 0
			for _, card := range b {
				if card == 0 {
					break
				}
				cards = append(cards, uint32(card))
				i++
			}
			t.cards = cards
		}
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
		if handMessage.MessageType == game.HandPlayerAction ||
			handMessage.MessageType == game.HandNextAction ||
			handMessage.MessageType == game.HandNewHand ||
			handMessage.MessageType == game.HandResultMessage ||
			handMessage.MessageType == game.HandFlop ||
			handMessage.MessageType == game.HandTurn ||
			handMessage.MessageType == game.HandRiver ||
			handMessage.MessageType == game.HandNoMoreActions {

			if handMessage.MessageType != game.HandNextAction {
				testPlayerLogger.Info().
					Uint32("club", t.player.ClubID).
					Uint64("game", t.player.GameID).
					Uint64("playerid", t.player.PlayerID).
					Uint32("seatNo", t.player.SeatNo).
					Str("player", t.player.PlayerName).
					Msg(fmt.Sprintf("%s", string(jsonb)))
			}
			if handMessage.MessageType == game.HandResultMessage {
				testPlayerLogger.Error().
					Uint32("club", t.player.ClubID).
					Uint64("game", t.player.GameID).
					Uint64("playerid", t.player.PlayerID).
					Uint32("seatNo", t.player.SeatNo).
					Str("player", t.player.PlayerName).
					Msg(fmt.Sprintf("%s", string(jsonb)))
			}
			// save next action information
			// used for pot validation
			if handMessage.MessageType == game.HandNextAction {
				t.actionChange = handMessage
			}

			if handMessage.MessageType == game.HandNoMoreActions {
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
			Uint64("game", t.player.GameID).
			Uint64("playerid", t.player.PlayerID).
			Uint32("seatNo", t.player.SeatNo).
			Str("player", t.player.PlayerName).
			Msg(fmt.Sprintf("HAND MESSAGE Json: %s", string(jsonb)))
	}
}

func (t *TestPlayer) GameMessageFromGame(messageBytes []byte, gameMessage *game.GameMessage, jsonb []byte) {
	testPlayerLogger.Trace().
		Uint32("club", t.player.ClubID).
		Uint64("game", t.player.GameID).
		Uint64("playerid", t.player.PlayerID).
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

		if messageTypeStr == game.GameTableState {
			t.lastTableState = gameMessage.GetTableState()
			t.observerCh <- messageBytes
		} else if messageTypeStr == "PLAYER_SAT" {
			if gameMessage.GetPlayerSat().PlayerId == t.player.PlayerID {
				t.seatNo = gameMessage.GetPlayerSat().SeatNo
			}
		} else {
			t.lastGameMessage = gameMessage
		}
	}
}
