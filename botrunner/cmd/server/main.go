package main

import (
	"flag"

	_ "github.com/lib/pq"
	"github.com/rs/zerolog/log"
	"voyager.com/botrunner/cmd/server/app"
)

var (
	cmdArgs    arg
	mainLogger = log.With().Str("logger_name", "main::main").Logger()
)

type arg struct {
	logDir string
	port   uint
}

func init() {
	flag.StringVar(&cmdArgs.logDir, "log-dir", "", "Directory to write botrunner logs")
	flag.UintVar(&cmdArgs.port, "port", 8081, "Listen port")
	flag.Parse()
}

func main() {
	mainLogger.Info().Msg("Log Dir: " + cmdArgs.logDir)
	app.RunRestServer(cmdArgs.port, cmdArgs.logDir)
}
