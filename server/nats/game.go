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
	clubID   uint32
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

	serverGame *game.Game
}

func newNatsGame(nc *natsgo.Conn, clubID uint32, gameID uint64, config *game.GameConfig) (*NatsGame, error) {

	// game subjects
	game2AllPlayersSubject := GetGame2AllPlayerSubject(config.GameCode)

	// hand subjects
	player2HandSubject := GetPlayer2HandSubject(config.GameCode)
	hand2AllPlayersSubject := GetHand2AllPlayerSubject(config.GameCode)

	// we need to use the API to get the game configuration
	natsGame := &NatsGame{
		clubID:                 clubID,
		gameID:                 gameID,
		gameCode:               config.GameCode,
		chEndGame:              make(chan bool),
		chManageGame:           make(chan []byte),
		game2AllPlayersSubject: game2AllPlayersSubject,
		hand2AllPlayersSubject: hand2AllPlayersSubject,
		pingSubject:            GetPingSubject(config.GameCode),
		natsConn:               nc,
	}

	// subscribe to topics
	var e error
	natsGame.player2HandSubscription, e = nc.Subscribe(player2HandSubject, natsGame.player2Hand)
	if e != nil {
		natsLogger.Error().Msg(fmt.Sprintf("Failed to subscribe to %s", player2HandSubject))
		return nil, e
	}

	// for receiving ping response
	playerPongSubject := GetPongSubject(config.GameCode)
	natsGame.pongSubscription, e = nc.Subscribe(playerPongSubject, natsGame.player2Pong)
	if e != nil {
		natsLogger.Error().Msg(fmt.Sprintf("Failed to subscribe to %s", playerPongSubject))
		return nil, e
	}

	if config.ActionTime == 0 {
		config.ActionTime = 20
	}

	serverGame, gameID, err := game.GameManager.InitializeGame(natsGame, config)
	if err != nil {
		return nil, err
	}
	natsGame.serverGame = serverGame
	natsGame.serverGame.GameStarted()
	return natsGame, nil
}

func (n *NatsGame) cleanup() {
	n.player2HandSubscription.Unsubscribe()
	n.pongSubscription.Unsubscribe()
}

// message sent from apiserver to game
func (n *NatsGame) gameStatusChanged(gameID uint64, newStatus GameStatus) {
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
		Msg(fmt.Sprintf("APIServer->Game: Status changed. GameID: %d, NewStatus: %s", gameID, game.GameStatus_name[int32(newStatus.GameStatus)]))

	var statusChangeMessage game.GameMessage
	statusChangeMessage.GameId = gameID
	statusChangeMessage.MessageType = game.GameStatusChanged
	statusChangeMessage.GameMessage = &game.GameMessage_StatusChange{StatusChange: &game.GameStatusChangeMessage{NewStatus: newStatus.GameStatus}}

	n.serverGame.QueueGameMessage(&statusChangeMessage)
	n.BroadcastGameMessage(&statusChangeMessage)

	var message game.GameMessage
	message.GameId = gameID
	message.GameCode = n.gameCode
	message.MessageType = game.GameCurrentStatus
	message.GameMessage = &game.GameMessage_Status{Status: &game.GameStatusMessage{Status: newStatus.GameStatus, TableStatus: newStatus.TableStatus}}

	//n.serverGame.SendGameMessageToChannel(&message)
	n.BroadcastGameMessage(&message)
}

// message sent from apiserver to game
func (n *NatsGame) playerUpdate(gameID uint64, update *PlayerUpdate) {
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
		Msg(fmt.Sprintf("APIServer->Game: Player update. GameID: %d, PlayerId: %d NewStatus: %s",
			gameID, update.PlayerId, game.PlayerStatus_name[int32(update.Status)]))
	var message game.GameMessage
	message.GameId = gameID
	message.GameCode = n.gameCode
	message.MessageType = game.PlayerUpdate
	playerUpdate := game.GamePlayerUpdate{
		PlayerId:  update.PlayerId,
		SeatNo:    uint32(update.SeatNo),
		Status:    update.Status,
		Stack:     float32(update.Stack),
		BuyIn:     float32(update.BuyIn),
		GameToken: update.GameToken,
		OldSeat:   uint32(update.OldSeatNo),
		NewUpdate: game.NewUpdate(update.NewUpdate),
	}

	message.GameMessage = &game.GameMessage_PlayerUpdate{PlayerUpdate: &playerUpdate}

	n.BroadcastGameMessage(&message)
}

