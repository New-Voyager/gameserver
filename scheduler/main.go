package main

import (
	"flag"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"voyager.com/scheduler/internal/scheduler"
	"voyager.com/scheduler/internal/util"
	"voyager.com/scheduler/rest"
)

var (
	cmdArgs    arg
	mainLogger = log.With().Str("logger_name", "main::main").Logger()
)

type arg struct {
	port uint
}

func init() {
	flag.UintVar(&cmdArgs.port, "port", 8083, "Listen port")
	flag.Parse()
}

func main() {
	logLevel := util.Env.GetZeroLogLogLevel()
	fmt.Printf("Setting log level to %s\n", logLevel)
	zerolog.SetGlobalLevel(logLevel)

	apiServerURL := util.Env.GetAPIServerInternalURL()
	waitForAPIServer(apiServerURL)

	time.Sleep(1 * time.Second)
	mainLogger.Info().Msgf("Starting scheduler server. Port: %d", cmdArgs.port)
	go rest.RunServer(cmdArgs.port)

	// Start the periodic cleanup of expired games.
	go scheduler.CleanUpExpiredGames()

	// Periodic clean up of data.
	go scheduler.DataRetention()

	select {}
}

func waitForAPIServer(apiServerURL string) {
	readyURL := fmt.Sprintf("%s/internal/ready", apiServerURL)
	client := http.Client{Timeout: 2 * time.Second}
	for {
		mainLogger.Info().Msgf("Checking API server ready (%s)", readyURL)
		resp, err := client.Get(readyURL)
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			break
		}
		if err == nil {
			mainLogger.Error().Msgf("%s returend %d", readyURL, resp.StatusCode)
			resp.Body.Close()
		}
		time.Sleep(1 * time.Second)
	}
}
