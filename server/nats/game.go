package nats

import (
	"encoding/json"
	"fmt"

	natsgo "github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"voyager.com/logging"
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
	logger       *zerolog.Logger
	gameID       uint64
	gameCode     string
	tournamentID uint64
	tableNo      uint32

	chEndGame    chan bool
	chManageGame chan []byte

	hand2AllPlayersSubject string
	game2AllPlayersSubject string

	player2HandSubscription *natsgo.Subscription
	clientAliveSubscription *natsgo.Subscription
	natsConn                *natsgo.Conn

	maxRetries       uint32
	retryDelayMillis uint32

	serverGame *game.Game
}

func newNatsGame(nc *natsgo.Conn, gameID uint64, gameCode string, tournamentID uint64, tableNo uint32) (*NatsGame, error) {
	logger := logging.GetZeroLogger("nats::NatsGame", nil).With().
		Uint64(logging.GameIDKey, gameID).
		Str(logging.GameCodeKey, gameCode).
		Logger()

	// game subjects
	game2AllPlayersSubject := GetGame2AllPlayerSubject(gameCode)

	// hand subjects
	player2HandSubject := GetPlayer2HandSubject(gameCode)
	hand2AllPlayersSubject := GetHand2AllPlayerSubject(gameCode)

	// we need to use the API to get the game configuration
	natsGame := &NatsGame{
		logger:                 &logger,
		tournamentID:           tournamentID,
		tableNo:                tableNo,
		gameID:                 gameID,
		gameCode:               gameCode,
		chEndGame:              make(chan bool),
		chManageGame:           make(chan []byte),
		game2AllPlayersSubject: game2AllPlayersSubject,
		hand2AllPlayersSubject: hand2AllPlayersSubject,
		natsConn:               nc,
		maxRetries:             10,
		retryDelayMillis:       1500,
	}

	// subscribe to topics
	var e error
	natsGame.player2HandSubscription, e = nc.Subscribe(player2HandSubject, natsGame.player2Hand)
	if e != nil {
		return nil, errors.Wrapf(e, "Failed to subscribe to %s", player2HandSubject)
	}

	// for receiving ping response
	clientAliveSubject := GetClientAliveSubject(gameCode)
	natsGame.clientAliveSubscription, e = nc.Subscribe(clientAliveSubject, natsGame.clientAlive)
	if e != nil {
		return nil, errors.Wrapf(e, "Failed to subscribe to %s", clientAliveSubject)
	}

	serverGame, _, err := game.GameManager.InitializeGame(natsGame, gameID, gameCode, tournamentID, tableNo)
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
	err := n.player2HandSubscription.Unsubscribe()
	if err != nil {
		n.logger.Warn().
			Str(logging.NatsSubjectKey, n.player2HandSubscription.Subject).
			Msgf("Could not unsubscribe player->hand subject during cleanup")
	}
	err = n.clientAliveSubscription.Unsubscribe()
	if err != nil {
		n.logger.Warn().
			Str(logging.NatsSubjectKey, n.clientAliveSubscription.Subject).
			Msgf("Could not unsubscribe client liveness subject during cleanup")
	}
}

