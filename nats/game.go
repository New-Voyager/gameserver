package nats

import (
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
	player2GameSubject     string
	player2HandSubject     string
	hand2PlayerAllSubject  string
	game2AllPlayersSubject string

	serverGame *game.Game

	player2GameSub *natsgo.Subscription
	player2HandSub *natsgo.Subscription
	nc             *natsgo.Conn
}

func newNatsGame(nc *natsgo.Conn, clubID uint32, gameID uint64, config *game.GameConfig) (*NatsGame, error) {

	// game subjects
	player2GameSubject := fmt.Sprintf("game.%d.player", gameID)
	game2AllPlayersSubject := fmt.Sprintf("game.%d.allplayers", gameID)

	// hand subjects
	player2HandSubject := fmt.Sprintf("game.%d.hand.player", gameID)
	hand2PlayerAllSubject := fmt.Sprintf("game.%d.hand.all", gameID)

	// we need to use the API to get the game configuration
	natsGame := &NatsGame{
		clubID:                 clubID,
		gameID:                 gameID,
		chEndGame:              make(chan bool),
		nc:                     nc,
		player2GameSubject:     player2GameSubject,
		game2AllPlayersSubject: game2AllPlayersSubject,
		player2HandSubject:     player2HandSubject,
		hand2PlayerAllSubject:  hand2PlayerAllSubject,
	}

	// subscribe to topics
	var e error
	natsGame.player2HandSub, e = nc.Subscribe(player2HandSubject, natsGame.player2Hand)
	if e != nil {
		natsLogger.Error().Msg(fmt.Sprintf("Failed to subscribe to %s", player2HandSubject))
		return nil, e
	}
	natsGame.player2GameSub, e = nc.Subscribe(player2GameSubject, natsGame.player2Game)
	if e != nil {
		natsLogger.Error().Msg(fmt.Sprintf("Failed to subscribe to %s", player2GameSubject))
		natsGame.player2HandSub.Unsubscribe()
		return nil, e
	}
	gameType := game.GameType(game.GameType_value[config.GameTypeStr])

	serverGame, gameID := game.GameManager.InitializeGame(*natsGame, clubID,
		gameID,
		gameType,
		config.Title,
		int(config.MinPlayers),
		config.AutoStart, false)
	natsGame.serverGame = serverGame
	return natsGame, nil
}

func (n *NatsGame) cleanup() {
	n.player2HandSub.Unsubscribe()
	n.player2GameSub.Unsubscribe()
}

// message sent from apiserver to game
func (n *NatsGame) GameStatusChanged(gameID uint64, newStatus game.GameStatus) {
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
		Msg(fmt.Sprintf("APIServer->Game: Status changed. GameID: %d, NewStatus: %s", gameID, game.GameStatus_name[int32(newStatus)]))
	var message game.GameMessage
	message.GameId = gameID
	message.MessageType = game.GameStatusChanged
	message.GameMessage = &game.GameMessage_StatusChange{StatusChange: &game.GameStatusChangeMessage{NewStatus: newStatus}}

	n.serverGame.SendGameMessage(&message)
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

	n.serverGame.SendGameMessage(&message)
}

// messages sent from player to game hand
func (n *NatsGame) player2Hand(msg *natsgo.Msg) {
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
		Msg(fmt.Sprintf("Player->Hand: %s", string(msg.Data)))
	var message game.HandMessage
	//err := jsoniter.Unmarshal(msg.Data, &message)
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

	if message.MessageType == game.GameTableState {
		// update table status
		UpdateTableStatus(message.GameId, message.GetTableState().TableStatus)
	}

	n.nc.Publish(n.game2AllPlayersSubject, data)
}

func (n NatsGame) BroadcastHandMessage(message *game.HandMessage) {
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
		Msg(fmt.Sprintf("Hand->AllPlayers: %s", message.MessageType))
	//hand2PlayerSubject := fmt.Sprintf("game.%d%d.hand.player.*", n.clubID, n.gameNum)
	data, _ := protojson.Marshal(message)
	n.nc.Publish(n.hand2PlayerAllSubject, data)
}

func (n NatsGame) SendHandMessageToPlayer(message *game.HandMessage, playerID uint32) {
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
		Msg(fmt.Sprintf("Hand->Player: %s", message.MessageType))
	hand2PlayerSubject := fmt.Sprintf("game.%d.hand.player.%d", n.gameID, playerID)
	message.PlayerId = playerID
	data, _ := protojson.Marshal(message)
	n.nc.Publish(hand2PlayerSubject, data)
}

func (n NatsGame) SendGameMessageToPlayer(message *game.GameMessage, playerID uint32) {
	natsLogger.Info().Uint64("game", n.gameID).Uint32("clubID", n.clubID).
		Msg(fmt.Sprintf("Game->Player: %s", message.MessageType))
	subject := fmt.Sprintf("game.%d.player.%d", message.GameId, playerID)
	// let send this to all players
	data, _ := protojson.Marshal(message)
	n.nc.Publish(subject, data)
}
