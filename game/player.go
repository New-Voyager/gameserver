package game

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

/**
NOTE: Seat numbers are indexed from 1-9 like the real poker table.
**/

var playerLogger = log.With().Str("logger_name", "game::player").Logger()

type Player struct {
	clubID     uint32
	gameNum    uint32
	playerName string
	playerID   uint32
	chGame     chan GameMessage
	chHand     chan HandMessage
}

func NewPlayer(name string, playerID uint32) *Player {
	channelPlayer := Player{
		playerID:   playerID,
		playerName: name,
		chGame:     make(chan GameMessage),
		chHand:     make(chan HandMessage),
	}

	return &channelPlayer
}

func (c *Player) handleHandMessage(message HandMessage) {
	if message.messageType == HandDeal {
		c.onCardsDealt(message)
	} else {
		playerLogger.Warn().
			Uint32("club", message.clubID).
			Uint32("game", message.gameNum).
			Msg(fmt.Sprintf("Unhandled Hand message type: %s %v", message.messageType, message))
	}
}

func (c *Player) onCardsDealt(message HandMessage) error {
	// cards dealt, display the cards
	cards := &HandDealCards{}
	proto.Unmarshal(message.messageProto, cards)
	playerLogger.Info().
		Uint32("club", cards.ClubId).
		Uint32("game", cards.GameNum).
		Uint32("hand", cards.HandNum).
		Str("player", c.playerName).
		Msg(fmt.Sprintf("Cards: %s", cards.CardsStr))

	jsonb, err := protojson.Marshal(cards)
	if err != nil {
		return err
	}

	playerLogger.Info().
		Uint32("club", cards.ClubId).
		Uint32("game", cards.GameNum).
		Uint32("hand", cards.HandNum).
		Str("player", c.playerName).
		Msg(fmt.Sprintf("Json: %s", string(jsonb)))

	return nil
}

func (c *Player) handleGameMessage(message GameMessage) {
	playerLogger.Info().
		Uint32("club", message.clubID).
		Uint32("game", message.gameNum).
		Msg(fmt.Sprintf("Message type: %s", message.messageType))
}

func (c *Player) playGame() {
	stopped := false
	for !stopped {
		select {
		case message := <-c.chHand:
			c.handleHandMessage(message)
		case message := <-c.chGame:
			c.handleGameMessage(message)
		default:
			// yield
			time.Sleep(50 * time.Millisecond)
		}
	}
}
