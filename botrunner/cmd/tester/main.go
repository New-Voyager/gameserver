package main

import (
	"flag"
	"os"

	"github.com/rs/zerolog/log"
	"voyager.com/botrunner/cmd/tester/app"
	"voyager.com/botrunner/internal/game"
)

var (
	cmdArgs    arg
	mainLogger = log.With().Str("logger_name", "main::main").Logger()
)

type arg struct {
	gameCode   string
	configFile string
}

func init() {
	flag.StringVar(&cmdArgs.gameCode, "game-code", "", "Game code to join")
	flag.StringVar(&cmdArgs.configFile, "config", "", "Botrunner & tester config YAML file")
	flag.Parse()
}

func main() {
	os.Exit(tester())
}

func tester() int {
	mainLogger.Info().Msg("Game Code: " + cmdArgs.gameCode)
	mainLogger.Info().Msg("Config File: " + cmdArgs.configFile)
	if cmdArgs.gameCode == "" {
		mainLogger.Error().Msg("No game code is provided.")
		return 1
	}
	if cmdArgs.configFile == "" {
		mainLogger.Error().Msg("No config file is provided.")
		return 1
	}
	config, err := game.ParseYAMLConfig(cmdArgs.configFile)
	if err != nil {
		mainLogger.Error().Msgf("Error while parsing config file: %+v", err)
		return 1
	}

	t, err := app.NewTester(*config, cmdArgs.gameCode)
	if err != nil {
		mainLogger.Error().Msgf("Error while creating a tester instance %+v", err)
		return 1
	}
	err = t.Run()
	if err != nil {
		mainLogger.Error().Msgf("Unhandled error from tester %+v", err)
		return 1
	}

	return 0
}
