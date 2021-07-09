package nats

import (
	"encoding/json"
	"fmt"

	natsgo "github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/encoding/protojson"
	"voyager.com/server/game"
	"voyager.com/server/poker"
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
	clubID                 uint32
	gameID                 uint64
	chEndGame              chan bool
	chManageGame           chan []byte
	player2GameSubject     string
	player2HandSubject     string
	playerPongSubject      string
	hand2PlayerAllSubject  string
	game2AllPlayersSubject string

	serverGame *game.Game

	gameCode       string
	player2GameSub *natsgo.Subscription
	player2HandSub *natsgo.Subscription
	player2PongSub *natsgo.Subscription
	nc             *natsgo.Conn
}

func newNatsGame(nc *natsgo.Conn, clubID uint32, gameID uint64, config *game.GameConfig) (*NatsGame, error) {

	// game subjects
	//player2GameSubject := fmt.Sprintf("game.%d.player", gameID)
	game2AllPlayersSubject := fmt.Sprintf("game.%s.player", config.GameCode)

	// hand subjects
	player2HandSubject := fmt.Sprintf("player.%s.hand", config.GameCode)
	hand2AllPlayersSubject := fmt.Sprintf("hand.%s.player.all", config.GameCode)

	// for receiving ping response
	playerPongSubject := fmt.Sprintf("pong.%s", config.GameCode)

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
		playerPongSubject:      playerPongSubject,
		gameCode:               config.GameCode,
	}

	// subscribe to topics
	var e error
	natsGame.player2HandSub, e = nc.Subscribe(player2HandSubject, natsGame.player2Hand)
	if e != nil {
		natsLogger.Error().Msg(fmt.Sprintf("Failed to subscribe to %s", player2HandSubject))
		return nil, e
	}
	natsGame.player2PongSub, e = nc.Subscribe(playerPongSubject, natsGame.player2Pong)
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
	n.player2HandSub.Unsubscribe()
	n.player2GameSub.Unsubscribe()
	n.player2PongSub.Unsubscribe()
}

// message sent from apiserver to game
func (n *NatsGame) gameStatusChanged(gameID uint64, newStatus GameStatus) {
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
		Msg(fmt.Sprintf("APIServer->Game: Status changed. GameID: %d, NewStatus: %s", gameID, game.GameStatus_name[int32(newStatus.GameStatus)]))

	var statusChangeMessage game.GameMessage
	statusChangeMessage.GameId = gameID
	statusChangeMessage.MessageType = game.GameStatusChanged
	statusChangeMessage.GameMessage = &game.GameMessage_StatusChange{StatusChange: &game.GameStatusChangeMessage{NewStatus: newStatus.GameStatus}}

	n.serverGame.SendGameMessageToChannel(&statusChangeMessage)
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
	go func() {
		var message game.GameMessage
		message.GameId = n.gameID
		message.GameCode = n.gameCode
		message.GameId = n.gameID
		message.GameCode = n.gameCode
		message.MessageType = game.GameCurrentStatus

		status := &game.GameStatusMessage{Status: gameStatus, TableStatus: tableStatus}
		message.GameMessage = &game.GameMessage_Status{Status: status}
		n.serverGame.SendGameMessageToChannel(&message)
		message.MessageType = game.GamePendingUpdatesDone
		message.GameMessage = nil
		n.serverGame.SendGameMessageToChannel(&message)
	}()
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
		ButtonPos:         handSetup.ButtonPos,
		Board:             handSetup.Board,
		Board2:            handSetup.Board2,
		Flop:              handSetup.Flop,
		Turn:              handSetup.Turn,
		River:             handSetup.River,
		PlayerCards:       playerCards,
		PlayerCardsBySeat: playerCardsBySeat,
		Pause:             handSetup.Pause,
	}

	message.ClubId = 0
	message.GameId = n.gameID
	message.GameCode = n.gameCode
	message.MessageType = game.GameSetupNextHand
	message.GameMessage = &game.GameMessage_NextHand{NextHand: nextHandSetup}

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

	messageHandled := false
	if len(message.Messages) > 1 {
		message1 := message.Messages[0]
		if message1.MessageType == game.HandQueryCurrentHand {
			n.onQueryHand(message.GameId, message.PlayerId, message.MessageId, message1)
			messageHandled = true
		}
	}
	if !messageHandled {
		n.serverGame.SendHandMessage(&message)
	}
}

