package game

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"
)

type moveToNextHandResp struct {
	GameCode    string
	HandNum     int
	GameStatus  GameStatus
	TableStatus TableStatus
}

func (g *Game) saveHandResult2ToAPIServer(result2 *HandResultServer) (*SaveHandResult, error) {
	// call the API server to save the hand result
	var m protojson.MarshalOptions
	m.EmitUnpopulated = true
	data, _ := m.Marshal(result2)
	channelGameLogger.Debug().
		Str("game", g.gameCode).
		Msgf("Result to API server: %s", string(data))
	url := fmt.Sprintf("%s/internal/save-hand/gameId/%d/handNum/%d", g.apiServerURL, result2.GameId, result2.HandNum)
	retries := 0
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	for err != nil && retries < int(g.maxRetries) {
		retries++
		channelGameLogger.Error().
			Str("game", g.gameCode).
			Msgf("Error in post %s: %s. Retrying (%d/%d)", url, err, retries, g.maxRetries)
		time.Sleep(time.Duration(g.retryDelayMillis) * time.Millisecond)
		resp, err = http.Post(url, "application/json", bytes.NewBuffer(data))
	}
	// if the api server returns nil, do nothing
	if err != nil {
		return nil, errors.Wrapf(err, "Error from post %s", url)
	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrapf(err, "Unable to read response body from %s", url)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Received HTTP status %d from %s. Response body: %s", resp.StatusCode, url, string(bodyBytes))
	}

	var saveResult SaveHandResult
	err = json.Unmarshal(bodyBytes, &saveResult)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to parse response body json into struct")
	}
	return &saveResult, nil
}

func (g *Game) getNewHandInfo() (*NewHandInfo, error) {
	url := fmt.Sprintf("%s/internal/next-hand-info/game_num/%s", g.apiServerURL, g.gameCode)

	retries := 0
	resp, err := http.Get(url)
	for err != nil && retries < int(g.maxRetries) {
		retries++
		channelGameLogger.Error().
			Str("game", g.gameCode).
			Msgf("Error in get %s: %s. Retrying (%d/%d)", url, err, retries, g.maxRetries)
		time.Sleep(time.Duration(g.retryDelayMillis) * time.Millisecond)
		resp, err = http.Get(url)
	}

	// if the api server returns nil, do nothing
	if resp == nil {
		return nil, fmt.Errorf("[%s] Cannot get new hand information", g.gameCode)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		bodyBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			channelGameLogger.Error().
				Str("game", g.gameCode).
				Msgf("[%s] Cannot get new hand information", g.gameCode)
		}
		var newHandInfo NewHandInfo
		json.Unmarshal(bodyBytes, &newHandInfo)
		return &newHandInfo, nil
	}
	return nil, fmt.Errorf("[%s] Cannot get new hand information", g.gameCode)
}

func (g *Game) moveAPIServerToNextHand(gameServerHandNum uint32) (moveToNextHandResp, error) {
	url := fmt.Sprintf("%s/internal/move-to-next-hand/game_num/%s/hand_num/%d", g.apiServerURL, g.gameCode, gameServerHandNum)

	channelGameLogger.Debug().
		Str("game", g.gameCode).
		Msgf("Calling /move-to-next-hand with current hand number %d", gameServerHandNum)
	retries := 0
	resp, err := http.Post(url, "text/plain", bytes.NewBuffer([]byte{}))
	for err != nil && retries < int(g.maxRetries) {
		retries++
		channelGameLogger.Error().
			Str("game", g.gameCode).
			Msgf("Error in post %s: %s. Retrying (%d/%d)", url, err, retries, g.maxRetries)
		time.Sleep(time.Duration(g.retryDelayMillis) * time.Millisecond)
		resp, err = http.Post(url, "text/plain", bytes.NewBuffer([]byte{}))
	}

	// if the api server returns nil, do nothing
	if resp == nil {
		return moveToNextHandResp{}, fmt.Errorf("Nil response received from api server /move-to-next-hand")
	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		msg := "Cannot read response from /move-to-next-hand"
		channelGameLogger.Error().
			Str("game", g.gameCode).
			Msgf(msg)
		return moveToNextHandResp{}, errors.Wrap(err, msg)
	}
	channelGameLogger.Debug().
		Str("game", g.gameCode).
		Msgf("Response from /move-to-next-hand: %s", bodyBytes)

	if resp.StatusCode != http.StatusOK {
		return moveToNextHandResp{}, fmt.Errorf("/move-to-next-hand returned HTTP status %d", resp.StatusCode)
	}

	var body moveToNextHandResp
	err = json.Unmarshal(bodyBytes, &body)
	if err != nil {
		return moveToNextHandResp{}, errors.Wrap(err, "Unable to unmarshal json body")
	}

	return body, nil
}
