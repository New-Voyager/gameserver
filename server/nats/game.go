package nats

import (
	"encoding/json"
	"fmt"

	natsgo "github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"voyager.com/server/game"
	"voyager.com/server/util"
)

// NatsGame is an adapter that interacts with the NATS server and
// passes the information to the game using the channels

// protocols supported
// StartGame
// PauseGame
// EndGame
// JoinGame
//

var natsGameLogger = log.With().Str("logger_name", "nats::game").Logger()

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
	gameID   uint64
	gameCode string

	chEndGame    chan bool
	chManageGame chan []byte

	hand2AllPlayersSubject string
	game2AllPlayersSubject string
	pingSubject            string

	player2HandSubscription *natsgo.Subscription
	pongSubscription        *natsgo.Subscription
	natsConn                *natsgo.Conn

	maxRetries       uint32
	retryDelayMillis uint32

	serverGame *game.Game
}

func newNatsGame(nc *natsgo.Conn, gameID uint64, gameCode string) (*NatsGame, error) {

	// game subjects
	game2AllPlayersSubject := GetGame2AllPlayerSubject(gameCode)

	// hand subjects
	player2HandSubject := GetPlayer2HandSubject(gameCode)
	hand2AllPlayersSubject := GetHand2AllPlayerSubject(gameCode)

	// we need to use the API to get the game configuration
	natsGame := &NatsGame{
		gameID:                 gameID,
		gameCode:               gameCode,
		chEndGame:              make(chan bool),
		chManageGame:           make(chan []byte),
		game2AllPlayersSubject: game2AllPlayersSubject,
		hand2AllPlayersSubject: hand2AllPlayersSubject,
		pingSubject:            GetPingSubject(gameCode),
		natsConn:               nc,
		maxRetries:             10,
		retryDelayMillis:       1500,
	}

	// subscribe to topics
	var e error
	natsGame.player2HandSubscription, e = nc.Subscribe(player2HandSubject, natsGame.player2Hand)
	if e != nil {
		natsGameLogger.Error().Msgf("Failed to subscribe to %s", player2HandSubject)
		return nil, e
	}

	// for receiving ping response
	playerPongSubject := GetPongSubject(gameCode)
	natsGame.pongSubscription, e = nc.Subscribe(playerPongSubject, natsGame.player2Pong)
	if e != nil {
		natsGameLogger.Error().Msgf("Failed to subscribe to %s", playerPongSubject)
		return nil, e
	}

	serverGame, gameID, err := game.GameManager.InitializeGame(natsGame, gameID, gameCode)
	if err != nil {
		return nil, err
	}
	natsGame.serverGame = serverGame
	err = natsGame.serverGame.GameStarted()
	if err != nil {
		return nil, err
	}
	return natsGame, nil
}

func (n *NatsGame) cleanup() {
	n.player2HandSubscription.Unsubscribe()
	n.pongSubscription.Unsubscribe()
}

func (n *NatsGame) resumeGame() {
	natsGameLogger.Debug().Uint64("game", n.gameID).
		Msg(fmt.Sprintf("APIServer->Game: Resume game. GameID: %d", n.gameID))

	message2 := game.GameMessage{
		GameId:      n.gameID,
		GameCode:    n.gameCode,
		MessageType: game.GameResume,
	}
	n.serverGame.QueueGameMessage(&message2)
}

