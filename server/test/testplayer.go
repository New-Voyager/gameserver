package test

import (
	"encoding/binary"
	"fmt"
	"strconv"

	jsoniter "github.com/json-iterator/go"
	"voyager.com/logging"
	"voyager.com/server/game"
)

var testPlayerLogger = logging.GetZeroLogger("test::testplayer", nil)

// TestPlayer is a receiver for game and hand messages
// it also sends messages to game and hand via player object
type TestPlayer struct {
	playerInfo   game.GamePlayer
	player       *game.Player
	seatNo       uint32
	testObserver bool
	observerCh   chan observerChItem

	// we preserve the last message
	lastHandMessage     *game.HandMessage
	lastHandMessageItem *game.HandMessageItem
	lastGameMessage     *game.GameMessage
	actionChange        *game.HandMessageItem
	noMoreActions       *game.HandMessageItem
	runItTwice          *game.HandMessageItem
	yourAction          *game.HandMessageItem

	// current hand message
	currentHand *game.HandMessageItem

	// players cards
	cards []uint32

	// preserve different stages of the messages
	flop       *game.Flop
	turn       *game.Turn
	river      *game.River
	showdown   *game.Showdown
	handResult *game.HandResultClient

	observer *TestPlayer
	// preserve last received table state
	lastTableState *game.TestGameTableStateMessage
}

func NewTestPlayer(playerInfo game.GamePlayer, observer *TestPlayer) *TestPlayer {
	return &TestPlayer{
		playerInfo: playerInfo,
		observer:   observer,
	}
}

func NewTestPlayerAsObserver(playerInfo game.GamePlayer, observerCh chan observerChItem) *TestPlayer {
	return &TestPlayer{
		playerInfo:   playerInfo,
		testObserver: true,
		observerCh:   observerCh,
	}
}

func (t *TestPlayer) setPlayer(player *game.Player) {
	t.player = player
}

func (t *TestPlayer) joinGame(gameID uint64, seatNo uint32, buyIn float32, runItTwice bool, runItTwicePromptResponse bool, postBlind bool) {
	t.player.JoinGame(gameID, seatNo, buyIn, runItTwice, runItTwicePromptResponse, postBlind)
}

func (t *TestPlayer) resetBlinds(gameID uint64) {
	t.player.ResetBlinds(gameID)
}

func (t *TestPlayer) HandMessageFromGame(messageBytes []byte, handMessage *game.HandMessage, msgItem *game.HandMessageItem, jsonb []byte) {

	if msgItem.MessageType == game.HandNewHand {
		t.currentHand = msgItem
		t.flop = nil
		/*
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
			}*/
	} else if msgItem.MessageType == "DEAL" {
		// unscramble cards
		maskedCards := msgItem.GetDealCards().Cards
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
	} else if msgItem.MessageType == "FLOP" {
		t.flop = msgItem.GetFlop()
	} else if msgItem.MessageType == "TURN" {
		t.turn = msgItem.GetTurn()
	} else if msgItem.MessageType == "RIVER" {
		t.river = msgItem.GetRiver()
	}
	t.lastHandMessage = handMessage
	//fmt.Printf("%s\n", msgItem.MessageType)
	if msgItem.MessageType == game.HandResultMessage2 {
		newResult := msgItem.GetHandResultClient()
		t.handResult = newResult
	}
	t.lastHandMessageItem = msgItem

	logged := false
	if msgItem.MessageType == game.HandNextAction {
		t.actionChange = msgItem
	}

	if t.testObserver {
		if msgItem.MessageType == game.HandPlayerAction ||
			// handMessage.MessageType == game.HandNextAction ||
			msgItem.MessageType == game.HandNewHand ||
			msgItem.MessageType == game.HandResultMessage2 ||
			msgItem.MessageType == game.HandFlop ||
			msgItem.MessageType == game.HandTurn ||
			msgItem.MessageType == game.HandRiver ||
			msgItem.MessageType == game.HandNoMoreActions ||
			msgItem.MessageType == game.HandRunItTwice {

			if msgItem.MessageType != game.HandNextAction {
				testPlayerLogger.Debug().
					Uint32("club", t.player.ClubID).
					Uint64("game", t.player.GameID).
					Uint64("playerid", t.player.PlayerID).
					Uint32("seatNo", t.player.SeatNo).
					Str("player", t.player.PlayerName).
					Msg(fmt.Sprintf("%s", string(jsonb)))
			}
			if msgItem.MessageType == game.HandResultMessage {
				testPlayerLogger.Debug().
					Uint32("club", t.player.ClubID).
					Uint64("game", t.player.GameID).
					Uint64("playerid", t.player.PlayerID).
					Uint32("seatNo", t.player.SeatNo).
					Str("player", t.player.PlayerName).
					Msg(fmt.Sprintf("%s", string(jsonb)))
			}
			if msgItem.MessageType == game.HandResultMessage2 {
				testPlayerLogger.Debug().
					Uint32("club", t.player.ClubID).
					Uint64("game", t.player.GameID).
					Uint64("playerid", t.player.PlayerID).
					Uint32("seatNo", t.player.SeatNo).
					Str("player", t.player.PlayerName).
					Msg(fmt.Sprintf("%s", string(jsonb)))
			}
			// save next action information
			// used for pot validation
			if msgItem.MessageType == game.HandNoMoreActions {
				t.noMoreActions = msgItem
			}

			if msgItem.MessageType == game.HandRunItTwice {
				t.runItTwice = msgItem
			}

			logged = true
			// signal the observer to consume this message
			t.observerCh <- observerChItem{
				gameMessage: nil,
				handMessage: handMessage,
				handMsgItem: msgItem,
			}
		}
	} else {
		if msgItem.MessageType == game.HandPlayerAction {
			t.yourAction = msgItem
			// tell observer to consume
			if t.observer != nil {
				t.observer.observerCh <- observerChItem{
					gameMessage: nil,
					handMessage: handMessage,
					handMsgItem: msgItem,
				}
			}
		} else if msgItem.MessageType == game.HandDeal {
			// tell observer to consume
			if t.observer != nil {
				t.observer.observerCh <- observerChItem{
					gameMessage: nil,
					handMessage: handMessage,
					handMsgItem: msgItem,
				}
			}
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
			t.observerCh <- observerChItem{
				gameMessage: gameMessage,
				handMessage: nil,
				handMsgItem: nil,
			}
			// } else if messageTypeStr == "PLAYER_SAT" {
			// 	if gameMessage.GetPlayerSat().PlayerId == t.player.PlayerID {
			// 		t.seatNo = gameMessage.GetPlayerSat().SeatNo
			// 	}
		} else {
			t.lastGameMessage = gameMessage
		}
	}
}
