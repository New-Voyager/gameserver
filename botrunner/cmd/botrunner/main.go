package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/rs/zerolog/log"
	"voyager.com/botrunner/internal/driver"
	"voyager.com/botrunner/internal/game"
	"voyager.com/botrunner/internal/util"
)

var (
	cmdArgs    arg
	mainLogger = log.With().Str("logger_name", "main::main").Logger()
)

type arg struct {
	configFile string
	clubCode   string
	gameCode   string
}

func init() {
	flag.StringVar(&cmdArgs.configFile, "config", "", "Botrunner config YAML file")
	flag.StringVar(&cmdArgs.clubCode, "club-code", "", "Club code to use. If not provided, a club will be created and owned by a bot.")
	flag.StringVar(&cmdArgs.gameCode, "game-code", "", "Game code to use. If not provided, a game will be created and started by a bot.")
	flag.Parse()
}

func main() {
	os.Exit(botrunner())
}

func botrunner() int {
	mainLogger.Info().Msg("Config File: " + cmdArgs.configFile)
	if cmdArgs.configFile == "" {
		mainLogger.Error().Msg("No config file is provided.")
		return 1
	}
	fmt.Printf("Nats url: %s", util.Env.GetNatsURL())
	config, err := game.ParseYAMLConfig(cmdArgs.configFile)
	if err != nil {
		mainLogger.Error().Msgf("Error while parsing config file: %+v", err)
		return 1
	}
	driverLogger := log.With().Str("logger_name", "BotRunner").Logger()
	playerLogger := log.With().Str("logger_name", "BotPlayer").Logger()
	botRunner, err := driver.NewBotRunner(cmdArgs.clubCode, cmdArgs.gameCode, *config, &driverLogger, &playerLogger)
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