func (n *NatsGame) pendingUpdatesDone(gameStatus game.GameStatus, tableStatus game.TableStatus) {
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
		Msg(fmt.Sprintf("APIServer->Game: Pending updates done. GameID: %d GameStatus: %d Table Status: %d", n.gameID, gameStatus, tableStatus))

	status := &game.GameStatusMessage{Status: gameStatus, TableStatus: tableStatus}
	message := game.GameMessage{
		GameId:      n.gameID,
		GameCode:    n.gameCode,
		MessageType: game.GameCurrentStatus,
		GameMessage: &game.GameMessage_Status{Status: status},
	}
	n.serverGame.QueueGameMessage(&message)

	message2 := game.GameMessage{
		GameId:      n.gameID,
		GameCode:    n.gameCode,
		MessageType: game.GamePendingUpdatesDone,
	}
	n.serverGame.QueueGameMessage(&message2)
}

// message sent from bot to game
func (n *NatsGame) setupHand(handSetup HandSetup) {
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
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

	message.ClubId = 0
	message.GameId = n.gameID
	message.GameCode = n.gameCode
	message.MessageType = game.GameSetupNextHand
	message.GameMessage = &game.GameMessage_NextHand{NextHand: nextHandSetup}

	n.serverGame.QueueGameMessage(&message)
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

	n.serverGame.QueueGameMessage(&message)
}

// messages sent from player to game hand
func (n *NatsGame) player2Hand(msg *natsgo.Msg) {
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
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
	natsLogger.Info().Uint64("game", n.gameID).
		Msgf("Player->Hand: Player [%d] Query current hand", playerID)
	err := n.serverGame.HandleQueryCurrentHand(playerID, messageID)
	natsLogger.Info().Uint64("game", n.gameID).
		Msgf("Player->Hand: Query current hand [%d] returned", playerID)
	if err != nil {
		return err
	}
	return nil
}

