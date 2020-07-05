package test

import (
	"time"

	"github.com/rs/zerolog/log"
	"voyager.com/server/game"
)

var testGameLogger = log.With().Str("logger_name", "test::testgame").Logger()

// TestGame is a game simulation object to drive the game from client perspective
// this is used for testing the game, hands, winners, split pots
type TestGame struct {
	clubID           uint32
	gameNum          uint32
	players          map[uint32]*TestPlayer
	nextActionPlayer *TestPlayer
}

func NewTestGame(testDriver *TestDriver, clubID uint32,
	gameType game.GameType,
	name string,
	autoStart bool,
	players []GamePlayer) *TestGame {

	gamePlayers := make(map[uint32]*TestPlayer)
	gameNum := gameManager.InitializeGame(clubID, gameType, name, len(players), autoStart, false)

	for _, playerInfo := range players {
		testPlayer := NewTestPlayer(playerInfo)
		player := game.NewPlayer(clubID, gameNum, playerInfo.Name, playerInfo.ID, testPlayer)
		testPlayer.setPlayer(player)
		gamePlayers[playerInfo.ID] = testPlayer
	}

	// add test driver as an observer/player
	testDriverObserver := GamePlayer{ID: 0xFFFFFFFF, Name: "TestDriver"}
	testDriver.Observer = NewTestPlayer(testDriverObserver)
	player := game.NewPlayer(clubID, gameNum, testDriverObserver.Name, testDriverObserver.ID, testDriver.Observer)
	testDriver.Observer.setPlayer(player)
	gamePlayers[testDriverObserver.ID] = testDriver.Observer

	// wait for the cards to be dealt
	time.Sleep(500 * time.Millisecond)
	return &TestGame{
		clubID:  clubID,
		gameNum: gameNum,
		players: gamePlayers,
	}
}

func (t *TestGame) Start(playerAtSeats []PlayerSeat) {
	for _, testPlayer := range t.players {
		gameManager.JoinGame(t.clubID,
			t.gameNum,
			testPlayer.player)
	}

	for _, testPlayer := range playerAtSeats {
		gameManager.SitAtTable(t.clubID,
			t.gameNum,
			t.players[testPlayer.Player].player,
			testPlayer.SeatNo,
			testPlayer.BuyIn)
	}

	gameManager.StartGame(t.clubID, t.gameNum)
	time.Sleep(500 * time.Millisecond)
}

func (t *TestGame) SetupNextHand(deck []byte, buttonPos uint32) error {
	err := gameManager.SetupNextHand(t.clubID, t.gameNum, deck, buttonPos)
	if err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	return nil
}
