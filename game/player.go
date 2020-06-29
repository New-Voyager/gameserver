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
	clubID                  uint32
	gameNum                 uint32
	playerName              string
	playerID                uint32
	networkConnectionActive bool
	// callbacks to interact with different player communication mechanism
	delegate PlayerMessageDelegate

	// channel used by game object to game related messages
	chGame chan GameMessage
	chHand chan HandMessage

	// game object
	game *Game
}

// PlayerMesssageDelegate are set of callbacks used for communicating
// with different player implementation.
// PlayerTest implements the callbacks for unit testing
// PlayerWebSocket implements callbacks to communicate with the app
type PlayerMessageDelegate interface {
	onHandMessage(json []byte)
	onGameMessage(json []byte)

	getHandChannel() chan []byte
	getGameChannel() chan []byte
}

func NewPlayer(name string, playerID uint32, delegate PlayerMessageDelegate) *Player {
	channelPlayer := Player{
		playerID:   playerID,
		playerName: name,
		delegate:   delegate,
		chGame:     make(chan GameMessage),
		chHand:     make(chan HandMessage),
	}

	return &channelPlayer
}

func (p *Player) handleHandMessage(message HandMessage) {
	if message.messageType == HandDeal {
		p.onCardsDealt(message)
	} else {
		playerLogger.Warn().
			Uint32("club", message.clubID).
			Uint32("game", message.gameNum).
			Msg(fmt.Sprintf("Unhandled Hand message type: %s %v", message.messageType, message))
	}
}

func (p *Player) onCardsDealt(message HandMessage) error {
	// cards dealt, display the cards
	cards := &HandDealCards{}
	proto.Unmarshal(message.messageProto, cards)
	playerLogger.Info().
		Uint32("club", cards.ClubId).
		Uint32("game", cards.GameNum).
		Uint32("hand", cards.HandNum).
		Str("player", p.playerName).
		Msg(fmt.Sprintf("Cards: %s", cards.CardsStr))

	jsonb, err := protojson.Marshal(cards)
	if err != nil {
		return err
	}

	if p.delegate != nil {
		p.delegate.onHandMessage(jsonb)
	}
	return nil
}

func (p *Player) handleGameMessage(message GameMessage) {
	playerLogger.Info().
		Uint32("club", message.clubID).
		Uint32("game", message.gameNum).
		Msg(fmt.Sprintf("Message type: %s", message.messageType))

	if p.delegate != nil {
		//p.delegate.onGameMessage(jsonb)
	}

}

func (p *Player) playGame() {
	stopped := false
	for !stopped {
		select {
		case message := <-p.chHand:
			p.handleHandMessage(message)
		case message := <-p.chGame:
			p.handleGameMessage(message)
		case message := <-p.delegate.getGameChannel():
			p.sendGameMessage(message)
		case message := <-p.delegate.getHandChannel():
			p.sendHandMessage(message)
		default:
			// yield
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func (p *Player) sendGameMessage(json []byte) {
	// convert json into protobuf
}

func (p *Player) sendHandMessage(json []byte) {
	// convert json into protobuf
}
