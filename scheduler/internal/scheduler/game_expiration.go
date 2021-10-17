package scheduler

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"voyager.com/scheduler/internal/util"
)

var (
	expireGamesLogger = log.With().Str("logger_name", "scheduler::game_expiration").Logger()
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
