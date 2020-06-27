package game

import (
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/rs/zerolog/log"
	"voyager.com/server/poker"
)

/**
NOTE: Seat numbers are indexed from 1-9 like the real poker table.
**/

var playerLogger = log.With().Str("logger_name", "game::player").Logger()

type Player struct {
	clubID           uint32
	gameNum          uint32
	playerName       string
	playerID         uint32
	ch               chan GameMessage
	chGameManagement chan GameMessage
}

func NewPlayer(name string, playerID uint32) *Player {
	channelPlayer := Player{
		playerID:   playerID,
		playerName: name,
		ch:         make(chan GameMessage),
	}

	return &channelPlayer
}

func (c *Player) handleGameMessage(message GameMessage) {
	if message.messageType == HandDeal {
		c.onCardsDealt(message)
	} else {
		playerLogger.Warn().
			Uint32("club", message.clubID).
			Uint32("game", message.gameNum).
			Msg(fmt.Sprintf("Unhandled Hand message type: %s %v", message.messageType, message))
	}
}

func (c *Player) onCardsDealt(message GameMessage) {
	// cards dealt, display the cards
	cards := &HandDealCards{}
	proto.Unmarshal(message.messageProto, cards)
	cardsDisplay := poker.CardsToString(cards.Cards)
	playerLogger.Info().
		Uint32("club", cards.ClubId).
		Uint32("game", cards.GameNum).
		Uint32("hand", cards.HandNum).
		Str("player", c.playerName).
		Msg(fmt.Sprintf("Cards: %s", cardsDisplay))
}

func (c *Player) handleGameManagementMessage(message GameMessage) {
	playerLogger.Info().
		Uint32("club", message.clubID).
		Uint32("game", message.gameNum).
		Msg(fmt.Sprintf("Message type: %s", message.messageType))
}

func (c *Player) playGame() {
	stopped := false
	for !stopped {
		select {
		case message := <-c.ch:
			c.handleGameMessage(message)
		case message := <-c.chGameManagement:
			c.handleGameManagementMessage(message)
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}
}
