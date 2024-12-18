package main

import (
	"bytes"
	"flag"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog"
	"voyager.com/logging"
	"voyager.com/timer/internal/util"
	"voyager.com/timer/rest"
)

var (
	cmdArgs    arg
	mainLogger = logging.GetZeroLogger("main::main", nil)
)

type arg struct {
	port uint
}

func init() {
	flag.UintVar(&cmdArgs.port, "port", 8082, "Listen port")
	flag.Parse()
}
func main() {
	logLevel := util.Env.GetZeroLogLogLevel()
	fmt.Printf("Setting log level to %s\n", logLevel)
	zerolog.SetGlobalLevel(logLevel)

	mainLogger.Info().Msgf("Port: %d", cmdArgs.port)
	go rest.RunServer(cmdArgs.port)

	time.Sleep(1 * time.Second)
	apiServerURL := util.Env.GetAPIServerInternalURL()
	waitForAPIServer(apiServerURL)
	mainLogger.Info().Msg("Requesting to restart the active timers.")
	requestRestartTimers(apiServerURL)

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

func requestRestartTimers(apiServerURL string) {
	var err error

	restartURL := fmt.Sprintf("%s/internal/restart-timers", apiServerURL)
	resp, err := http.Post(restartURL, "application/json", bytes.NewBuffer([]byte{}))
	if err != nil {
		mainLogger.Fatal().Msgf("Failed to restart timers. Error: %s", err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		mainLogger.Fatal().Msgf("Failed to restart timers. Received http status %d from %s", resp.StatusCode, restartURL)
	}
}
