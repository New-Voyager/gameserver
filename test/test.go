package test

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"voyager.com/server/game"
)

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
	players          map[uint32]*TestPlayer
	nextActionPlayer *TestPlayer
}

func NewTestGame(clubID uint32, gameType game.GameType, name string, autoStart bool, players []GamePlayer) *TestGame {
	gamePlayers := make(map[uint32]*TestPlayer)
	gameNum := gameManager.InitializeGame(clubID, gameType, name, len(players), autoStart, false)
	for _, playerInfo := range players {
		testPlayer := NewTestPlayer(playerInfo)
		player := game.NewPlayer(clubID, gameNum, playerInfo.Name, playerInfo.ID, testPlayer)
		testPlayer.setPlayer(player)
		gamePlayers[playerInfo.ID] = testPlayer
	}

	// wait for the cards to be dealt
	time.Sleep(500 * time.Millisecond)
	return &TestGame{
		clubID: clubID,
		gameNum: gameNum,
		players: gamePlayers,
	}
}

func (t *TestGame) Start(playerAtSeats []PlayerSeat) {
	for _, testPlayer := range playerAtSeats {
		gameManager.SitAtTable(t.clubID, 
								t.gameNum,
								t.players[testPlayer.Player].player,
								testPlayer.SeatNo, 
								testPlayer.BuyIn)
	}

	time.Sleep(500 * time.Millisecond)
	gameManager.StartGame(t.clubID, t.gameNum)
}

// TestPlayer is a receiver for game and hand messages
// it also sends messages to game and hand via player object
type TestPlayer struct {
	playerInfo GamePlayer
	player *game.Player
}

func NewTestPlayer(playerInfo GamePlayer) *TestPlayer {
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
