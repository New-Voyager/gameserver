package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"voyager.com/botrunner/internal/driver"
	"voyager.com/gamescript"
	"voyager.com/logging"
)

// AppGame manages an instance of a BotRunner that creates a script game using an existing club.
type AppGame struct {
	logger          zerolog.Logger
	botRunnerLogDir string
	players         *gamescript.Players
	script          *gamescript.Script
	clubCode        string
	name            string
	instance        *driver.BotRunner
}

// NewAppGame creates a new instance of AppGame.
func NewAppGame(clubCode string, name string, players *gamescript.Players, script *gamescript.Script) (*AppGame, error) {
	b := AppGame{
		logger:          *logging.GetZeroLogger("AppGame", nil),
		botRunnerLogDir: filepath.Join(baseLogDir, "app_game"),
		players:         players,
		script:          script,
		clubCode:        clubCode,
		name:            name,
	}
	return &b, nil
}

// Launch launches the BotRunner.
func (b *AppGame) Launch() error {
	err := os.MkdirAll(b.botRunnerLogDir, os.ModePerm)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Unable to create log directory %s", b.botRunnerLogDir))
	}
	logFileName := b.getLogFileName(b.botRunnerLogDir)
	f, err := os.Create(logFileName)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Unable to create log file %s", logFileName))
	}

	b.logger.Info().Msgf("Launching bot runner to start an app game. Logging to %s", logFileName)
	botRunnerLogger := logging.GetZeroLogger("BotRunner", f).With().Logger()
	botPlayerLogger := logging.GetZeroLogger("BotPlayer", f).With().Logger()
	botRunner, err := driver.NewBotRunner(b.clubCode, "", b.script, b.players, &botRunnerLogger, &botPlayerLogger, false, false)
	if err != nil {
		errors.Wrap(err, "Error while creating a BotRunner")
	}
	go botRunner.Run()
	b.instance = botRunner
	return nil
}

func (b *AppGame) getLogFileName(baseDir string) string {
	return filepath.Join(baseDir, fmt.Sprintf("%s.log", b.name))
}
