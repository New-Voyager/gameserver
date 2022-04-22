package scheduler

import (
	"time"

	"voyager.com/logging"
)

var (
	lobbyGamesLogger = logging.GetZeroLogger("scheduler::lobby_games", nil)
)

func LobbyGames() {
	for {
		lobbyGamesLogger.Debug().Msg("Starting lobby games")
		start := time.Now()
		_, err := requestStartLobbyGames()
		duration := time.Since(start)
		if err == nil {
			lobbyGamesLogger.Debug().Msgf("Starting lobby games took %.3f seconds", duration.Seconds())
			break
		}
		lobbyGamesLogger.Error().Msgf("Error while starting lobby games: %s", err)
		time.Sleep(5 * time.Second)
	}
}
