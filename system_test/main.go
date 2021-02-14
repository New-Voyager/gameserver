package main

import (
	"flag"
	"fmt"
	"os"

	"voyager.com/test/internal"
)

const (
	gameServerExec = "./server"
	botRunnerExec  = "./botrunner"
)

var (
	gameServerDir *string
	botRunnerDir  *string
	configFile    *string
)

func main() {
	gameServerDir = flag.String("game-server-dir", "", "Game server directory")
	botRunnerDir = flag.String("botrunner-dir", "", "Botrunner directory")
	configFile = flag.String("config", "", "Test config YAML file")

	flag.Parse()
	errors := make([]string, 0)
	fmt.Printf("Game Server Dir : %s\n", *gameServerDir)
	fmt.Printf("Bot Runner Dir  : %s\n", *botRunnerDir)
	if *gameServerDir == "" {
		errors = append(errors, "--game-server-dir is required")
	}
	if *botRunnerDir == "" {
		errors = append(errors, "--botrunner-dir is required")
	}

	if len(errors) > 0 {
		for _, e := range errors {
			fmt.Println(e)
		}
		os.Exit(1)
	}

	testConfig, err := internal.ReadConfig(*configFile)
	if err != nil {
		fmt.Printf("Error while reading test config: %s", err)
	}

	for _, testCase := range testConfig.Tests {
		fmt.Printf("Executing test case %+v", testCase)
		t := internal.NewStandardTest(*gameServerDir, gameServerExec, *botRunnerDir, botRunnerExec, testCase.Name, testCase.Script, testCase.Timeout, testCase.ExpectedMsgsFile, testCase.MsgDumpFile)
		err := t.Run()
		if err != nil {
			panic(err)
		}
	}
}
