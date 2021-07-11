package test

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"voyager.com/server/game"
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
	observerCh       chan observerChItem // observer and game manager/club owner
	observer         *TestPlayer
	playersInSeats   map[uint32]*TestPlayer
}

type observerChItem struct {
	gameMessage *game.GameMessage
	handMessage *game.HandMessage
	handMsgItem *game.HandMessageItem
}

func NewTestGame(gameScript *TestGameScript, clubID uint32,
	gameType game.GameType,
	name string,
	autoStart bool,
	players []game.GamePlayer) (*TestGame, *TestPlayer, error) {

	gamePlayers := make(map[uint64]*TestPlayer)

	now := time.Now().UnixNano()
	gameCode := fmt.Sprintf("%d", now)
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
	gameScript.gameScript.GameConfig.GameCode = gameCode
	gameScript.gameScript.GameConfig.ClubId = clubID
	gameScript.gameScript.GameConfig.GameType = gameType
	gameScript.gameScript.GameConfig.GameId = uint64(now)

	serverGame, gameID, err := game.GameManager.InitializeGame(nil, &gameScript.gameScript.GameConfig)
	if err != nil {
		return nil, nil, err
	}
	serverGame.GameStarted()

	observerCh := make(chan observerChItem)
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
		clubID:         clubID,
		gameID:         gameID,
		players:        gamePlayers,
		observerCh:     observerCh,
		observer:       observer,
		playersInSeats: make(map[uint32]*TestPlayer),
	}, observer, nil
}

func (t *TestGame) PopulateSeats(playerAtSeats []game.PlayerSeat) {
	for _, testPlayer := range playerAtSeats {
		player := t.players[testPlayer.Player]
		player.joinGame(t.gameID, testPlayer.SeatNo,
			testPlayer.BuyIn, testPlayer.RunItTwice,
			testPlayer.RunItTwicePromptResponse,
			testPlayer.PostBlind)
		t.playersInSeats[testPlayer.SeatNo] = player
	}

	// observer joins seat 0
	t.observer.player.JoinGame(t.gameID, 0, 0, false, false, false)
}

func (t *TestGame) Observer() *TestPlayer {
	return t.observer
}

func (o *TestPlayer) setupNextHand(num uint32, handSetup game.HandSetup) error {
	return o.player.SetupNextHand(num, handSetup)
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
