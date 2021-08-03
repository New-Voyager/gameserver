package game

import (
	b64 "encoding/base64"
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
var TotalJsonBytesReceived = 0
var TotalBase64BytesReceived = 0
var TotalBinaryDataReceived = 0

//
// Player object is a virtual player who is in a table whether the player
// is siting in the table, or viewing the table, or in the waiting queue
// This virtual player object will have an adapter to exchange messages
// with the app player using websocket or other mechanism
//
// This virtual player cannot exist in the system without a club/game id
//
type Player struct {
	ClubID                   uint32
	GameID                   uint64
	PlayerName               string
	PlayerID                 uint64
	SeatNo                   uint32
	RunItTwice               bool
	RunItTwicePromptResponse bool

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
	HandMessageFromGame(handMessageBytes []byte, handMessage *HandMessage, msgItem *HandMessageItem, json []byte)
	GameMessageFromGame(gameMessageBytes []byte, gameMessage *GameMessage, json []byte)
}

func NewPlayer(clubID uint32, gameID uint64, name string, playerID uint64, delegate PlayerMessageDelegate) *Player {
	channelPlayer := Player{
		ClubID:        clubID,
		GameID:        gameID,
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

func (p *Player) handleHandMessage(messageBytes []byte, message *HandMessage) {
	marshaller := protojson.MarshalOptions{
		EmitUnpopulated: true,
	}
	jsonb, _ := marshaller.Marshal(message)
	base64 := b64.StdEncoding.EncodeToString(messageBytes)

	TotalBase64BytesReceived += len(base64)
	TotalJsonBytesReceived += len(jsonb)
	TotalBinaryDataReceived += len(messageBytes)

	// playerLogger.Warn().Str("dir", "GH->P").Msg(string(jsonb))

	msgItems := message.GetMessages()
	for _, msgItem := range msgItems {
		if msgItem.MessageType == HandDeal {
			p.onCardsDealt(messageBytes, message, msgItem)
		} else if msgItem.MessageType == HandNextAction {
			p.onNextAction(messageBytes, message, msgItem)
		} else if msgItem.MessageType == HandPlayerAction && msgItem.GetSeatAction().SeatNo == p.SeatNo {
			p.onPlayerAction(messageBytes, message, msgItem)
		} else if msgItem.MessageType == HandNewHand {
			p.onPlayerNewHand(messageBytes, message, msgItem)
		} else if msgItem.MessageType == HandResultMessage {
			p.onHandResult(messageBytes, message, msgItem)
		} else if msgItem.MessageType == HandNoMoreActions {
			p.onHandNoMoreActions(messageBytes, message, msgItem)
		} else if msgItem.MessageType == HandFlop {
			p.onFlop(messageBytes, message, msgItem)
		} else if msgItem.MessageType == HandTurn {
			p.onTurn(messageBytes, message, msgItem)
		} else if msgItem.MessageType == HandRiver {
			p.onRiver(messageBytes, message, msgItem)
		} else if msgItem.MessageType == HandRunItTwice {
			p.onRunItTwice(messageBytes, message, msgItem)
		} else {
			// playerLogger.Warn().
			// 	Uint32("club", message.ClubId).
			// 	Uint64("game", message.GameId).
			// 	Msg(fmt.Sprintf("Unhandled Hand message type: %s %v", msgItem.MessageType, message))
		}
	}
}

func (p *Player) onCardsDealt(messageBytes []byte, message *HandMessage, msgItem *HandMessageItem) error {
	jsonb, err := protojson.Marshal(message)
	if err != nil {
		return err
	}
	playerLogger.Info().Msg(string(jsonb))

	if p.delegate != nil {
		p.delegate.HandMessageFromGame(messageBytes, message, msgItem, jsonb)
	}
	return nil
}

func (p *Player) onPlayerNewHand(messageBytes []byte, message *HandMessage, msgItem *HandMessageItem) error {
	marshaller := protojson.MarshalOptions{
		EmitUnpopulated: true,
	}
	jsonb, err := marshaller.Marshal(message)
	if err != nil {
		return err
	}

	if p.delegate != nil {
		p.delegate.HandMessageFromGame(messageBytes, message, msgItem, jsonb)
	}
	return nil
}

func (p *Player) onNextAction(messageBytes []byte, message *HandMessage, msgItem *HandMessageItem) error {
	jsonb, err := protojson.Marshal(message)
	if err != nil {
		return err
	}

	if p.delegate != nil {
		p.delegate.HandMessageFromGame(messageBytes, message, msgItem, jsonb)
	}
	return nil
}

func (p *Player) onPlayerAction(messageBytes []byte, message *HandMessage, msgItem *HandMessageItem) error {
	// this player is next to act
	jsonb, err := protojson.Marshal(message)
	if err != nil {
		return err
	}

	if p.delegate != nil {
		if msgItem.GetSeatAction().SeatNo != p.SeatNo {
			error := fmt.Sprintf("Seat number %d in message is not matching with seat number %d in test config", msgItem.GetSeatAction().SeatNo, p.SeatNo)
			panic(error)
		}

		seatAction := msgItem.GetSeatAction()
		if seatAction.AvailableActions != nil && len(seatAction.AvailableActions) >= 1 {
			if seatAction.AvailableActions[0] == ACTION_RUN_IT_TWICE_PROMPT {
				playerLogger.Info().
					Str("game", message.GameCode).
					Msg(fmt.Sprintf("Run it twice prompt. Seat No: %d", seatAction.SeatNo))
			}
		}

		p.delegate.HandMessageFromGame(messageBytes, message, msgItem, jsonb)
	}
	return nil
}

func (p *Player) onHandResult(messageBytes []byte, message *HandMessage, msgItem *HandMessageItem) error {
	// this player is next to act
	jsonb, err := protojson.Marshal(message)
	if err != nil {
		return err
	}

	if p.delegate != nil {
		p.delegate.HandMessageFromGame(messageBytes, message, msgItem, jsonb)
	}
	return nil
}

func (p *Player) onFlop(messageBytes []byte, message *HandMessage, msgItem *HandMessageItem) error {
	jsonb, err := protojson.Marshal(message)
	if err != nil {
		return err
	}

	if p.delegate != nil {
		p.delegate.HandMessageFromGame(messageBytes, message, msgItem, jsonb)
	}
	return nil
}

func (p *Player) onHandNoMoreActions(messageBytes []byte, message *HandMessage, msgItem *HandMessageItem) error {
	jsonb, err := protojson.Marshal(message)
	if err != nil {
		return err
	}

	if p.delegate != nil {
		p.delegate.HandMessageFromGame(messageBytes, message, msgItem, jsonb)
	}
	return nil
}

func (p *Player) onTurn(messageBytes []byte, message *HandMessage, msgItem *HandMessageItem) error {
	jsonb, err := protojson.Marshal(message)
	if err != nil {
		return err
	}

	if p.delegate != nil {
		p.delegate.HandMessageFromGame(messageBytes, message, msgItem, jsonb)
	}
	return nil
}

func (p *Player) onRiver(messageBytes []byte, message *HandMessage, msgItem *HandMessageItem) error {
	jsonb, err := protojson.Marshal(message)
	if err != nil {
		return err
	}

	if p.delegate != nil {
		p.delegate.HandMessageFromGame(messageBytes, message, msgItem, jsonb)
	}
	return nil
}

func (p *Player) onRunItTwice(messageBytes []byte, message *HandMessage, msgItem *HandMessageItem) error {
	jsonb, err := protojson.Marshal(message)
	if err != nil {
		return err
	}

	if p.delegate != nil {
		p.delegate.HandMessageFromGame(messageBytes, message, msgItem, jsonb)
	}
	return nil
}

func (p *Player) handleGameMessage(messageBytes []byte, message *GameMessage) error {
	jsonb, err := protojson.Marshal(message)
	if err != nil {
		return err
	}
	playerLogger.Warn().Str("dir", "G->P").Msg(string(jsonb))

	if p.delegate != nil {
		p.delegate.GameMessageFromGame(messageBytes, message, jsonb)
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
				p.handleHandMessage(message, &handMessage)
			}
		case message := <-p.chGame:
			var gameMessage GameMessage
			err := proto.Unmarshal(message, &gameMessage)
			if err == nil {
				p.handleGameMessage(message, &gameMessage)
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

	// send incoming message to the game
	p.sendGameMessage(message)
	return nil
}

func (p *Player) JoinGame(gameID uint64, seatNo uint32, buyIn float32, runItTwice bool, runItTwicePromptResponse bool, postBlind bool) error {
	gameIDStr := fmt.Sprintf("%d", gameID)
	if _, ok := GameManager.activeGames[gameIDStr]; !ok {
		// game not found
		return fmt.Errorf("Game %d is not found", gameID)
	}
	p.SeatNo = seatNo
	p.RunItTwice = runItTwice
	p.RunItTwicePromptResponse = runItTwicePromptResponse
	game, _ := GameManager.activeGames[gameIDStr]
	game.addScriptTestPlayer(p, buyIn, postBlind)
	p.game = game

	// start listenting for game/hand events
	go p.playGame()

	return nil
}

func (p *Player) ResetBlinds(gameID uint64) error {
	gameIDStr := fmt.Sprintf("%d", gameID)
	if _, ok := GameManager.activeGames[gameIDStr]; !ok {
		// game not found
		return fmt.Errorf("Game %d is not found", gameID)
	}
	game, _ := GameManager.activeGames[gameIDStr]
	game.resetBlinds()
	return nil
}

// SetupNextHand method can be called only from the test driver
// and this is available only in test mode.
// We will never allow hands to be set by any scripts in real games
func (p *Player) SetupNextHand(num uint32, handSetup HandSetup) error {
	var message GameMessage
	var playerCards []*GameSetupSeatCards
	var playerCardsBySeat map[uint32]*GameSetupSeatCards
	if handSetup.SeatCards != nil {
		for _, sc := range handSetup.SeatCards {
			seatCards := GameSetupSeatCards{
				Cards: sc.Cards,
			}
			playerCards = append(playerCards, &seatCards)
			if sc.SeatNo != 0 {
				if playerCardsBySeat == nil {
					playerCardsBySeat = make(map[uint32]*GameSetupSeatCards)
				}
				playerCardsBySeat[sc.SeatNo] = &seatCards
			}
		}
	}

	nextHand := &TestHandSetup{
		HandNum:           num,
		ButtonPos:         handSetup.ButtonPos,
		Board:             handSetup.Board,
		Board2:            handSetup.Board2,
		Flop:              handSetup.Flop,
		Turn:              handSetup.Turn,
		River:             handSetup.River,
		PlayerCards:       playerCards,
		PlayerCardsBySeat: playerCardsBySeat,
		Pause:             0,
	}

	message.ClubId = p.ClubID
	message.GameId = p.GameID
	message.MessageType = GameSetupNextHand
	message.GameMessage = &GameMessage_NextHand{NextHand: nextHand}

	e := p.GameProtoMessageFromAdapter(&message)

	return e
}

func (p *Player) GetTableState() error {
	queryTableState := &GameQueryTableStateMessage{PlayerId: p.PlayerID}
	var gameMessage GameMessage
	gameMessage.ClubId = p.ClubID
	gameMessage.GameId = p.GameID
	gameMessage.PlayerId = p.PlayerID
	gameMessage.MessageType = GameQueryTableState
	gameMessage.GameMessage = &GameMessage_QueryTableState{QueryTableState: queryTableState}
	e := p.GameProtoMessageFromAdapter(&gameMessage)
	return e
}

func (p *Player) sendGameMessage(message *GameMessage) error {
	messageData, err := proto.Marshal(message)
	if err != nil {
		return err
	}
	gameIDStr := fmt.Sprintf("%d", p.GameID)
	if _, ok := GameManager.activeGames[gameIDStr]; !ok {
		// game not found
		return fmt.Errorf("Game %d is not found", p.GameID)
	}
	game, _ := GameManager.activeGames[gameIDStr]
	game.chGame <- messageData
	return nil
}

func (p *Player) DealHand() error {

	var gameMessage GameMessage

	dealHandMessage := &GameDealHandMessage{}

	gameMessage.ClubId = p.ClubID
	gameMessage.GameId = p.GameID
	gameMessage.MessageType = GameDealHand
	gameMessage.GameMessage = &GameMessage_DealHand{DealHand: dealHandMessage}
	e := p.GameProtoMessageFromAdapter(&gameMessage)
	return e
}