// message sent from bot to game
func (n *NatsGame) setupHand(handSetup HandSetup) {
	natsGameLogger.Debug().Uint64("game", n.gameID).
		Msg(fmt.Sprintf("Bot->Game: Setup deck. GameID: %d, ButtonPos: %d", n.gameID, handSetup.ButtonPos))
	// build a game message and send to the game
	var message game.GameMessage

	var playerCards []*game.GameSetupSeatCards
	var playerCardsBySeat map[uint32]*game.GameSetupSeatCards
	if handSetup.PlayerCards != nil {
		for _, pc := range handSetup.PlayerCards {
			cards := game.GameSetupSeatCards{
				Cards: pc.Cards,
			}
			playerCards = append(playerCards, &cards)
			if pc.Seat != 0 {
				if playerCardsBySeat == nil {
					playerCardsBySeat = make(map[uint32]*game.GameSetupSeatCards)
				}
				playerCardsBySeat[pc.Seat] = &cards
			}
		}
	}

	nextHandSetup := &game.TestHandSetup{
		HandNum:           handSetup.Num,
		ButtonPos:         handSetup.ButtonPos,
		Board:             handSetup.Board,
		Board2:            handSetup.Board2,
		Flop:              handSetup.Flop,
		Turn:              handSetup.Turn,
		River:             handSetup.River,
		PlayerCards:       playerCards,
		PlayerCardsBySeat: playerCardsBySeat,
		Pause:             handSetup.Pause,
		BombPot:           handSetup.BombPot,
		BombPotBet:        handSetup.BombPotBet,
		DoubleBoard:       handSetup.DoubleBoard,
		IncludeStats:      handSetup.IncludeStatsInResult,
		ResultPauseTime:   handSetup.ResultPauseTime,
	}

	if nextHandSetup.ResultPauseTime == 0 {
		nextHandSetup.ResultPauseTime = 100
	}

	message.GameId = n.gameID
	message.GameCode = n.gameCode
	message.MessageType = game.GameSetupNextHand
	message.GameMessage = &game.GameMessage_NextHand{NextHand: nextHandSetup}

	n.serverGame.QueueGameMessage(&message)
}

// messages sent from player to game
func (n *NatsGame) player2Game(msg *natsgo.Msg) {
	natsGameLogger.Debug().Uint64("game", n.gameID).
		Msg(fmt.Sprintf("Player->Game: %s", string(msg.Data)))
	// convert to protobuf message
	// convert json message to go message
	var message game.GameMessage
	//err := jsoniter.Unmarshal(msg.Data, &message)
	e := protojson.Unmarshal(msg.Data, &message)
	if e != nil {
		return
	}

	n.serverGame.QueueGameMessage(&message)
}

// messages sent from player to game hand
func (n *NatsGame) player2Hand(msg *natsgo.Msg) {
	natsGameLogger.Debug().Uint64("game", n.gameID).
		Msg(fmt.Sprintf("Player->Hand: %s", string(msg.Data)))
	var message game.HandMessage
	e := proto.Unmarshal(msg.Data, &message)
	if e != nil {
		return
	}

	messageHandled := false
	if len(message.Messages) >= 1 {
		message1 := message.Messages[0]
		if message1.MessageType == game.HandQueryCurrentHand {
			n.onQueryHand(n.gameID, message.PlayerId, message.MessageId)
			messageHandled = true
		}
	}
	if !messageHandled {
		n.serverGame.QueueHandMessage(&message)
	}
}

func (n *NatsGame) onQueryHand(gameID uint64, playerID uint64, messageID string) error {
	natsGameLogger.Debug().Uint64("game", n.gameID).
		Msgf("Player->Hand: Player [%d] Query current hand", playerID)
	err := n.serverGame.HandleQueryCurrentHand(playerID, messageID)
	natsGameLogger.Debug().Uint64("game", n.gameID).
		Msgf("Player->Hand: Query current hand [%d] returned", playerID)
	if err != nil {
		return err
	}
	return nil
}

// messages sent from player to pong channel for network check
func (n *NatsGame) player2Pong(msg *natsgo.Msg) {
	if util.Env.ShouldDebugConnectivityCheck() {
		natsGameLogger.Info().Uint64("game", n.gameID).
			Msg(fmt.Sprintf("Player->Pong: %s", string(msg.Data)))
	}
	var message game.PingPongMessage
	e := proto.Unmarshal(msg.Data, &message)
	if e != nil {
		return
	}

	n.serverGame.HandlePongMessage(&message)
}

func (n NatsGame) BroadcastGameMessage(message *game.GameMessage) {
	natsGameLogger.Debug().Uint64("game", n.gameID).
		Msg(fmt.Sprintf("Game->AllPlayers: %s", message.MessageType))
	// let send this to all players
	data, _ := protojson.Marshal(message)
	// fmt.Printf("%s\n", string(data))

	if message.GameCode != n.gameCode {
		// TODO: send to the other games
	} else if message.GameCode == n.gameCode {
		// fmt.Printf("%s\n", string(data))
		// if message.MessageType == game.GameCurrentStatus {
		// 	// update table status
		// 	UpdateTableStatus(message.GameId, message.GetStatus().GetTableStatus(), n.maxRetries, n.retryDelayMillis)
		// }
		n.natsConn.Publish(n.game2AllPlayersSubject, data)
	}
}

