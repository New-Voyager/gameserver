package main

import (
	"bytes"
	"flag"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
	"voyager.com/timer/internal/util"
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
	go rest.RunServer(cmdArgs.port)

	time.Sleep(1 * time.Second)
	apiServerURL := util.Env.GetAPIServerURL()
	mainLogger.Info().Msg("Requesting to restart the active games.")
	waitForAPIServer(apiServerURL)
	requestRestartTimers(apiServerURL)

	select {}
}

func waitForAPIServer(apiServerURL string) {
	readyURL := fmt.Sprintf("%s/internal/ready", apiServerURL)
	client := http.Client{Timeout: 2 * time.Second}
	for {
		mainLogger.Info().Msgf("Checking API server ready")
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
		mainLogger.Fatal().Msg(fmt.Sprintf("Failed to restart timers. Error: %s", err.Error()))
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		mainLogger.Fatal().Msg(fmt.Sprintf("Failed to restart timers. Error: %d", resp.StatusCode))
	}
}
