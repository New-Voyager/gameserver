package timer

import (
	"fmt"
	"net/http"
	"time"

	"voyager.com/logging"
	"voyager.com/timer/internal/util"
)

var (
	APIServerURL     = util.Env.GetAPIServerInternalURL()
	maxRetries       = 3
	retryDelayMillis = 2000
)

func notifyTimeout(t *Timer) {
	timerLogger.Info().
		Uint64(logging.GameIDKey, t.gameID).
		Uint64(logging.PlayerIDKey, t.playerID).
		Str(logging.TimerPurposeKey, t.purpose).
		Msgf("Notifying timeout")
	url := fmt.Sprintf("%s/internal/timer-callback/gameId/%d/playerId/%d/purpose/%s", APIServerURL, t.gameID, t.playerID, t.purpose)

	retries := 0
	resp, err := http.Post(url, "application/json", nil)
	for err != nil && retries < int(maxRetries) {
		retries++
		timerLogger.Error().
			Uint64(logging.GameIDKey, t.gameID).
			Uint64(logging.PlayerIDKey, t.playerID).
			Str(logging.TimerPurposeKey, t.purpose).
			Msgf("Error in post %s: %s. Retrying (%d/%d)", url, err, retries, maxRetries)
		time.Sleep(time.Duration(retryDelayMillis) * time.Millisecond)
		resp, err = http.Post(url, "application/json", nil)
	}

	if err != nil {
		timerLogger.Error().
			Uint64(logging.GameIDKey, t.gameID).
			Uint64(logging.PlayerIDKey, t.playerID).
			Str(logging.TimerPurposeKey, t.purpose).
			Msgf("Retry exhausted for post %s", url)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		timerLogger.Error().
			Uint64(logging.GameIDKey, t.gameID).
			Uint64(logging.PlayerIDKey, t.playerID).
			Str(logging.TimerPurposeKey, t.purpose).
			Msgf("Received http status %d from %s", resp.StatusCode, url)
	}
}
