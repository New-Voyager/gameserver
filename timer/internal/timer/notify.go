package timer

import (
	"bytes"
	"encoding/json"
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
		Str(logging.TimerPayloadKey, t.payload).
		Uint64(logging.GameIDKey, t.gameID).
		Uint64(logging.PlayerIDKey, t.playerID).
		Str(logging.TimerPurposeKey, t.purpose).
		Msgf("Notifying timeout")

	var data []byte
	var url string
	if t.payload != "" {
		type payloadStruct struct {
			Payload string `json:"payload"`
		}
		payload := payloadStruct{
			Payload: t.payload,
		}
		var err error
		data, err = json.Marshal(payload)
		if err != nil {
			timerLogger.Error().
				Err(err).
				Str(logging.TimerPayloadKey, t.payload).
				Msgf("Failed to marshal payload")
			return
		}
		url = fmt.Sprintf("%s/internal/timer-callback-generic", APIServerURL)
	} else {
		url = fmt.Sprintf("%s/internal/timer-callback/gameId/%d/playerId/%d/purpose/%s", APIServerURL, t.gameID, t.playerID, t.purpose)
	}

	retries := 0
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	for err != nil && retries < int(maxRetries) {
		retries++
		timerLogger.Error().
			Str(logging.TimerPayloadKey, t.payload).
			Uint64(logging.GameIDKey, t.gameID).
			Uint64(logging.PlayerIDKey, t.playerID).
			Str(logging.TimerPurposeKey, t.purpose).
			Msgf("Error in post %s: %s. Retrying (%d/%d)", url, err, retries, maxRetries)
		time.Sleep(time.Duration(retryDelayMillis) * time.Millisecond)
		resp, err = http.Post(url, "application/json", nil)
	}

	if err != nil {
		timerLogger.Error().
			Str(logging.TimerPayloadKey, t.payload).
			Uint64(logging.GameIDKey, t.gameID).
			Uint64(logging.PlayerIDKey, t.playerID).
			Str(logging.TimerPurposeKey, t.purpose).
			Msgf("Retry exhausted for post %s", url)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		timerLogger.Error().
			Str(logging.TimerPayloadKey, t.payload).
			Uint64(logging.GameIDKey, t.gameID).
			Uint64(logging.PlayerIDKey, t.playerID).
			Str(logging.TimerPurposeKey, t.purpose).
			Msgf("Received http status %d from %s", resp.StatusCode, url)
	}
}
