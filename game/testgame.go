package game

import (
	"fmt"
	"github.com/rs/zerolog/log"
)

//import "time"
var testGameLogger = log.With().Str("logger_name", "game::testgame").Logger()

func RunTestGame() {
	gameManager := NewGameManager()
	gameNum := gameManager.StartGame(GameType_HOLDEM, "First game", 5)
	player1Delegate := NewTestPlayer()
	player1 := NewPlayer("rob", 1, player1Delegate)
	player1Delegate.setPlayer(player1)
	gameManager.JoinGame(gameNum, player1, 1)

	player2Delegate := NewTestPlayer()
	player2 := NewPlayer("steve", 2, player2Delegate)
	player2Delegate.setPlayer(player1)
	gameManager.JoinGame(gameNum, player2, 2)
	player3Delegate := NewTestPlayer()
	player3 := NewPlayer("larry", 3, player3Delegate)
	player3Delegate.setPlayer(player1)
	gameManager.JoinGame(gameNum, player3, 3)
	player4Delegate := NewTestPlayer()
	player4 := NewPlayer("pike", 4, player4Delegate)
	player4Delegate.setPlayer(player1)
	gameManager.JoinGame(gameNum, player4, 4)
	player5Delegate := NewTestPlayer()
	player5 := NewPlayer("fish", 5, player5Delegate)
	player5Delegate.setPlayer(player1)
	gameManager.JoinGame(gameNum, player5, 5)
	select {}
}

// TestPlayer is a receiver for game and hand messages
// it also sends messages to game and hand via player object
type TestPlayer struct {
	player *Player
	// channel to send messages to game
	chSendGame chan []byte
	// channel to send messages to hand
	chSendHand chan []byte
}

func NewTestPlayer() *TestPlayer {
	return &TestPlayer{
		chSendGame: make(chan []byte),
		chSendHand: make(chan []byte),
	}
}

func (t *TestPlayer) setPlayer(player *Player) {
	t.player = player
}

func (t *TestPlayer) onHandMessage(jsonb []byte) {
	testGameLogger.Info().
		Msg(fmt.Sprintf("Json: %s", string(jsonb)))
}

func (t *TestPlayer) onGameMessage(jsonb []byte) {
	testGameLogger.Info().
		Msg(fmt.Sprintf("Json: %s", string(jsonb)))
}

func (t *TestPlayer) getHandChannel() chan []byte {
	return t.chSendHand
}

func (t *TestPlayer) getGameChannel() chan []byte {
	return t.chSendGame
}
