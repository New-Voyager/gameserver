package app

import (
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
}

// NewHumanGame creates a new instance of HumanGame.
func NewHumanGame(clubCode string, gameID uint64, gameCode string, players *gamescript.Players, script *gamescript.Script) (*HumanGame, error) {
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
	}
	return &b, nil
}

// Launch launches the BotRunner.
func (b *HumanGame) Launch() error {
	botRunnerLogger := logging.GetZeroLogger("BotRunner", nil).With().
		Uint64(logging.GameIDKey, b.gameID).
		Str(logging.GameCodeKey, b.gameCode).
		Logger()
	botPlayerLogger := logging.GetZeroLogger("BotPlayer", nil).With().
		Uint64(logging.GameIDKey, b.gameID).
		Str(logging.GameCodeKey, b.gameCode).
		Logger()

	b.logger.Info().Msgf("Launching bot runner to join a human game.")
	playerGame := false
	if b.clubCode == "" {
		playerGame = true
	}
	botRunner, err := driver.NewBotRunner(b.clubCode, b.gameCode, b.script, b.players, &botRunnerLogger, &botPlayerLogger, false, playerGame)
	if err != nil {
		errors.Wrap(err, "Error while creating a BotRunner")
	}
	go botRunner.Run()
	b.instance = botRunner
	return nil
}
