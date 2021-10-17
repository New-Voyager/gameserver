package main

import (
	"flag"
	"fmt"

	_ "github.com/lib/pq"
	"github.com/rs/zerolog"
	"voyager.com/botrunner/cmd/server/app"
	"voyager.com/botrunner/internal/util"
	"voyager.com/logging"
)

var (
	cmdArgs    arg
	mainLogger = logging.GetZeroLogger("main::main", nil)
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
	logLevel := util.Env.GetZeroLogLogLevel()
	fmt.Printf("Setting log level to %s\n", logLevel)
	zerolog.SetGlobalLevel(logLevel)
	mainLogger.Info().Msg("Log Dir:" + cmdArgs.logDir)
	app.RunRestServer(cmdArgs.port, cmdArgs.logDir)
}
