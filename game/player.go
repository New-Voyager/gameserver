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
	chGame chan []byte // protobuf GameMessage in bytes
	chHand chan []byte // protobuf HandMessage in bytes

	// adapter channels
	chAdapterGame chan []byte // adapter sending the messages to the game
	chAdapterHand chan []byte // adapter sending hand messages to game hand

	// game object
	game *Game //
}

// PlayerMesssageDelegate are set of callbacks used for communicating
// with different communication implementation.
// TestPlayer implements the callbacks for unit testing
// WebSocketPlayer implements callbacks to communicate with the app
type PlayerMessageDelegate interface {
	HandMessageFromGame(handMessageBytes []byte, handMessage *HandMessage, json []byte)
	GameMessageFromGame(gameMessageBytes []byte, gameMessage *GameMessage, json []byte)
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

func (p *Player) handleHandMessage(messageBytes []byte, message HandMessage) {
	jsonb, _ := protojson.Marshal(&message)
	playerLogger.Warn().Str("dir", "GH->P").Msg(string(jsonb))

	if message.MessageType == HandDeal {
		p.onCardsDealt(messageBytes, message)
	} else if message.MessageType == HandNextAction {
		p.onNextAction(messageBytes, message)
	} else if message.MessageType == HandPlayerAction {
		p.onPlayerAction(messageBytes, message)
	} else if message.MessageType == HandNewHand {
		p.onPlayerNewHand(messageBytes, message)
	} else if message.MessageType == HandResultMessage {
		p.onHandResult(messageBytes, message)
	} else if message.MessageType == HandNoMoreActions {
		p.onHandNoMoreActions(messageBytes, message)
	} else if message.MessageType == HandFlop {
		p.onFlop(messageBytes, message)
	} else if message.MessageType == HandTurn {
		p.onTurn(messageBytes, message)
	} else if message.MessageType == HandRiver {
		p.onRiver(messageBytes, message)
	} else {
		playerLogger.Warn().
			Uint32("club", message.ClubId).
			Uint32("game", message.GameNum).
			Msg(fmt.Sprintf("Unhandled Hand message type: %s %v", message.MessageType, message))
	}
}

func (p *Player) onCardsDealt(messageBytes []byte, message HandMessage) error {
	jsonb, err := protojson.Marshal(&message)
	if err != nil {
		return err
	}
	playerLogger.Info().Msg(string(jsonb))

	if p.delegate != nil {
		p.delegate.HandMessageFromGame(messageBytes, &message, jsonb)
	}
	return nil
}

func (p *Player) onPlayerNewHand(messageBytes []byte, message HandMessage) error {
	jsonb, err := protojson.Marshal(&message)
	if err != nil {
		return err
	}

	if p.delegate != nil {
		p.delegate.HandMessageFromGame(messageBytes, &message, jsonb)
	}
	return nil
}

func (p *Player) onNextAction(messageBytes []byte, message HandMessage) error {
	jsonb, err := protojson.Marshal(&message)
	if err != nil {
		return err
	}

	if p.delegate != nil {
		p.delegate.HandMessageFromGame(messageBytes, &message, jsonb)
	}
	return nil
}

func (p *Player) onPlayerAction(messageBytes []byte, message HandMessage) error {
	// this player is next to act
	jsonb, err := protojson.Marshal(&message)
	if err != nil {
		return err
	}

	if p.delegate != nil {
		p.delegate.HandMessageFromGame(messageBytes, &message, jsonb)
	}
	return nil
}

func (p *Player) onHandResult(messageBytes []byte, message HandMessage) error {
	// this player is next to act
	jsonb, err := protojson.Marshal(&message)
	if err != nil {
		return err
	}

	if p.delegate != nil {
		p.delegate.HandMessageFromGame(messageBytes, &message, jsonb)
	}
	return nil
}

func (p *Player) onFlop(messageBytes []byte, message HandMessage) error {
	jsonb, err := protojson.Marshal(&message)
	if err != nil {
		return err
	}

	if p.delegate != nil {
		p.delegate.HandMessageFromGame(messageBytes, &message, jsonb)
	}
	return nil
}

func (p *Player) onHandNoMoreActions(messageBytes []byte, message HandMessage) error {
	jsonb, err := protojson.Marshal(&message)
	if err != nil {
		return err
	}

	if p.delegate != nil {
		p.delegate.HandMessageFromGame(messageBytes, &message, jsonb)
	}
	return nil
}

func (p *Player) onTurn(messageBytes []byte, message HandMessage) error {
	jsonb, err := protojson.Marshal(&message)
	if err != nil {
		return err
	}

	if p.delegate != nil {
		p.delegate.HandMessageFromGame(messageBytes, &message, jsonb)
	}
	return nil
}

func (p *Player) onRiver(messageBytes []byte, message HandMessage) error {
	jsonb, err := protojson.Marshal(&message)
	if err != nil {
		return err
	}

	if p.delegate != nil {
		p.delegate.HandMessageFromGame(messageBytes, &message, jsonb)
	}
	return nil
}

func (p *Player) handleGameMessage(messageBytes []byte, message GameMessage) error {
	jsonb, err := protojson.Marshal(&message)
	if err != nil {
		return err
	}
	playerLogger.Warn().Str("dir", "G->P").Msg(string(jsonb))

	if p.delegate != nil {
		if message.MessageType == PlayerSat {
			// save seat number
			if message.GetPlayerSat().GetPlayerId() == p.PlayerID {
				p.SeatNo = message.GetPlayerSat().SeatNo
			}
		}
		p.delegate.GameMessageFromGame(messageBytes, &message, jsonb)
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
				p.handleHandMessage(message, handMessage)
			}
		case message := <-p.chGame:
			var gameMessage GameMessage
			err := proto.Unmarshal(message, &gameMessage)
			if err == nil {
				p.handleGameMessage(message, gameMessage)
			}
		case message := <-p.chAdapterGame:
			p.HandMessageFromAdapter(message)
		case message := <-p.chAdapterHand:
			p.GameMessageFromAdapter(message)
		default:
			// yield
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func (p *Player) HandMessageFromAdapter(json []byte) error {
	var message HandMessage
	err := protojson.Unmarshal(json, &message)
	if err != nil {
		return err
	}
	return p.HandProtoMessageFromAdapter(&message)
}

func (p *Player) GameMessageFromAdapter(json []byte) error {
	var message GameMessage
	err := protojson.Unmarshal(json, &message)
	if err != nil {
		return err
	}
	return p.GameProtoMessageFromAdapter(&message)
}

func (p *Player) HandProtoMessageFromAdapter(message *HandMessage) error {
	jsonb, err := protojson.Marshal(message)
	if err != nil {
		return err
	}
	playerLogger.Warn().Str("dir", "P->H").Msg(string(jsonb))

	data, err := proto.Marshal(message)
	if err != nil {
		return err
	}
	p.game.chHand <- data
	return nil
}

func (p *Player) GameProtoMessageFromAdapter(message *GameMessage) error {
	jsonb, err := protojson.Marshal(message)
	if err != nil {
		return err
	}
	playerLogger.Warn().Str("dir", "P->G").Msg(string(jsonb))

	// let us use game manager to handle incoming game messages
	gameManager.handleGameMessage(message, p)
	return nil
}

func (p *Player) startGame(clubID uint32, gameNum uint32) error {
	var message GameMessage
	message.ClubId = clubID
	message.GameNum = gameNum
	message.MessageType = GameStart

	startGame := &GameStartMessage{RequestingPlayerId: p.PlayerID}
	// only club owner/manager can start a game
	message.GameMessage = &GameMessage_StartGame{StartGame: startGame}
	e := p.GameProtoMessageFromAdapter(&message)
	return e
}

func (p *Player) joinGame(clubID uint32, gameNum uint32) error {
	var message GameMessage
	message.ClubId = clubID
	message.GameNum = gameNum
	message.MessageType = GameJoin

	joinGame := &GameJoinMessage{}
	// only club owner/manager can start a game
	message.GameMessage = &GameMessage_JoinGame{JoinGame: joinGame}
	e := p.GameProtoMessageFromAdapter(&message)
	if e != nil {
		p.ClubID = clubID
		p.GameNum = gameNum
	}
	return e
}

func (p *Player) sitAtTable(seatNo uint32, buyIn float32) error {
	var message GameMessage
	message.ClubId = p.ClubID
	message.GameNum = p.GameNum
	message.MessageType = PlayerTakeSeat

	sitMessage := &GameSitMessage{PlayerId: p.PlayerID, SeatNo: seatNo, BuyIn: buyIn}
	// only club owner/manager can start a game
	message.GameMessage = &GameMessage_TakeSeat{TakeSeat: sitMessage}
	e := p.GameProtoMessageFromAdapter(&message)
	return e
}
