package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"voyager.com/botrunner/internal/driver"
	"voyager.com/gamescript"
)

// HumanGame manages an instance of a BotRunner that joins to a human-created game.
type HumanGame struct {
	logger          zerolog.Logger
	botRunnerLogDir string
	players         *gamescript.Players
	script          *gamescript.Script
	clubCode        string
	gameCode        string
	instance        *driver.BotRunner
}

// NewHumanGame creates a new instance of HumanGame.
func NewHumanGame(clubCode string, gameCode string, players *gamescript.Players, script *gamescript.Script) (*HumanGame, error) {
	b := HumanGame{
		logger:          log.With().Str("logger_name", "HumanGame").Logger(),
		botRunnerLogDir: filepath.Join(baseLogDir, "human_game"),
		players:         players,
		script:          script,
		clubCode:        clubCode,
		gameCode:        gameCode,
	}
	return &b, nil
}

// Launch launches the BotRunner.
func (b *HumanGame) Launch() error {
	botRunnerLoggerName := fmt.Sprintf("BotRunner<%s>", b.gameCode)
	botPlayerLoggerName := fmt.Sprintf("BotPlayer<%s>", b.gameCode)
	err := os.MkdirAll(b.botRunnerLogDir, os.ModePerm)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Unable to create log directory %s", b.botRunnerLogDir))
	}
	logFileName := b.getLogFileName(b.botRunnerLogDir)
	f, err := os.Create(logFileName)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Unable to create log file %s", logFileName))
	}
	botRunnerLogger := zerolog.New(f).With().Str("logger_name", botRunnerLoggerName).Logger()
	botPlayerLogger := zerolog.New(f).With().Str("logger_name", botPlayerLoggerName).Logger()

	b.logger.Info().Msgf("Launching bot runner to join a human game. Logging to %s", logFileName)
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

func (b *HumanGame) getLogFileName(baseDir string) string {
	return filepath.Join(baseDir, fmt.Sprintf("%s.log", b.gameCode))
}
