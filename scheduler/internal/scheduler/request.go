package scheduler

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/pkg/errors"
	"voyager.com/logging"
	"voyager.com/scheduler/internal/util"
)

var (
	requestLogger         = logging.GetZeroLogger("scheduler::request", nil)
	APIServerURL          = util.Env.GetAPIServerInternalURL()
	generalHttpTimeoutSec = 10 * time.Second
)

func requestPostProcess(gameID uint64) ([]uint64, bool, error) {
	requestLogger.Info().
		Uint64(logging.GameIDKey, gameID).
		Msgf("Requesting post-processing")
	httpClient := &http.Client{
		Timeout: time.Duration(util.Env.GetPostProcessingTimeoutSec()) * time.Second,
	}
	path := "/admin/post-process-games"
	url := fmt.Sprintf("%s%s", APIServerURL, path)
	retries := 0
	resp, err := httpClient.Post(url, "application/json", nil)
	for err != nil && retries < int(maxRetries) {
		retries++
		requestLogger.Error().Msgf("Error in post %s: %s. Retrying (%d/%d)", url, err, retries, maxRetries)
		time.Sleep(time.Duration(retryDelayMillis) * time.Millisecond)
		resp, err = httpClient.Post(url, "application/json", nil)
	}

	if err != nil {
		return nil, false, errors.Wrap(err, "Error from http post")
	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, false, errors.Wrapf(err, "Cannot read response from %s", url)
	}
	requestLogger.Debug().
		Msgf("Response from %s: %s", url, string(bodyBytes))

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("%s returned HTTP status %d", url, resp.StatusCode)
	}

	type postProcessResp struct {
		Aggregated []uint64 `json:"aggregated"`
		More       bool     `json:"more"`
	}
	var body postProcessResp
	err = json.Unmarshal(bodyBytes, &body)
	if err != nil {
		return nil, false, errors.Wrap(err, "Unable to unmarshal json body")
	}

	return body.Aggregated, body.More, nil
}

func requestEndExpiredGames() (uint32, error) {
	requestLogger.Info().Msgf("Requesting to end expired games.")
	httpClient := &http.Client{
		Timeout: time.Duration(util.Env.GetExpireGamesTimeoutSec()) * time.Second,
	}
	path := "/internal/end-expired-games"
	url := fmt.Sprintf("%s%s", APIServerURL, path)
	retries := 0
	resp, err := httpClient.Post(url, "application/json", nil)
	for err != nil && retries < int(maxRetries) {
		retries++
		requestLogger.Error().Msgf("Error in post %s: %s. Retrying (%d/%d)", url, err, retries, maxRetries)
		time.Sleep(time.Duration(retryDelayMillis) * time.Millisecond)
		resp, err = httpClient.Post(url, "application/json", nil)
	}

	if err != nil {
		return 0, errors.Wrap(err, "Error from http post")
	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, errors.Wrapf(err, "Cannot read response from %s", url)
	}
	requestLogger.Debug().
		Msgf("Response from %s: %s", url, string(bodyBytes))

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("%s returned HTTP status %d", url, resp.StatusCode)
	}

	type expireGamesResp struct {
		Expired uint32 `json:"expired"`
	}
	var body expireGamesResp
	err = json.Unmarshal(bodyBytes, &body)
	if err != nil {
		return 0, errors.Wrap(err, "Unable to unmarshal json body")
	}

	return body.Expired, nil
}

type dataRetentionResp struct {
	HandHistory uint32 `json:"handHistory"`
}

func requestDataRetention() (dataRetentionResp, error) {
	requestLogger.Info().Msgf("Requesting data retention.")
	httpClient := &http.Client{
		Timeout: time.Duration(util.Env.GetDataRetentionTimeoutMin()) * time.Minute,
	}
	path := "/admin/data-retention"
	url := fmt.Sprintf("%s%s", APIServerURL, path)
	resp, err := httpClient.Post(url, "application/json", nil)

	if err != nil {
		return dataRetentionResp{}, errors.Wrap(err, "Error from http post")
	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return dataRetentionResp{}, errors.Wrapf(err, "Cannot read response from %s", url)
	}
	requestLogger.Debug().
		Msgf("Response from %s: %s", url, string(bodyBytes))

	if resp.StatusCode != http.StatusOK {
		return dataRetentionResp{}, fmt.Errorf("%s returned HTTP status %d", url, resp.StatusCode)
	}

	var body dataRetentionResp
	err = json.Unmarshal(bodyBytes, &body)
	if err != nil {
		return dataRetentionResp{}, errors.Wrap(err, "Unable to unmarshal json body")
	}

	return body, nil
}

type lobbyGamesResp struct{}

func requestStartLobbyGames() (lobbyGamesResp, error) {
	requestLogger.Info().Msgf("Requesting to start lobby games.")
	httpClient := &http.Client{
		Timeout: generalHttpTimeoutSec,
	}
	path := "/internal/refresh-lobby-games"
	url := fmt.Sprintf("%s%s", APIServerURL, path)
	resp, err := httpClient.Post(url, "application/json", nil)

	if err != nil {
		return lobbyGamesResp{}, errors.Wrap(err, "Error from http post")
	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return lobbyGamesResp{}, errors.Wrapf(err, "Cannot read response from %s", url)
	}
	requestLogger.Debug().
		Msgf("Response from %s: %s", url, string(bodyBytes))

	if resp.StatusCode != http.StatusOK {
		return lobbyGamesResp{}, fmt.Errorf("%s returned HTTP status %d", url, resp.StatusCode)
	}

	var body lobbyGamesResp
	err = json.Unmarshal(bodyBytes, &body)
	if err != nil {
		return lobbyGamesResp{}, errors.Wrap(err, "Unable to unmarshal json body")
	}

	return body, nil
}
