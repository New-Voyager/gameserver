package timer

import (
	"fmt"
	"net/http"
)

var APIServerUrl string

func notifyTimeout(t *Timer) {
	timerLogger.Info().Msgf("Notifying timeout %d|%d|%s", t.gameID, t.playerID, t.purpose)
	url := fmt.Sprintf("%s/internal/timer-callback/gameId/%d/playerId/%d/purpose/%s", APIServerUrl, t.gameID, t.playerID, t.purpose)
	resp, _ := http.Post(url, "application/json", nil)
	if resp.StatusCode != 200 {
		timerLogger.Fatal().Uint64("game", t.gameID).Msg(fmt.Sprintf("Failed to call timer callback. Error: %d", resp.StatusCode))
	}
}
