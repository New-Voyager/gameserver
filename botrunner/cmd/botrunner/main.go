package main

import (
	"flag"
	"os"

	"github.com/rs/zerolog/log"
	"voyager.com/botrunner/internal/driver"
	"voyager.com/botrunner/internal/util"

	"voyager.com/gamescript"
)

var (
	cmdArgs    arg
	mainLogger = log.With().Str("logger_name", "main::main").Logger()
)

type arg struct {
	playersFile      string
	scriptFile       string
	clubCode         string
	gameCode         string
	expectedMsgsFile string
	msgDumpFile      string
}

func init() {
	flag.StringVar(&cmdArgs.scriptFile, "script", "", "Game script YAML file")
	flag.StringVar(&cmdArgs.playersFile, "players", "botrunner_scripts/common/players.yaml", "Players YAML file")
	flag.StringVar(&cmdArgs.clubCode, "club-code", "", "Club code to use. If not provided, a club will be created and owned by a bot.")
	flag.StringVar(&cmdArgs.gameCode, "game-code", "", "Game code to use. If not provided, a game will be created and started by a bot.")
	flag.StringVar(&cmdArgs.expectedMsgsFile, "expected-msgs", "", "JSON file containing expected game server messages.")
	flag.StringVar(&cmdArgs.msgDumpFile, "dump-msgs-to", "", "Dump server messages to this file.")
	flag.Parse()
}

func main() {
	os.Exit(botrunner())
}

func botrunner() int {
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
	// config, err := game.ParseYAMLConfig(cmdArgs.scriptFile)
	// if err != nil {
	// 	mainLogger.Error().Msgf("Error while parsing config file: %+v", err)
	// 	return 1
	// }
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
	driverLogger := log.With().Str("logger_name", "BotRunner").Logger()
	playerLogger := log.With().Str("logger_name", "BotPlayer").Logger()
	botRunner, err := driver.NewBotRunner(cmdArgs.clubCode, cmdArgs.gameCode, script, players, &driverLogger, &playerLogger, cmdArgs.expectedMsgsFile, cmdArgs.msgDumpFile)
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
