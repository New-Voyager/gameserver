package main

import (
	"flag"
	"fmt"
	"os"

	_ "github.com/lib/pq"
	"github.com/rs/zerolog"
	"voyager.com/botrunner/internal/driver"
	"voyager.com/botrunner/internal/util"
	"voyager.com/logging"

	"voyager.com/gamescript"
)

var (
	cmdArgs    arg
	mainLogger = logging.GetZeroLogger("main::main", nil)
)

type arg struct {
	playersFile string
	scriptFile  string
	clubCode    string
	gameCode    string
	resetDB     bool
}

func init() {
	flag.StringVar(&cmdArgs.scriptFile, "script", "", "Game script YAML file")
	flag.StringVar(&cmdArgs.playersFile, "players", "botrunner_scripts/players/default.yaml", "Players YAML file")
	flag.StringVar(&cmdArgs.clubCode, "club-code", "", "Club code to use. If not provided, a club will be created and owned by a bot.")
	flag.StringVar(&cmdArgs.gameCode, "game-code", "", "Game code to use. If not provided, a game will be created and started by a bot.")
	flag.BoolVar(&cmdArgs.resetDB, "reset-db", false, "If true, resets postgres tables before starting the game.")
	flag.Parse()
}

func main() {
	os.Exit(botrunner())
}

func botrunner() int {
	logLevel := util.Env.GetZeroLogLogLevel()
	fmt.Printf("Setting log level to %s\n", logLevel)
	zerolog.SetGlobalLevel(logLevel)
	mainLogger.Info().Msgf("Nats url: %s", util.Env.GetNatsURL())
	mainLogger.Info().Msgf("Players Config File: %s", cmdArgs.playersFile)
	mainLogger.Info().Msgf("Game Script File: %s", cmdArgs.scriptFile)
	if cmdArgs.playersFile == "" {
		mainLogger.Error().Msg("No players config file is provided.")
		return 1
	}
	if cmdArgs.scriptFile == "" {
		mainLogger.Error().Msg("No script file is provided.")
		return 1
	}
	players, err := gamescript.ReadPlayersConfig(cmdArgs.playersFile)
	if err != nil {
		mainLogger.Error().Msgf("Error while parsing players file: %+v", err)
		return 1
	}
	script, err := gamescript.ReadGameScript(cmdArgs.scriptFile)
	if err != nil {
		mainLogger.Error().Msgf("Error while parsing script file: %+v", err)
		return 1
	}
	driverLogger := logging.GetZeroLogger("BotRunner", nil)
	playerLogger := logging.GetZeroLogger("BotPlayer", nil)
	botRunner, err := driver.NewBotRunner(cmdArgs.clubCode, cmdArgs.gameCode, script, players, driverLogger, playerLogger, cmdArgs.resetDB, false)
	if err != nil {
		mainLogger.Error().Msgf("Error while creating a bot runner %+v", err)
		return 1
	}
	err = botRunner.Run()
	if err != nil {
		mainLogger.Error().Msgf("Unhandled error from bot runner: %s", err)
		return 1
	}

	return 0
}
