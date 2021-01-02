package nats

import (
	"encoding/json"
	"fmt"

	natsgo "github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/encoding/protojson"
	"voyager.com/server/game"
)

// NatsGame is an adapter that interacts with the NATS server and
// passes the information to the game using the channels

// protocols supported
// StartGame
// PauseGame
// EndGame
// JoinGame
//

var natsLogger = log.With().Str("logger_name", "nats::game").Logger()

// id: clubId.gameNum
/**
For each game, we are going to listen in two subjects for incoming messages from players.
game.<id>.main
game.<id>.hand
game.<id>.heartbeat
game.<id>.driver2game : used by test driver bot to send message to the game
game.<id>.game2driver: used by game to send messages to driver bot

The only message comes from the player for the game is PLAYER_ACTED.
The heartbeat helps us tracking the connectivity of the player.

The gamestate tracks all the active players in the table.

Test driver scenario:
1. Test driver initializes game with game configuration.
2. Launches players to join the game.
3. Waits for all players took the seats.
4. Signals the game to start the game <game>.<id>.game
5. Monitors the players/actions.
*/

type NatsGame struct {
	clubID                 uint32
	gameID                 uint64
	chEndGame              chan bool
	chManageGame           chan []byte
	player2GameSubject     string
	player2HandSubject     string
	hand2PlayerAllSubject  string
	game2AllPlayersSubject string

	serverGame *game.Game

	gameCode       string
	player2GameSub *natsgo.Subscription
	player2HandSub *natsgo.Subscription
	nc             *natsgo.Conn
}

func newNatsGame(nc *natsgo.Conn, clubID uint32, gameID uint64, config *game.GameConfig) (*NatsGame, error) {

	// game subjects
	//player2GameSubject := fmt.Sprintf("game.%d.player", gameID)
	game2AllPlayersSubject := fmt.Sprintf("game.%s.player", config.GameCode)

	// hand subjects
	player2HandSubject := fmt.Sprintf("player.%s.hand", config.GameCode)
	hand2AllPlayersSubject := fmt.Sprintf("hand.%s.player.all", config.GameCode)

	// we need to use the API to get the game configuration
	natsGame := &NatsGame{
		clubID:       clubID,
		gameID:       gameID,
		chEndGame:    make(chan bool),
		chManageGame: make(chan []byte),
		nc:           nc,
		//		player2GameSubject:     player2GameSubject,
		game2AllPlayersSubject: game2AllPlayersSubject,
		player2HandSubject:     player2HandSubject,
		hand2PlayerAllSubject:  hand2AllPlayersSubject,
		gameCode:               config.GameCode,
	}

	// subscribe to topics
	var e error
	natsGame.player2HandSub, e = nc.Subscribe(player2HandSubject, natsGame.player2Hand)
	if e != nil {
		natsLogger.Error().Msg(fmt.Sprintf("Failed to subscribe to %s", player2HandSubject))
		return nil, e
	}

	if config.ActionTime == 0 {
		config.ActionTime = 20
	}

	serverGame, gameID, err := game.GameManager.InitializeGame(*natsGame, config, true)
	if err != nil {
		return nil, err
	}
	natsGame.serverGame = serverGame
	return natsGame, nil
}

func (n *NatsGame) cleanup() {
	n.player2HandSub.Unsubscribe()
	n.player2GameSub.Unsubscribe()
}

// message sent from apiserver to game
func (n *NatsGame) gameStatusChanged(gameID uint64, newStatus game.GameStatus) {
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
		Msg(fmt.Sprintf("APIServer->Game: Status changed. GameID: %d, NewStatus: %s", gameID, game.GameStatus_name[int32(newStatus)]))
	var message game.GameMessage
	message.GameId = gameID
	message.MessageType = game.GameStatusChanged
	message.GameMessage = &game.GameMessage_StatusChange{StatusChange: &game.GameStatusChangeMessage{NewStatus: newStatus}}

	n.serverGame.SendGameMessageToChannel(&message)
}

// message sent from apiserver to game
func (n *NatsGame) playerUpdate(gameID uint64, update *PlayerUpdate) {
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
		Msg(fmt.Sprintf("APIServer->Game: Player update. GameID: %d, PlayerId: %d NewStatus: %s",
			gameID, update.PlayerId, game.PlayerStatus_name[int32(update.Status)]))
	var message game.GameMessage
	message.GameId = gameID
	message.MessageType = game.PlayerUpdate
	playerUpdate := game.GamePlayerUpdate{
		PlayerId:  update.PlayerId,
		SeatNo:    uint32(update.SeatNo),
		Status:    update.Status,
		Stack:     float32(update.Stack),
		BuyIn:     float32(update.BuyIn),
		GameToken: update.GameToken,
	}

	message.GameMessage = &game.GameMessage_PlayerUpdate{PlayerUpdate: &playerUpdate}

	go n.serverGame.SendGameMessageToChannel(&message)
}

func (n *NatsGame) pendingUpdatesDone() {
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
		Msg(fmt.Sprintf("APIServer->Game: Pending updates done. GameID: %d", n.gameID))
	var message game.GameMessage
	message.GameId = n.gameID
	message.MessageType = game.GamePendingUpdatesDone
	go n.serverGame.SendGameMessageToChannel(&message)
}

