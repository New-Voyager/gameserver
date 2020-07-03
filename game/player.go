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
	chGame chan []byte
	chHand chan []byte // protobuf HandMessage

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
		chGame:     make(chan []byte),
		chHand:     make(chan []byte),
	}

	return &channelPlayer
}

func (p *Player) handleHandMessage(message HandMessage) {
	if message.MessageType == HandDeal {
		p.onCardsDealt(message)
	} else if message.MessageType == HandNextAction {
		p.onNextAction(message)
	} else {
		playerLogger.Warn().
			Uint32("club", message.ClubId).
			Uint32("game", message.GameNum).
			Msg(fmt.Sprintf("Unhandled Hand message type: %s %v", message.MessageType, message))
	}
}

func (p *Player) onCardsDealt(message HandMessage) error {
	// cards dealt, display the cards
	cards := message.GetDealCards()
	// playerLogger.Info().
	// 	Uint32("club", cards.ClubId).
	// 	Uint32("game", cards.GameNum).
	// 	Uint32("hand", cards.HandNum).
	// 	Str("player", p.playerName).
	// 	Msg(fmt.Sprintf("Cards: %s", cards.CardsStr))

	jsonb, err := protojson.Marshal(cards)
	if err != nil {
		return err
	}

	if p.delegate != nil {
		p.delegate.onHandMessage(jsonb)
	}
	return nil
}

func (p *Player) onNextAction(message HandMessage) error {
	// cards dealt, display the cards
	seatAction := message.GetSeatAction()
	// playerLogger.Info().
	// 	Uint32("club", message.ClubId).
	// 	Uint32("game", message.GameNum).
	// 	Uint32("hand", message.HandNum).
	// 	Str("player", p.playerName).
	// 	Msg(fmt.Sprintf("Action: %v", seatAction))

	jsonb, err := protojson.Marshal(seatAction)
	if err != nil {
		return err
	}

	if p.delegate != nil {
		p.delegate.onHandMessage(jsonb)
	}
	return nil
}

func (p *Player) handleGameMessage(message GameMessage) {
	// playerLogger.Info().
	// 	Uint32("club", message.clubID).
	// 	Uint32("game", message.gameNum).
	// 	Msg(fmt.Sprintf("Message type: %s", message.messageType))

	if p.delegate != nil {
		//p.delegate.onGameMessage(jsonb)
	}
}

func (p *Player) playGame() {
	stopped := false
	for !stopped {
		select {
		case message := <-p.chHand:
			var handMessage HandMessage

			err := proto.Unmarshal(message, &handMessage)
			if err == nil {
				p.handleHandMessage(handMessage)
			}
		case message := <-p.chGame:
			var gameMessage GameMessage
			err := proto.Unmarshal(message, &gameMessage)
			if err == nil {
				p.handleGameMessage(gameMessage)
			}
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