func (n *NatsGame) onQueryHand(gameId uint64, playerId uint64, messageId string, message *game.HandMessageItem) error {
	natsLogger.Info().Uint64("game", n.gameID).
		Msgf("Player->Hand: Query current hand [%d]", playerId)

	handState, err := n.serverGame.GetHandState()
	if err != nil || handState == nil ||
		handState.HandNum == 0 ||
		handState.CurrentState == game.HandStatus_HAND_CLOSED {
		natsLogger.Info().Uint64("game", n.gameID).
			Msgf("Player->Hand: Query current hand [%d] handstate is not available", playerId)
		currentHandState := game.CurrentHandState{
			HandNum: 0,
		}
		handStateMsg := &game.HandMessageItem{
			MessageType: game.HandQueryCurrentHand,
			Content:     &game.HandMessageItem_CurrentHandState{CurrentHandState: &currentHandState},
		}

		serverMsg := &game.HandMessage{
			GameId:    n.gameID,
			PlayerId:  playerId,
			HandNum:   0,
			MessageId: n.serverGame.GenerateMsgID("CURRENT_HAND", handState.HandNum, handState.CurrentState, playerId, messageId, handState.CurrentActionNum),
			Messages:  []*game.HandMessageItem{handStateMsg},
		}
		n.SendHandMessageToPlayer(serverMsg, playerId)
		return nil
	}

	boardCards := make([]uint32, len(handState.BoardCards))
	for i, card := range handState.BoardCards {
		boardCards[i] = uint32(card)
	}

	pots := make([]float32, 0)
	for _, pot := range handState.Pots {
		pots = append(pots, pot.Pot)
	}
	currentPot := pots[len(pots)-1]
	bettingInProgress := handState.CurrentState == game.HandStatus_PREFLOP ||
		handState.CurrentState == game.HandStatus_FLOP ||
		handState.CurrentState == game.HandStatus_TURN ||
		handState.CurrentState == game.HandStatus_RIVER
	if bettingInProgress {
		currentRoundState, ok := handState.RoundState[uint32(handState.CurrentState)]
		if !ok || currentRoundState == nil {
			b, err := json.Marshal(handState)
			if err != nil {
				return fmt.Errorf("Unable to find current round state. currentRoundState: %+v. handState.CurrentState: %d handState.RoundState: %+v", currentRoundState, handState.CurrentState, handState.RoundState)
			}
			return fmt.Errorf("Unable to find current round state. handState: %s", string(b))
		}
		currentBettingRound := currentRoundState.Betting
		for _, bet := range currentBettingRound.SeatBet {
			currentPot = currentPot + bet
		}
	}

	var boardCardsOut []uint32
	switch handState.CurrentState {
	case game.HandStatus_FLOP:
		boardCardsOut = boardCards[:3]
	case game.HandStatus_TURN:
		boardCardsOut = boardCards[:4]

	case game.HandStatus_RIVER:
	case game.HandStatus_RESULT:
	case game.HandStatus_SHOW_DOWN:
		boardCardsOut = boardCards

	default:
		boardCardsOut = make([]uint32, 0)
	}
	cardsStr := poker.CardsToString(boardCardsOut)

	currentHandState := game.CurrentHandState{
		HandNum:       handState.HandNum,
		GameType:      handState.GameType,
		CurrentRound:  handState.CurrentState,
		BoardCards:    boardCardsOut,
		BoardCards_2:  nil,
		CardsStr:      cardsStr,
		Pots:          pots,
		PotUpdates:    currentPot,
		ButtonPos:     handState.ButtonPos,
		SmallBlindPos: handState.SmallBlindPos,
		BigBlindPos:   handState.BigBlindPos,
		SmallBlind:    handState.SmallBlind,
		BigBlind:      handState.BigBlind,
		NoCards:       n.serverGame.NumCards(handState.GameType),
	}
	currentHandState.PlayersActed = make(map[uint32]*game.PlayerActRound, 0)

	var playerSeatNo uint32
	for seatNo, pid := range handState.GetPlayersInSeats() {
		if pid == playerId {
			playerSeatNo = uint32(seatNo)
			break
		}
	}

	for seatNo, action := range handState.GetPlayersActed() {
		if action.State == game.PlayerActState_PLAYER_ACT_EMPTY_SEAT {
			continue
		}
		currentHandState.PlayersActed[uint32(seatNo)] = action
	}

	if playerSeatNo != 0 {
		player := n.serverGame.PlayersInSeats[playerSeatNo]
		_, maskedCards := n.serverGame.MaskCards(handState.GetPlayersCards()[playerSeatNo], player.GameTokenInt)
		currentHandState.PlayerCards = fmt.Sprintf("%d", maskedCards)
		currentHandState.PlayerSeatNo = playerSeatNo
	}

	if bettingInProgress && handState.NextSeatAction != nil {
		currentHandState.NextSeatToAct = handState.NextSeatAction.SeatNo
		currentHandState.RemainingActionTime = n.serverGame.GetRemainingActionTime()
		currentHandState.NextSeatAction = handState.NextSeatAction
	}
	currentHandState.PlayersStack = make(map[uint64]float32, 0)
	playerState := handState.GetPlayersState()
	for seatNo, playerID := range handState.GetPlayersInSeats() {
		if playerID == 0 {
			continue
		}
		currentHandState.PlayersStack[uint64(seatNo)] = playerState[playerID].Stack
	}

	handStateMsg := &game.HandMessageItem{
		MessageType: game.HandQueryCurrentHand,
		Content:     &game.HandMessageItem_CurrentHandState{CurrentHandState: &currentHandState},
	}

	serverMsg := &game.HandMessage{
		GameId:     gameId,
		PlayerId:   playerId,
		HandNum:    handState.HandNum,
		HandStatus: handState.CurrentState,
		MessageId: n.serverGame.GenerateMsgID("CURRENT_HAND", handState.HandNum,
			handState.CurrentState, playerId, messageId, handState.CurrentActionNum),
		Messages: []*game.HandMessageItem{handStateMsg},
	}
	n.SendHandMessageToPlayer(serverMsg, playerId)
	natsLogger.Info().Uint64("game", n.gameID).
		Msgf("Player->Hand: Query current hand [%d] returned", playerId)

	return nil
}