// message sent from bot to game
func (n *NatsGame) setupDeck(deck []byte, buttonPos uint32) {
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
		Msg(fmt.Sprintf("Bot->Game: Setup deck. GameID: %d, ButtonPos: %d", n.gameID, buttonPos))
	// build a game message and send to the game
	var message game.GameMessage

	nextHand := &game.GameSetupNextHandMessage{
		Deck:      deck,
		ButtonPos: buttonPos,
	}

	message.ClubId = 0
	message.GameId = n.gameID
	message.MessageType = game.GameSetupNextHand
	message.GameMessage = &game.GameMessage_NextHand{NextHand: nextHand}

	n.serverGame.SendGameMessageToChannel(&message)
}

// messages sent from player to game
func (n *NatsGame) player2Game(msg *natsgo.Msg) {
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
		Msg(fmt.Sprintf("Player->Game: %s", string(msg.Data)))
	// convert to protobuf message
	// convert json message to go message
	var message game.GameMessage
	//err := jsoniter.Unmarshal(msg.Data, &message)
	e := protojson.Unmarshal(msg.Data, &message)
	if e != nil {
		return
	}

	n.serverGame.SendGameMessageToChannel(&message)
}

// messages sent from player to game hand
func (n *NatsGame) player2Hand(msg *natsgo.Msg) {
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
		Msg(fmt.Sprintf("Player->Hand: %s", string(msg.Data)))
	var message game.HandMessage
	e := protojson.Unmarshal(msg.Data, &message)
	if e != nil {
		return
	}

	n.serverGame.SendHandMessage(&message)
}

func (n NatsGame) BroadcastGameMessage(message *game.GameMessage) {
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
		Msg(fmt.Sprintf("Game->AllPlayers: %s", message.MessageType))
	// let send this to all players
	data, _ := protojson.Marshal(message)

	if message.MessageType == game.GameCurrentStatus {
		fmt.Printf("%s\n", string(data))
		// update table status
		UpdateTableStatus(message.GameId, message.GetStatus().GetTableStatus())
	}

	n.nc.Publish(n.game2AllPlayersSubject, data)
}

func (n NatsGame) BroadcastHandMessage(message *game.HandMessage) {
	message.PlayerId = 0

	marshaller := protojson.MarshalOptions{
		EmitUnpopulated: true,
	}
	data, _ := marshaller.Marshal(message)
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).Str("Message", message.MessageType).
		Str("subject", n.hand2PlayerAllSubject).
		Msg(fmt.Sprintf("H->A: %s", string(data)))
	n.nc.Publish(n.hand2PlayerAllSubject, data)
}

func (n NatsGame) SendHandMessageToPlayer(message *game.HandMessage, playerID uint64) {
	hand2PlayerSubject := fmt.Sprintf("hand.%s.player.%d", n.gameCode, playerID)
	message.PlayerId = playerID
	data, _ := protojson.Marshal(message)
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).Str("Message", message.MessageType).
		Str("subject", hand2PlayerSubject).
		Msg(fmt.Sprintf("H->P: %s", string(data)))
	n.nc.Publish(hand2PlayerSubject, data)
}

func (n NatsGame) SendGameMessageToPlayer(message *game.GameMessage, playerID uint64) {
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
		Msg(fmt.Sprintf("Game->Player: %s", message.MessageType))

	if playerID == 0 {
		data, _ := protojson.Marshal(message)
		n.chManageGame <- data
	} else {
		subject := fmt.Sprintf("game.%s.player.%d", n.gameCode, playerID)
		data, _ := protojson.Marshal(message)
		n.nc.Publish(subject, data)
	}
}

func (n *NatsGame) gameEnded() error {
	// first send a message to all the players
	message := &game.GameMessage{
		GameId:      n.gameID,
		GameCode:    n.gameCode,
		MessageType: game.GameCurrentStatus,
	}
	message.GameMessage = &game.GameMessage_Status{Status: &game.GameStatusMessage{Status: game.GameStatus_ENDED,
		TableStatus: game.TableStatus_WAITING_TO_BE_STARTED}}
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
		Msg(fmt.Sprintf("Game->All: %s Game ENDED", message.MessageType))
	n.BroadcastGameMessage(message)

	n.serverGame.GameEnded()
	return nil
}

func (n *NatsGame) getHandLog() *map[string]interface{} {
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
		Msg(fmt.Sprintf("Bot->Game: Get HAND LOG: %d", n.gameID))
	// build a game message and send to the game
	var message game.GameMessage

	message.ClubId = 0
	message.GameId = n.gameID
	message.MessageType = game.GetHandLog

	n.serverGame.SendGameMessageToChannel(&message)
	resp := <-n.chManageGame
	var gameMessage game.GameMessage
	protojson.Unmarshal(resp, &gameMessage)
	handStateBytes := gameMessage.GetHandLog()
	var data map[string]interface{}
	if handStateBytes != nil {
		json.Unmarshal(handStateBytes, &data)
	}
	return &data
}
