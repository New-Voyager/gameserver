package game

import (
	"github.com/rs/zerolog/log"
)

var testGameLogger = log.With().Str("logger_name", "test::testgame").Logger()

// TestGame is a game simulation object to drive the game from client perspective
// this is used for testing the game, hands, winners, split pots
type TestGame struct {
	clubID           uint32
	gameNum          uint32
	players          map[uint32]*TestPlayer
	nextActionPlayer *TestPlayer
	observerCh       chan []byte // observer and game manager/club owner
	observer         *TestPlayer
}

func NewTestGame(gameScript *TestGameScript, clubID uint32,
	gameType GameType,
	name string,
	autoStart bool,
	players []GamePlayer) (*TestGame, *TestPlayer) {

	gamePlayers := make(map[uint32]*TestPlayer)
	gameNum := gameManager.InitializeGame(clubID, gameType, name, len(players), autoStart, false)

	for _, playerInfo := range players {
		testPlayer := NewTestPlayer(playerInfo)
		player := NewPlayer(clubID, gameNum, playerInfo.Name, playerInfo.ID, testPlayer)
		testPlayer.setPlayer(player)
		gamePlayers[playerInfo.ID] = testPlayer
	}

	observerCh := make(chan []byte)
	// add test driver as an observer/player
	gameScriptPlayer := GamePlayer{ID: 0xFFFFFFFF, Name: "GameScript"}
	observer := NewTestPlayerAsObserver(gameScriptPlayer, observerCh)
	player := NewPlayer(clubID, gameNum, gameScriptPlayer.Name, gameScriptPlayer.ID, observer)
	observer.setPlayer(player)
	gamePlayers[gameScriptPlayer.ID] = observer

	// wait for the cards to be dealt
	return &TestGame{
		clubID:     clubID,
		gameNum:    gameNum,
		players:    gamePlayers,
		observerCh: observerCh,
		observer:   observer,
	}, observer
}

func (t *TestGame) Start(playerAtSeats []PlayerSeat) {
	for _, testPlayer := range t.players {
		testPlayer.player.joinGame(t.clubID, t.gameNum)
	}

	for _, testPlayer := range playerAtSeats {
		t.players[testPlayer.Player].player.sitAtTable(testPlayer.SeatNo, testPlayer.BuyIn)
	}
	t.observer.startGame(t.clubID, t.gameNum)
}

func (t *TestGame) Observer() *TestPlayer {
	return t.observer
}

func (o *TestPlayer) startGame(clubID uint32, gameNum uint32) error {
	return o.player.startGame(clubID, gameNum)
}

func (o *TestPlayer) setupNextHand(deck []byte, buttonPos uint32) error {
	return o.player.setupNextHand(deck, buttonPos)
}

func (o *TestPlayer) getTableState() error {
	return o.player.getTableState()
}

func (o *TestPlayer) dealNextHand() error {
	err := o.player.dealHand()
	if err != nil {
		return err
	}
	return nil
}
