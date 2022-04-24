package scheduler

import (
	"time"

	"voyager.com/logging"
	"voyager.com/scheduler/internal/util"
)

var (
	lobbyGamesLogger = logging.GetZeroLogger("scheduler::lobby_games", nil)
)

func LobbyGames() {
	refreshLobbyGames()
	interval := time.Duration(util.Env.GetRefreshLobbyGamesIntervalMin()) * time.Minute
	for range time.NewTicker(interval).C {
		refreshLobbyGames()
	}
}

func refreshLobbyGames() {
	for {
		lobbyGamesLogger.Debug().Msg("Refreshing lobby games")
		start := time.Now()
		_, err := requestRefreshLobbyGames()
		duration := time.Since(start)
		if err == nil {
			lobbyGamesLogger.Debug().Msgf("Refreshing lobby games took %.3f seconds", duration.Seconds())
			break
		}
		lobbyGamesLogger.Error().Msgf("Error while refreshing lobby games: %s", err)
		time.Sleep(5 * time.Second)
	}
}
