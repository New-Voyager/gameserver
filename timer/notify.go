package timer

import (
	"fmt"
	"net/http"
	"time"
)

var APIServerUrl string

func notifyTimeout(t *Timer) {
	timerLogger.Info().Msgf("Notifying timeout %d|%d|%s", t.gameID, t.playerID, t.purpose)
	url := fmt.Sprintf("%s/internal/timer-callback/gameId/%d/playerId/%d/purpose/%s", APIServerUrl, t.gameID, t.playerID, t.purpose)
	retry := true
	for retry {
		resp, _ := http.Post(url, "application/json", nil)
		if resp == nil {
			// sleep for 5 seconds and retry
			time.Sleep(5 * time.Second)
			continue
		}

		if resp.StatusCode != 200 {
			timerLogger.Fatal().Uint64("game", t.gameID).Msg(fmt.Sprintf("Failed to call timer callback. Error: %d", resp.StatusCode))
		}
		retry = false
	}
}