func (n *NatsGame) resumeGame() {
	n.logger.Debug().
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
	n.logger.Debug().
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

func (n *NatsGame) HandlePlayerMovedTable(gameCode string, tournamentID uint32, oldTableNo uint32, newTableNo uint32, newSeatNo uint32, playerID uint64, gameInfo string) error {
	err := n.serverGame.HandlePlayerMovedTable(gameCode, tournamentID, oldTableNo, newTableNo, newSeatNo, playerID, gameInfo)
	if err != nil {
		return err
	}

	return nil
}

// messages sent from player to game hand
func (n *NatsGame) player2Hand(msg *natsgo.Msg) {
	var message game.HandMessage
	e := proto.Unmarshal(msg.Data, &message)
	logLevel := n.logger.GetLevel()
	if logLevel == zerolog.DebugLevel || logLevel == zerolog.TraceLevel {
		b, err := protojson.Marshal(&message)
		if err != nil {
			n.logger.Warn().Err(err).Msgf("Could not protojson-marshal hand message from player for logging")
		} else {
			n.logger.Debug().
				Msg(fmt.Sprintf("Player->Hand: %v", string(b)))
		}
	}
	if e != nil {
		n.logger.Error().Err(e).
			Msg("Could not proto-unmarshal hand message from player")
		return
	}

	messageHandled := false
	if len(message.Messages) >= 1 {
		message1 := message.Messages[0]
		if message1.MessageType == game.HandQueryCurrentHand {
			err := n.onQueryHand(n.gameID, message.PlayerId, message.MessageId)
			if err != nil {
				n.logger.Error().Err(err).
					Uint64(logging.PlayerIDKey, message.PlayerId).
					Msg("Could not handle query current hand")
			}
			messageHandled = true
		}
	}
	if !messageHandled {
		n.serverGame.QueueHandMessage(&message)
	}
}

func (n *NatsGame) onQueryHand(gameID uint64, playerID uint64, messageID string) error {
	n.logger.Debug().
		Msgf("Player->Hand: Player [%d] Query current hand", playerID)
	err := n.serverGame.HandleQueryCurrentHand(playerID, messageID)
	n.logger.Debug().
		Msgf("Player->Hand: Query current hand [%d] returned", playerID)
	if err != nil {
		n.logger.Error().Err(err).
			Uint64(logging.PlayerIDKey, playerID).
			Msgf("Could not handle query current hand for player")
		return err
	}
	return nil
}

// messages sent from client to client liveness channel for notifying liveness
func (n *NatsGame) clientAlive(msg *natsgo.Msg) {
	var message game.ClientAliveMessage
	err := proto.Unmarshal(msg.Data, &message)
	if err != nil {
		n.logger.Error().Err(err).Msg("Could not proto-unmarshal clientAlive message from player")
		return
	}

	if message.GameCode != n.gameCode {
		n.logger.Error().Msgf("Discarding client alive message. Unexpected game code. Game ID: %d, game code: %s", message.GameId, message.GameCode)
		return
	}

	n.serverGame.HandleAliveMessage(&message)
}

func (n NatsGame) BroadcastGameMessage(message *game.GameMessage, noLog bool) {
	if !noLog {
		n.logger.Debug().
			Msg(fmt.Sprintf("Game->AllPlayers: %s", message.MessageType))
	}
	// let send this to all players
	data, err := protojson.Marshal(message)
	if err != nil {
		n.logger.Error().Err(err).Msg("Could not protojson-marshal game message")
		return
	}

	if message.GameCode != n.gameCode {
		n.logger.Warn().Msgf("BroadcastGameMessage called with message that contains wrong game code. Message game code: %s, NatsGame.gameCode: %s", message.GameCode, n.gameCode)
		return
	}

	err = n.natsConn.Publish(n.game2AllPlayersSubject, data)
	if err != nil {
		n.logger.Error().Err(err).Msg("Could not publish game message")
	}
}

func (n NatsGame) BroadcastHandMessage(message *game.HandMessage) {
	message.PlayerId = 0

	marshaller := protojson.MarshalOptions{
		EmitUnpopulated: true,
	}
	jsonData, err := marshaller.Marshal(message)
	if err != nil {
		n.logger.Warn().Msg("Could not protojson-marshal hand message for logging")
	} else {
		var msgTypes []string
		for _, msgItem := range message.GetMessages() {
			msgTypes = append(msgTypes, msgItem.MessageType)
		}
		n.logger.Debug().Str("Messages", fmt.Sprintf("%v", msgTypes)).
			Str(logging.NatsSubjectKey, n.hand2AllPlayersSubject).
			Msg(fmt.Sprintf("H->A: %s", string(jsonData)))
	}
	data, err := proto.Marshal(message)
	if err != nil {
		n.logger.Error().Err(err).Msg("Could not proto-marshal hand message")
		return
	}
	err = n.natsConn.Publish(n.hand2AllPlayersSubject, data)
	if err != nil {
		n.logger.Error().Err(err).Msg("Could not publish hand message")
	}
}

func (n NatsGame) SendHandMessageToPlayer(message *game.HandMessage, playerID uint64) {
	hand2PlayerSubject := GetHand2PlayerSubject(n.gameCode, playerID)
	message.PlayerId = playerID
	jsonData, err := protojson.Marshal(message)
	if err != nil {
		n.logger.Warn().Msg("Could not protojson-marshal hand message to player for logging")
	} else {
		var msgTypes []string
		for _, msgItem := range message.GetMessages() {
			msgTypes = append(msgTypes, msgItem.MessageType)
		}
		n.logger.Debug().Str("Message", fmt.Sprintf("%v", msgTypes)).
			Str(logging.NatsSubjectKey, hand2PlayerSubject).
			Msg(fmt.Sprintf("H->P: %s", string(jsonData)))
	}

	data, err := proto.Marshal(message)
	if err != nil {
		n.logger.Error().Err(err).Msgf("Could not pro-marshal hand message to player %d", playerID)
		return
	}

	if util.Env.IsEncryptionEnabled() {
		encryptedData, err := n.serverGame.EncryptForPlayer(data, playerID)
		if err != nil {
			n.logger.Error().Msgf("Unable to encrypt message to player %d", playerID)
			return
		}
		data = encryptedData
	}

	err = n.natsConn.Publish(hand2PlayerSubject, data)
	if err != nil {
		n.logger.Error().Err(err).Msgf("Could not publish hand message to player %d", playerID)
	}
}

func (n NatsGame) SendHandMessageToTournamentPlayer(message *game.HandMessage, tournamentID uint32, playerID uint64) {
	tournamentPlayerSubject := GetTournamentPlayerSubject(tournamentID, playerID)
	message.PlayerId = playerID
	jsonData, err := protojson.Marshal(message)
	if err != nil {
		n.logger.Warn().Msg("Could not protojson-marshal tournament message to player for logging")
	} else {
		var msgTypes []string
		for _, msgItem := range message.GetMessages() {
			msgTypes = append(msgTypes, msgItem.MessageType)
		}
		n.logger.Debug().Str("Message", fmt.Sprintf("%v", msgTypes)).
			Str(logging.NatsSubjectKey, tournamentPlayerSubject).
			Msg(fmt.Sprintf("H->TP: %s", string(jsonData)))
	}

	data, err := proto.Marshal(message)
	if err != nil {
		n.logger.Error().Err(err).Msgf("Could not proto-marshal tournament message to player %d", playerID)
		return
	}

	err = n.natsConn.Publish(tournamentPlayerSubject, data)
	if err != nil {
		n.logger.Error().Err(err).Msgf("Could not publish tournament message to player %d", playerID)
	}
}

func (n NatsGame) SendGameMessageToPlayer(message *game.GameMessage, playerID uint64) {
	n.logger.Debug().
		Msg(fmt.Sprintf("Game->Player: %s", message.MessageType))

	if playerID == 0 {
		data, err := protojson.Marshal(message)
		if err != nil {
			n.logger.Error().Err(err).Msg("Could not protojson-marshal game message to chManageGame")
		}
		n.chManageGame <- data
	} else {
		subject := GetGame2PlayerSubject(n.gameCode, playerID)
		data, err := protojson.Marshal(message)
		if err != nil {
			n.logger.Error().Err(err).
				Uint64(logging.PlayerIDKey, playerID).
				Msg("Could not protojson-marshal game message to player")
			return
		}
		err = n.natsConn.Publish(subject, data)
		if err != nil {
			n.logger.Error().Err(err).
				Str(logging.NatsSubjectKey, subject).
				Msg("Could not publish game message to player")
		}
	}
}

func (n *NatsGame) gameEnded() error {
	err := n.serverGame.GameEnded()
	if err != nil {
		return err
	}
	return nil
}

func (n *NatsGame) getHandLog() *map[string]interface{} {
	n.logger.Debug().
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
