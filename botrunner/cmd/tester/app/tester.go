package app

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	_player "voyager.com/botrunner/internal/player"
	"voyager.com/botrunner/internal/util"
	"voyager.com/gamescript"
	"voyager.com/logging"
)

var (
	logger = logging.GetZeroLogger("app::tester", nil)
)

// Tester is the object that drives the tester application.
type Tester struct {
	players  *gamescript.Players
	script   *gamescript.Script
	gameCode string
	player   *_player.BotPlayer
}

// NewTester creates new instance of Tester.
func NewTester(players *gamescript.Players, script *gamescript.Script, gameCode string) (*Tester, error) {
	t := Tester{
		players:  players,
		script:   script,
		gameCode: gameCode,
	}

	return &t, nil
}

// Run joins the game and follows it to the end.
func (t *Tester) Run() error {
	logger.Debug().Msgf("Players: %+v, Script: %+v", t.players, t.script)
	playerConf := t.getPlayerConfig()
	if playerConf == nil {
		return fmt.Errorf("No player found in the setup script")
	}

	playerLogger := logging.GetZeroLogger("TesterPlayer", nil)
	player, err := _player.NewBotPlayer(_player.Config{
		Name:          playerConf.Name,
		DeviceID:      playerConf.DeviceID,
		Email:         playerConf.Email,
		Password:      playerConf.Password,
		Gps:           &playerConf.Gps,
		IsHuman:       true,
		APIServerURL:  util.Env.GetAPIServerURL(),
		NatsURL:       util.Env.GetNatsURL(),
		GQLTimeoutSec: 10,
		Script:        t.script,
		Players:       t.players,
	}, playerLogger)
	if err != nil {
		return err
	}
	t.player = player

	err = t.player.Login()
	if err != nil {
		return errors.Wrap(err, "Unable to login")
	}

	err = t.player.JoinGame(t.gameCode, nil)
	if err != nil {
		return errors.Wrap(err, "Unable to join game")
	}

	for !t.player.IsGameOver() {
		time.Sleep(200 * time.Millisecond)
	}

	return nil
}

func (t *Tester) getPlayerConfig() *gamescript.Player {
	testerPlayerName := t.script.Tester
	for _, player := range t.players.Players {
		if player.Name == testerPlayerName {
			return &player
		}
	}
	return nil
}