func (n NatsGame) BroadcastHandMessage(message *game.HandMessage) {
	message.PlayerId = 0

	marshaller := protojson.MarshalOptions{
		EmitUnpopulated: true,
	}
	jsonData, _ := marshaller.Marshal(message)
	var msgTypes []string
	for _, msgItem := range message.GetMessages() {
		msgTypes = append(msgTypes, msgItem.MessageType)
	}
	natsGameLogger.Debug().Uint64("game", n.gameID).Str("Messages", fmt.Sprintf("%v", msgTypes)).
		Str("subject", n.hand2AllPlayersSubject).
		Msg(fmt.Sprintf("H->A: %s", string(jsonData)))
	data, _ := proto.Marshal(message)
	n.natsConn.Publish(n.hand2AllPlayersSubject, data)
}

func (n NatsGame) BroadcastPingMessage(message *game.PingPongMessage) {
	jsonData, _ := protojson.Marshal(message)
	if util.Env.ShouldDebugConnectivityCheck() {
		natsGameLogger.Info().Uint64("game", n.gameID).
			Str("subject", n.pingSubject).
			Msg(fmt.Sprintf("Ping->All: %s", string(jsonData)))
	}
	data, _ := proto.Marshal(message)
	n.natsConn.Publish(n.pingSubject, data)
}

func (n NatsGame) SendHandMessageToPlayer(message *game.HandMessage, playerID uint64) {
	hand2PlayerSubject := fmt.Sprintf("hand.%s.player.%d", n.gameCode, playerID)
	message.PlayerId = playerID
	jsonData, _ := protojson.Marshal(message)
	data, _ := proto.Marshal(message)
	var msgTypes []string
	for _, msgItem := range message.GetMessages() {
		msgTypes = append(msgTypes, msgItem.MessageType)
	}
	natsGameLogger.Debug().Uint64("game", n.gameID).Str("Message", fmt.Sprintf("%v", msgTypes)).
		Str("subject", hand2PlayerSubject).
		Msg(fmt.Sprintf("H->P: %s", string(jsonData)))

	if util.Env.IsEncryptionEnabled() {
		encryptedData, err := n.serverGame.EncryptForPlayer(data, playerID)
		if err != nil {
			natsGameLogger.Error().Msgf("Unable to encrypt message to player %d", playerID)
			return
		}
		data = encryptedData
	}

	n.natsConn.Publish(hand2PlayerSubject, data)
}

func (n NatsGame) SendGameMessageToPlayer(message *game.GameMessage, playerID uint64) {
	natsGameLogger.Debug().Uint64("game", n.gameID).
		Msg(fmt.Sprintf("Game->Player: %s", message.MessageType))

	if playerID == 0 {
		data, _ := protojson.Marshal(message)
		n.chManageGame <- data
	} else {
		subject := fmt.Sprintf("game.%s.player.%d", n.gameCode, playerID)
		data, _ := protojson.Marshal(message)
		n.natsConn.Publish(subject, data)
	}
}

func (n *NatsGame) gameEnded() error {
	// // first send a message to all the players
	// message := &game.GameMessage{
	// 	GameId:      n.gameID,
	// 	GameCode:    n.gameCode,
	// 	MessageType: game.GameCurrentStatus,
	// }
	// message.GameMessage = &game.GameMessage_Status{Status: &game.GameStatusMessage{Status: game.GameStatus_ENDED,
	// 	TableStatus: game.TableStatus_WAITING_TO_BE_STARTED}}
	// natsGameLogger.Debug().Uint64("game", n.gameID).
	// 	Msg(fmt.Sprintf("Game->All: %s Game ENDED", message.MessageType))
	// n.BroadcastGameMessage(message)

	n.serverGame.GameEnded()
	return nil
}

func (n *NatsGame) getHandLog() *map[string]interface{} {
	natsGameLogger.Debug().Uint64("game", n.gameID).
		Msg(fmt.Sprintf("APIServer->Game: Get HAND LOG: %d", n.gameID))
	// build a game message and send to the game
	var message game.GameMessage

	message.GameId = n.gameID
	message.GameCode = n.gameCode
	message.MessageType = game.GetHandLog

	n.serverGame.QueueGameMessage(&message)
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
