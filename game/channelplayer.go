package game

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/rs/zerolog/log"
	"voyager.com/server/poker"
)

/**
NOTE: Seat numbers are indexed from 1-9 like the real poker table.
**/

var channePlayerLogger = log.With().Str("logger_name", "game::channelplayer").Logger()

type ChannelPlayer struct {
	clubID     uint32
	gameNum    uint32
	playerName string
	playerID   uint32
	ch         chan GameMessage
}

func NewPlayer(name string, playerID uint32) *ChannelPlayer {
	channelPlayer := ChannelPlayer{
		playerID:   playerID,
		playerName: name,
		ch:         make(chan GameMessage),
	}

	return &channelPlayer
}

func (c *ChannelPlayer) handleGameMessage(message GameMessage) {
	if message.messageType == MessageDeal {
		c.onCardsDealt(message)
	} else {
		channePlayerLogger.Warn().
			Uint32("club", message.clubID).
			Uint32("game", message.gameNum).
			Msg(fmt.Sprintf("Unhandled Hand message type: %s %v", message.messageType, message))
	}
}

func (c *ChannelPlayer) onCardsDealt(message GameMessage) {
	// cards dealt, display the cards
	cards := &HandDealCards{}
	proto.Unmarshal(message.messageProto, cards)
	cardsDisplay := poker.CardsToString(cards.Cards)
	channePlayerLogger.Info().
		Uint32("club", cards.ClubId).
		Uint32("game", cards.GameNum).
		Uint32("hand", cards.HandNum).
		Str("player", c.playerName).
		Msg(fmt.Sprintf("Cards: %s", cardsDisplay))
}

func (c *ChannelPlayer) handleGameManagementMessage(message GameMessage) {
	channePlayerLogger.Debug().
		Uint32("club", message.clubID).
		Uint32("game", message.gameNum).
		Msg(fmt.Sprintf("Message type: %s", message.messageType))
}

func (c *ChannelPlayer) playGame(gameChannel chan GameMessage, gameManagementChannel chan GameMessage) {
	stopped := false
	for !stopped {
		select {
		case message := <-c.ch:
			c.handleGameMessage(message)
		case message := <-gameManagementChannel:
			c.handleGameManagementMessage(message)
		}
	}
}