// messages sent from player to pong channel for network check
func (n *NatsGame) player2Pong(msg *natsgo.Msg) {
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
		Msg(fmt.Sprintf("Player->Pong: %s", string(msg.Data)))
	var message game.PingPongMessage
	e := protojson.Unmarshal(msg.Data, &message)
	if e != nil {
		return
	}

	n.serverGame.SendPongMessage(&message)
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
		n.nc.Publish(n.game2AllPlayersSubject, data)
	}
}

func (n NatsGame) BroadcastHandMessage(message *game.HandMessage) {
	message.PlayerId = 0

	marshaller := protojson.MarshalOptions{
		EmitUnpopulated: true,
	}
	data, _ := marshaller.Marshal(message)
	var msgTypes []string
	for _, msgItem := range message.GetMessages() {
		msgTypes = append(msgTypes, msgItem.MessageType)
	}
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).Str("Messages", fmt.Sprintf("%v", msgTypes)).
		Str("subject", n.hand2PlayerAllSubject).
		Msg(fmt.Sprintf("H->A: %s", string(data)))
	n.nc.Publish(n.hand2PlayerAllSubject, data)
}

func (n NatsGame) BroadcastPingMessage(message *game.PingPongMessage) {
	pingSubject := fmt.Sprintf("ping.%s", n.gameCode)
	data, _ := protojson.Marshal(message)
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
		Str("subject", pingSubject).
		Msg(fmt.Sprintf("Ping->All: %s", string(data)))
	n.nc.Publish(pingSubject, data)
}

func (n NatsGame) SendHandMessageToPlayer(message *game.HandMessage, playerID uint64) {
	hand2PlayerSubject := fmt.Sprintf("hand.%s.player.%d", n.gameCode, playerID)
	message.PlayerId = playerID
	data, _ := protojson.Marshal(message)
	var msgTypes []string
	for _, msgItem := range message.GetMessages() {
		msgTypes = append(msgTypes, msgItem.MessageType)
	}
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).Str("Message", fmt.Sprintf("%v", msgTypes)).
		Str("subject", hand2PlayerSubject).
		Msg(fmt.Sprintf("H->P: %s", string(data)))

	if util.GameServerEnvironment.IsEncryptionEnabled() {
		encryptedData, err := n.serverGame.EncryptForPlayer(data, playerID)
		if err != nil {
			natsLogger.Error().Msgf("Unable to encrypt message to player %d", playerID)
			return
		}
		data = encryptedData
	}

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
	message.GameCode = n.gameCode
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
	n.serverGame.SendGameMessageToChannel(message)
	return nil
}
