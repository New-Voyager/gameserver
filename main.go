package main

import (
	"flag"

	"github.com/rs/zerolog"
	"voyager.com/server/test"
)

func main() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	var runGameScript = flag.String("game-script", "test/game-scripts", "runs tests with game script files")
	if *runGameScript != "" {
		test.RunGameScriptTests(*runGameScript)
	}
}
