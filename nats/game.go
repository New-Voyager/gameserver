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

const NatsURL = "nats://localhost:4222"

var gameManager = game.NewGameManager()

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
	gameNum                uint32
	chEndGame              chan bool
	player2GameSubject     string
	player2HandSubject     string
	game2AllPlayersSubject string

	serverGame *game.Game

	player2GameSub *natsgo.Subscription
	player2HandSub *natsgo.Subscription
	nc             *natsgo.Conn
}

func NewGame(clubID uint32, gameNum uint32) (*NatsGame, error) {
	// let us try to connect to nats server
	nc, err := natsgo.Connect(NatsURL)
	if err != nil {
		natsLogger.Error().Msg(fmt.Sprintf("Failed to connect to nats server: %v", err))
		return nil, err
	}

	// game subjects
	player2GameSubject := fmt.Sprintf("game.%d%d.player", clubID, gameNum)
	game2AllPlayersSubject := fmt.Sprintf("game.%d%d.allplayers", clubID, gameNum)

	// hand subjects
	player2HandSubject := fmt.Sprintf("game.%d%d.hand.player", clubID, gameNum)

	// we need to use the API to get the game configuration
	game := &NatsGame{
		clubID:                 clubID,
		gameNum:                gameNum,
		chEndGame:              make(chan bool),
		nc:                     nc,
		player2GameSubject:     player2GameSubject,
		game2AllPlayersSubject: game2AllPlayersSubject,
		player2HandSubject:     player2HandSubject,
	}

	// subscribe to topics
	var e error
	game.player2HandSub, e = nc.Subscribe(player2HandSubject, game.player2Hand)
	if e != nil {
		natsLogger.Error().Msg(fmt.Sprintf("Failed to subscribe to %s", player2HandSubject))
		return nil, e
	}
	game.player2GameSub, e = nc.Subscribe(player2GameSubject, game.player2Game)
	if e != nil {
		natsLogger.Error().Msg(fmt.Sprintf("Failed to subscribe to %s", player2GameSubject))
		game.player2HandSub.Unsubscribe()
		return nil, e
	}
	return game, nil
}

func (n *NatsGame) cleanup() {
	n.player2HandSub.Unsubscribe()
	n.player2GameSub.Unsubscribe()
}

// messages sent from player to game
func (n *NatsGame) player2Game(msg *natsgo.Msg) {
	natsLogger.Info().Uint32("game", n.gameNum).Uint32("clubID", n.clubID).
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
	natsLogger.Info().Uint32("game", n.gameNum).Uint32("clubID", n.clubID).
		Msg(fmt.Sprintf("Player->Hand: %s", string(msg.Data)))
}

func (n NatsGame) BroadcastGameMessage(message *game.GameMessage) {
	natsLogger.Info().Uint32("game", n.gameNum).Uint32("clubID", n.clubID).
		Msg(fmt.Sprintf("Game->Player: %s", message.MessageType))
	// let send this to all players
	data, _ := protojson.Marshal(message)
	n.nc.Publish(n.game2AllPlayersSubject, data)
}

func (n NatsGame) BroadcastHandMessage(message *game.HandMessage) {
	natsLogger.Info().Uint32("game", n.gameNum).Uint32("clubID", n.clubID).
		Msg(fmt.Sprintf("Hand->AllPlayers: %s", message.MessageType))
	hand2PlayerSubject := fmt.Sprintf("game.%d%d.hand.player.*", n.clubID, n.gameNum)
	data, _ := protojson.Marshal(message)
	n.nc.Publish(hand2PlayerSubject, data)
}

func (n NatsGame) SendHandMessageToPlayer(message *game.HandMessage, playerID uint32) {
	natsLogger.Info().Uint32("game", n.gameNum).Uint32("clubID", n.clubID).
		Msg(fmt.Sprintf("Hand->Player: %s", message.MessageType))
	hand2PlayerSubject := fmt.Sprintf("game.%d%d.hand.player.%d", n.clubID, n.gameNum, playerID)
	message.PlayerId = playerID
	data, _ := protojson.Marshal(message)
	n.nc.Publish(hand2PlayerSubject, data)
}

func (n NatsGame) SendGameMessageToPlayer(message *game.GameMessage, playerID uint32) {
	natsLogger.Info().Uint32("game", n.gameNum).Uint32("clubID", n.clubID).
		Msg(fmt.Sprintf("Game->Player: %s", message.MessageType))
	subject := fmt.Sprintf("game.%d%d.player.%d", message.ClubId, message.GameNum, playerID)
	// let send this to all players
	data, _ := protojson.Marshal(message)
	n.nc.Publish(subject, data)
}
