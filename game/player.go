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

//
// Player object is a virtual player who is in a table whether the player
// is siting in the table, or viewing the table, or in the waiting queue
// This virtual player object will have an adapter to exchange messages
// with the app player using websocket or other mechanism
//
// This virtual player cannot exist in the system without a club/game id
//
type Player struct {
	ClubID                  uint32
	GameNum                 uint32
	PlayerName              string
	PlayerID                uint32
	SeatNo                  uint32
	NetworkConnectionActive bool
	// callbacks to interact with different player communication mechanism
	delegate PlayerMessageDelegate

	// channel used by game object to game related messages
	chGame chan []byte
	chHand chan []byte // protobuf HandMessage

	// adapter channels
	chAdapterGame chan []byte
	chAdapterHand chan []byte

	// game object
	game *Game
}

// PlayerMesssageDelegate are set of callbacks used for communicating
// with different communication implementation.
// TestPlayer implements the callbacks for unit testing
// WebSocketPlayer implements callbacks to communicate with the app
type PlayerMessageDelegate interface {
	HandMessageFromGame(handMessage *HandMessage, json []byte)
	GameMessageFromGame(gameMessage *GameMessage, json []byte)
}

func NewPlayer(clubID uint32, gameNum uint32, name string, playerID uint32, delegate PlayerMessageDelegate) *Player {
	channelPlayer := Player{
		ClubID:        clubID,
		GameNum:       gameNum,
		PlayerID:      playerID,
		PlayerName:    name,
		delegate:      delegate,
		chGame:        make(chan []byte),
		chHand:        make(chan []byte),
		chAdapterGame: make(chan []byte),
		chAdapterHand: make(chan []byte),
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
	//cards := message.GetDealCards()
	// playerLogger.Info().
	// 	Uint32("club", cards.ClubId).
	// 	Uint32("game", cards.GameNum).
	// 	Uint32("hand", cards.HandNum).
	// 	Str("player", p.playerName).
	// 	Msg(fmt.Sprintf("Cards: %s", cards.CardsStr))

	jsonb, err := protojson.Marshal(&message)
	if err != nil {
		return err
	}

	if p.delegate != nil {
		p.delegate.HandMessageFromGame(&message, jsonb)
	}
	return nil
}

func (p *Player) onNextAction(message HandMessage) error {
	// cards dealt, display the cards
	// seatAction := message.GetSeatAction()
	// playerLogger.Info().
	// 	Uint32("club", message.ClubId).
	// 	Uint32("game", message.GameNum).
	// 	Uint32("hand", message.HandNum).
	// 	Str("player", p.playerName).
	// 	Msg(fmt.Sprintf("Action: %v", seatAction))

	jsonb, err := protojson.Marshal(&message)
	if err != nil {
		return err
	}

	if p.delegate != nil {
		p.delegate.HandMessageFromGame(&message, jsonb)
	}
	return nil
}

func (p *Player) handleGameMessage(message GameMessage) error {
	// playerLogger.Info().
	// 	Uint32("club", message.clubID).
	// 	Uint32("game", message.gameNum).
	// 	Msg(fmt.Sprintf("Message type: %s", message.messageType))

	jsonb, err := protojson.Marshal(&message)
	if err != nil {
		return err
	}

	if p.delegate != nil {
		if message.MessageType == PlayerSat {
			// save seat number
			if message.GetPlayerSat().GetPlayerId() == p.PlayerID {
				p.SeatNo = message.GetPlayerSat().SeatNo
			}
		}
		p.delegate.GameMessageFromGame(&message, jsonb)
	}
	return nil
}

// go routine runs on-behalf of player to play a game
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
		case message := <-p.chAdapterGame:
			p.sendGameMessage(message)
		case message := <-p.chAdapterHand:
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

func (p *Player) HandMessageFromAdapter(json []byte) {
	p.chAdapterHand <- json
}

func (p *Player) GameMessageFromAdapter(json []byte) {
	p.chAdapterGame <- json
}
