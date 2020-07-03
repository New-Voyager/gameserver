package test

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"voyager.com/server/game"
)

//import "time"
var testGameLogger = log.With().Str("logger_name", "test::testgame").Logger()

type TestPlayerInfo struct {
	Name   string
	ID     uint32
	SeatNo uint32
	BuyIn  float32
}

var gameManager = game.NewGameManager()

// TestGame is a game simulation object to drive the game from client perspective
// this is used for testing the game, hands, winners, split pots
type TestGame struct {
	clubID					 uint32
	gameNum          uint32
	players          []*TestPlayer
	nextActionPlayer *TestPlayer
}

func NewGame(clubID uint32, gameType game.GameType, name string, players []TestPlayerInfo) *TestGame {
	gamePlayers := make([]*TestPlayer, len(players))
	gameNum := gameManager.StartGame(clubID, gameType, name, len(players))
	for i, playerInfo := range players {
		testPlayer := NewTestPlayer(playerInfo)
		player := game.NewPlayer(clubID, gameNum, playerInfo.Name, playerInfo.ID, testPlayer)
		testPlayer.setPlayer(player)
		gamePlayers[i] = testPlayer
	}

	// wait for the cards to be dealt
	time.Sleep(500 * time.Millisecond)
	return &TestGame{
		clubID: clubID,
		gameNum: gameNum,
		players: gamePlayers,
	}
}

func (t *TestGame) Start() {
	for _, testPlayer := range t.players {
		gameManager.SitAtTable(t.clubID, 
								t.gameNum, 
								testPlayer.player, 
								testPlayer.playerInfo.SeatNo, 
								testPlayer.playerInfo.BuyIn)
	}

	time.Sleep(500 * time.Millisecond)
}

// TestPlayer is a receiver for game and hand messages
// it also sends messages to game and hand via player object
type TestPlayer struct {
	playerInfo TestPlayerInfo
	player *game.Player
}

func NewTestPlayer(playerInfo TestPlayerInfo) *TestPlayer {
	return &TestPlayer{
		playerInfo: playerInfo,
	}
}

func (t *TestPlayer) setPlayer(player *game.Player) {
	t.player = player
}

func (t *TestPlayer) HandMessageFromGame(jsonb []byte) {
	testGameLogger.Info().
		Uint32("club", t.player.ClubID).
		Uint32("game", t.player.GameNum).
		Uint32("playerid", t.player.PlayerID).
		Uint32("seatNo", t.player.SeatNo).
		Str("player", t.player.PlayerName).
		Msg(fmt.Sprintf("Json: %s", string(jsonb)))
}

func (t *TestPlayer) GameMessageFromGame(jsonb []byte) {
	testGameLogger.Info().
		Uint32("club", t.player.ClubID).
		Uint32("game", t.player.GameNum).
		Uint32("playerid", t.player.PlayerID).
		Uint32("seatNo", t.player.SeatNo).
		Str("player", t.player.PlayerName).
		Msg(fmt.Sprintf("Json: %s", string(jsonb)))
}
