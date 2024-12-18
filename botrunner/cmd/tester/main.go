package main

import (
	"flag"
	"fmt"
	"os"

	_ "github.com/lib/pq"
	"github.com/rs/zerolog"
	"voyager.com/botrunner/cmd/tester/app"
	"voyager.com/botrunner/internal/util"
	"voyager.com/gamescript"
	"voyager.com/logging"
)

var (
	cmdArgs    arg
	mainLogger = logging.GetZeroLogger("main::main", nil)
)

type arg struct {
	gameCode    string
	playersFile string
	scriptFile  string
}

func init() {
	flag.StringVar(&cmdArgs.gameCode, "game-code", "", "Game code to join")
	flag.StringVar(&cmdArgs.scriptFile, "script", "", "Game script YAML file")
	flag.StringVar(&cmdArgs.playersFile, "players", "botrunner_scripts/players/default.yaml", "Players YAML file")
	flag.Parse()
}

func main() {
	os.Exit(tester())
}

func tester() int {
	logLevel := util.Env.GetZeroLogLogLevel()
	fmt.Printf("Setting log level to %s\n", logLevel)
	zerolog.SetGlobalLevel(logLevel)
	mainLogger.Info().Msg("Game Code: " + cmdArgs.gameCode)
	mainLogger.Info().Msg("Players File: " + cmdArgs.playersFile)
	mainLogger.Info().Msg("Script File: " + cmdArgs.scriptFile)
	if cmdArgs.gameCode == "" {
		mainLogger.Error().Msg("No game code is provided.")
		return 1
	}
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
	t, err := app.NewTester(players, script, cmdArgs.gameCode)
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
