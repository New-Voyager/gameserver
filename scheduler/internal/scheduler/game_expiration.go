package scheduler

import (
	"fmt"
	"time"

	"voyager.com/logging"
	"voyager.com/scheduler/internal/util"
)

var (
	expireGamesLogger = logging.GetZeroLogger("scheduler::game_expiration", nil)
)

func CleanUpExpiredGames() {
	interval := time.Duration(util.Env.GetExpireGamesIntervalSec()) * time.Second
	for range time.NewTicker(interval).C {
		start := time.Now()
		numExpiredGames, err := requestEndExpiredGames()
		duration := time.Since(start)
		expireGamesLogger.Debug().Msgf("Request to end expired games took %.3f seconds", duration.Seconds())

		if err != nil {
			expireGamesLogger.Error().Msgf("Error while ending expired games: %s", err)
		} else {
			msg := fmt.Sprintf("Expired %d game(s).", numExpiredGames)
			if numExpiredGames == 0 {
				expireGamesLogger.Debug().Msg(msg)
			} else {
				expireGamesLogger.Info().Msg(msg)
			}
		}
	}
}
