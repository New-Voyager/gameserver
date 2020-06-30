package game

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

//import "time"
var testGameLogger = log.With().Str("logger_name", "game::testgame").Logger()

type TestPlayerInfo struct {
	Name   string
	ID     uint32
	SeatNo uint32
}

var gameManager = NewGameManager()

// TestGame is a game simulation object to drive the game from client perspective
// this is used for testing the game, hands, winners, split pots
type TestGame struct {
	gameNum          uint32
	players          []*TestPlayer
	nextActionPlayer *TestPlayer
}

func NewGame(gameType GameType, name string, players []TestPlayerInfo) *TestGame {
	gamePlayers := make([]*TestPlayer, len(players))
	gameNum := gameManager.StartGame(gameType, name, len(players))
	for i, playerInfo := range players {
		testPlayer := NewTestPlayer(playerInfo.SeatNo)
		player := NewPlayer(playerInfo.Name, playerInfo.ID, testPlayer)
		testPlayer.setPlayer(player)
		gamePlayers[i] = testPlayer
	}

	// wait for the cards to be dealt
	time.Sleep(500 * time.Millisecond)
	return &TestGame{
		gameNum: gameNum,
		players: gamePlayers,
	}
}

func (t *TestGame) Start() {
	for _, testPlayer := range t.players {
		gameManager.JoinGame(t.gameNum, testPlayer.player, testPlayer.seatNo)
	}

	time.Sleep(500 * time.Millisecond)
}

// TestPlayer is a receiver for game and hand messages
// it also sends messages to game and hand via player object
type TestPlayer struct {
	player *Player
	seatNo uint32
	// channel to send messages to game
	chSendGame chan []byte
	// channel to send messages to hand
	chSendHand chan []byte
}

func NewTestPlayer(seatNo uint32) *TestPlayer {
	return &TestPlayer{
		seatNo:     seatNo,
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