// messages sent from player to pong channel for network check
func (n *NatsGame) player2Pong(msg *natsgo.Msg) {
	if util.Env.ShouldDebugConnectivityCheck() {
		natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
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
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
		Msg(fmt.Sprintf("Game->AllPlayers: %s", message.MessageType))
	// let send this to all players
	data, _ := protojson.Marshal(message)
	fmt.Printf("%s\n", string(data))

	if message.GameCode != n.gameCode {
		// TODO: send to the other games
	} else if message.GameCode == n.gameCode {
		fmt.Printf("%s\n", string(data))
		if message.MessageType == game.GameCurrentStatus {
			// update table status
			UpdateTableStatus(message.GameId, message.GetStatus().GetTableStatus())
		}
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
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).Str("Messages", fmt.Sprintf("%v", msgTypes)).
		Str("subject", n.hand2AllPlayersSubject).
		Msg(fmt.Sprintf("H->A: %s", string(jsonData)))
	data, _ := proto.Marshal(message)
	n.natsConn.Publish(n.hand2AllPlayersSubject, data)
}

func (n NatsGame) BroadcastPingMessage(message *game.PingPongMessage) {
	jsonData, _ := protojson.Marshal(message)
	if util.Env.ShouldDebugConnectivityCheck() {
		natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
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
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).Str("Message", fmt.Sprintf("%v", msgTypes)).
		Str("subject", hand2PlayerSubject).
		Msg(fmt.Sprintf("H->P: %s", string(jsonData)))

	if util.Env.IsEncryptionEnabled() {
		encryptedData, err := n.serverGame.EncryptForPlayer(data, playerID)
		if err != nil {
			natsLogger.Error().Msgf("Unable to encrypt message to player %d", playerID)
			return
		}
		data = encryptedData
	}

	n.natsConn.Publish(hand2PlayerSubject, data)
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
		n.natsConn.Publish(subject, data)
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
		Msg(fmt.Sprintf("APIServer->Game: Get HAND LOG: %d", n.gameID))
	// build a game message and send to the game
	var message game.GameMessage

	message.ClubId = 0
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

func (n *NatsGame) tableUpdate(gameID uint64, update *TableUpdate) {
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
		Msg(fmt.Sprintf("APIServer->Game: Table update. GameID: %d, Type: %s",
			gameID, update.Type))
	var message game.GameMessage
	message.GameId = gameID
	message.GameCode = n.gameCode
	message.MessageType = game.GameTableUpdate
	tableUpdate := game.TableUpdate{}
	tableUpdate.Type = update.Type
	if update.Type == game.TableSeatChangeProcess {
		tableUpdate.SeatChangeTime = update.SeatChangeRemainingTime
		tableUpdate.SeatChangePlayers = update.SeatChangePlayers
		tableUpdate.SeatChangeSeatNo = update.SeatChangeSeatNos
	} else if update.Type == game.TableWaitlistSeating {
		tableUpdate.WaitlistPlayerId = update.WaitlistPlayerId
		tableUpdate.WaitlistPlayerName = update.WaitlistPlayerName
		tableUpdate.WaitlistPlayerUuid = update.WaitlistPlayerUuid
		tableUpdate.WaitlistRemainingTime = update.WaitlistRemainingTime
	} else if update.Type == game.TableHostSeatChangeMove {
		tableUpdate.SeatMoves = make([]*game.SeatMove, len(update.SeatMoves))
		for i, move := range update.SeatMoves {
			tableUpdate.SeatMoves[i] = &game.SeatMove{
				PlayerId:   move.PlayerId,
				PlayerUuid: move.PlayerUuid,
				Name:       move.Name,
				OldSeatNo:  move.OldSeatNo,
				NewSeatNo:  move.NewSeatNo,
				Stack:      float32(move.Stack),
			}
		}
		natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
			Msgf("APIServer->Game: SeatMove. GameID: %d, Code: %s, Moves: +%v",
				gameID, n.gameCode, update.SeatMoves)
	} else if update.Type == game.TableHostSeatChangeProcessStart {
		tableUpdate.SeatChangeHost = update.SeatChangeHostId
	} else if update.Type == game.TableHostSeatChangeProcessEnd {
		tableUpdate.SeatChangeHost = update.SeatChangeHostId
		tableUpdate.SeatUpdates = make([]*game.SeatUpdate, len(update.SeatUpdates))
		for i, update := range update.SeatUpdates {
			tableUpdate.SeatUpdates[i] = &game.SeatUpdate{
				SeatNo:       update.SeatNo,
				PlayerId:     update.PlayerId,
				PlayerUuid:   update.PlayerUuid,
				Name:         update.Name,
				Stack:        float32(update.Stack),
				PlayerStatus: update.PlayerStatus,
				OpenSeat:     update.OpenSeat,
			}
		}
	}
	message.GameMessage = &game.GameMessage_TableUpdate{TableUpdate: &tableUpdate}
	// send the message to the players
	go n.BroadcastGameMessage(&message)
}

func (n *NatsGame) playerConfigUpdate(update *PlayerConfigUpdate) error {
	// first send a message to all the players
	message := &game.GameMessage{
		GameId:      n.gameID,
		GameCode:    n.gameCode,
		MessageType: game.PlayerConfigUpdateMsg,
	}

	message.GameMessage = &game.GameMessage_PlayerConfigUpdate{
		PlayerConfigUpdate: &game.PlayerConfigUpdate{
			PlayerId:         update.PlayerId,
			MuckLosingHand:   update.MuckLosingHand,
			RunItTwicePrompt: update.RunItTwicePrompt,
		},
	}
	n.serverGame.QueueGameMessage(message)
	return nil
}
