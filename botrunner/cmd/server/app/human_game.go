package app

import (
	"os"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"voyager.com/botrunner/internal/driver"
	"voyager.com/gamescript"
	"voyager.com/logging"
)

// HumanGame manages an instance of a BotRunner that joins to a human-created game.
type HumanGame struct {
	logger   *zerolog.Logger
	players  *gamescript.Players
	script   *gamescript.Script
	clubCode string
	gameCode string
	gameID   uint64
	instance *driver.BotRunner
	demoGame bool
}

// NewHumanGame creates a new instance of HumanGame.
func NewHumanGame(clubCode string, gameID uint64, gameCode string, players *gamescript.Players, script *gamescript.Script, demoGame bool) (*HumanGame, error) {
	logger := logging.GetZeroLogger("HumanGame", nil).With().
		Uint64(logging.GameIDKey, gameID).
		Str(logging.GameCodeKey, gameCode).
		Logger()

	b := HumanGame{
		logger:   &logger,
		players:  players,
		script:   script,
		clubCode: clubCode,
		gameCode: gameCode,
		gameID:   gameID,
		demoGame: demoGame,
	}
	return &b, nil
}

// Launch launches the BotRunner.
func (b *HumanGame) Launch() error {
	b.logger.Info().Msgf("Launching bot runner to join a human game.")
	playerGame := false
	if b.clubCode == "" {
		playerGame = true
	}
	botRunner, err := driver.NewBotRunner(b.clubCode, b.gameCode, b.script, b.players, os.Stdout, os.Stdout, false, playerGame, b.demoGame)
	if err != nil {
		errors.Wrap(err, "Error while creating a BotRunner")
	}
	go func() {
		err := botRunner.Run()
		if err != nil {
			b.logger.Err(err).Msg("Botrunner returned error")
		}
		// TOOD: Clean up this instance.
	}()
	b.instance = botRunner
	return nil
}
