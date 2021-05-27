package main

import (
	"flag"

	"github.com/rs/zerolog/log"
	"voyager.com/timer/rest"
)

var (
	cmdArgs    arg
	mainLogger = log.With().Str("logger_name", "main::main").Logger()
)

type arg struct {
	port uint
}

func init() {
	flag.UintVar(&cmdArgs.port, "port", 8082, "Listen port")
	flag.Parse()
}
func main() {
	mainLogger.Info().Msgf("Port: %d", cmdArgs.port)
	rest.RunServer(cmdArgs.port)
}
