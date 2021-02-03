package test

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"voyager.com/server/game"
	"voyager.com/server/poker"
)

var testGameLogger = log.With().Str("logger_name", "test::testgame").Logger()
var gameID = 1

// TestGame is a game simulation object to drive the game from client perspective
// this is used for testing the game, hands, winners, split pots
type TestGame struct {
	clubID           uint32
	gameID           uint64
	players          map[uint64]*TestPlayer
	nextActionPlayer *TestPlayer
	observerCh       chan []byte // observer and game manager/club owner
	observer         *TestPlayer
}

func NewTestGame(gameScript *TestGameScript, clubID uint32,
	gameType game.GameType,
	name string,
	autoStart bool,
	players []game.GamePlayer) (*TestGame, *TestPlayer, error) {

	gamePlayers := make(map[uint64]*TestPlayer)

	gameCode := fmt.Sprintf("%d", time.Now().Unix())
	maxPlayers := 9
	config := game.GameConfig{
		ClubId:     clubID,
		GameType:   gameType,
		GameCode:   gameCode,
		MinPlayers: len(players),
		MaxPlayers: maxPlayers,
		AutoStart:  autoStart,
		SmallBlind: gameScript.gameScript.GameConfig.SmallBlind,
		BigBlind:   gameScript.gameScript.GameConfig.BigBlind,
		ActionTime: 300,
	}
	_ = config
	if gameScript.gameScript.GameConfig.ActionTime == 0 {
		gameScript.gameScript.GameConfig.ActionTime = 300
	}
	gameID++
	gameScript.gameScript.GameConfig.GameCode = "000000"
	gameScript.gameScript.GameConfig.ClubId = clubID
	gameScript.gameScript.GameConfig.GameType = gameType
	gameScript.gameScript.GameConfig.GameId = uint64(time.Now().Unix())

	serverGame, gameID, err := game.GameManager.InitializeGame(nil, &gameScript.gameScript.GameConfig, false)
	if err != nil {
		return nil, nil, err
	}
	serverGame.SetScriptTest(true)

	observerCh := make(chan []byte)
	// add test driver as an observer/player
	gameScriptPlayer := game.GamePlayer{ID: 0xFFFFFFFF, Name: "GameScript"}
	observer := NewTestPlayerAsObserver(gameScriptPlayer, observerCh)

	for _, playerInfo := range players {
		testPlayer := NewTestPlayer(playerInfo, observer)
		player := game.NewPlayer(clubID, gameID, playerInfo.Name, playerInfo.ID, testPlayer)
		testPlayer.setPlayer(player)
		gamePlayers[playerInfo.ID] = testPlayer
	}

	player := game.NewPlayer(clubID, gameID, gameScriptPlayer.Name, gameScriptPlayer.ID, observer)
	observer.setPlayer(player)
	gamePlayers[gameScriptPlayer.ID] = observer

	// wait for the cards to be dealt
	return &TestGame{
		clubID:     clubID,
		gameID:     gameID,
		players:    gamePlayers,
		observerCh: observerCh,
		observer:   observer,
	}, observer, nil
}

func (t *TestGame) Start(playerAtSeats []game.PlayerSeat) {
	for _, testPlayer := range t.players {
		testPlayer.player.JoinGame(t.clubID, t.gameID)
	}

	for _, testPlayer := range playerAtSeats {
		t.players[testPlayer.Player].player.SitAtTable(testPlayer.SeatNo, testPlayer.BuyIn)
	}
	t.observer.player.StartGame(t.clubID, t.gameID)
}

func (t *TestGame) Observer() *TestPlayer {
	return t.observer
}

func (o *TestPlayer) startGame(clubID uint32, gameID uint64) error {
	return o.player.StartGame(clubID, gameID)
}

func (o *TestPlayer) setupNextHand(deck *poker.Deck, autoDeal bool, buttonPos uint32) error {
	var deckBytes []byte
	if deck != nil {
		deckBytes = deck.GetBytes()
	}
	return o.player.SetupNextHand(deckBytes, autoDeal, buttonPos)
}

func (o *TestPlayer) getTableState() error {
	return o.player.GetTableState()
}

func (o *TestPlayer) dealNextHand() error {
	err := o.player.DealHand()
	if err != nil {
		return err
	}
	return nil
}
