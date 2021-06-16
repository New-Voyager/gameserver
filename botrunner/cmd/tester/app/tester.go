package app

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	_player "voyager.com/botrunner/internal/player"
	"voyager.com/botrunner/internal/util"
	"voyager.com/gamescript"
)

var (
	logger = log.With().Str("logger_name", "app::tester").Logger()
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

	playerLogger := log.With().Str("logger_name", "TesterPlayer").Logger()
	player, err := _player.NewBotPlayer(_player.Config{
		Name:          playerConf.Name,
		DeviceID:      playerConf.DeviceID,
		Email:         playerConf.Email,
		Password:      playerConf.Password,
		IsHuman:       true,
		APIServerURL:  util.Env.GetAPIServerURL(),
		NatsURL:       util.Env.GetNatsURL(),
		GQLTimeoutSec: 10,
		Script:        t.script,
		Players:       t.players,
	}, &playerLogger)
	if err != nil {
		return err
	}
	t.player = player

	err = t.player.Login(playerConf.DeviceID, playerConf.DeviceID)
	if err != nil {
		return errors.Wrap(err, "Unable to login")
	}

	err = t.player.JoinGame(t.gameCode)
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
