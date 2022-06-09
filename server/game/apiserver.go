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

type endGameResp struct {
	Status string
	Error  string
}

func (g *Game) saveHandResult2ToAPIServer(result2 *HandResultServer) (*SaveHandResult, error) {
	// call the API server to save the hand result
	var m protojson.MarshalOptions
	m.EmitUnpopulated = true
	data, _ := m.Marshal(result2)
	g.logger.Debug().Msgf("Result to API server: %s", string(data))
	url := fmt.Sprintf("%s/internal/save-hand/gameId/%d/handNum/%d", g.apiServerURL, result2.GameId, result2.HandNum)
	retries := 0
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	for err != nil && retries < int(g.maxRetries) {
		retries++
		g.logger.Error().
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

func (g *Game) saveTournamentHandResult2ToAPIServer(tournamentURL string, result2 *HandResultServer) (*SaveHandResult, error) {
	// call the API server to save the hand result
	var m protojson.MarshalOptions
	m.EmitUnpopulated = true
	data, _ := m.Marshal(result2)
	g.logger.Debug().Msgf("Result to API server: %s", string(data))
	url := fmt.Sprintf("%s/internal/save-hand/tournamentId/%d/tableNo/%d", tournamentURL, g.tournamentID, g.tableNo)
	retries := 0
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	for err != nil && retries < int(g.maxRetries) {
		retries++
		g.logger.Error().
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
		g.logger.Error().
			Msgf("Error in get %s: %s. Retrying (%d/%d)", url, err, retries, g.maxRetries)
		time.Sleep(time.Duration(g.retryDelayMillis) * time.Millisecond)
		resp, err = http.Get(url)
	}

	if err != nil {
		return nil, errors.Wrapf(err, "Error from http get %s", url)
	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Error while reading response body from %s", url)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Received http status %d from %s. Response body: %s", resp.StatusCode, url, string(bodyBytes))
	}

	var newHandInfo NewHandInfo
	err = json.Unmarshal(bodyBytes, &newHandInfo)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not unmarshal response body from %s", url)
	}

	return &newHandInfo, nil
}

func (g *Game) moveAPIServerToNextHand(gameServerHandNum uint32) (moveToNextHandResp, error) {
	url := fmt.Sprintf("%s/internal/move-to-next-hand/game_num/%s/hand_num/%d", g.apiServerURL, g.gameCode, gameServerHandNum)

	g.logger.Debug().
		Msgf("Calling /move-to-next-hand with current hand number %d", gameServerHandNum)
	retries := 0
	resp, err := http.Post(url, "text/plain", bytes.NewBuffer([]byte{}))
	for err != nil && retries < int(g.maxRetries) {
		retries++
		g.logger.Error().
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
		g.logger.Error().
			Msgf(msg)
		return moveToNextHandResp{}, errors.Wrap(err, msg)
	}
	g.logger.Debug().
		Msgf("Response from /move-to-next-hand: %s", string(bodyBytes))

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

func (g *Game) requestEndGame(force bool) (endGameResp, error) {
	forceParam := 0
	if force {
		forceParam = 1
	}
	url := fmt.Sprintf("%s/internal/end-game/%s/force/%d", g.apiServerURL, g.gameCode, forceParam)

	g.logger.Info().Msgf("Calling /end-game")
	retries := 0
	resp, err := http.Post(url, "text/plain", bytes.NewBuffer([]byte{}))
	for err != nil && retries < int(g.maxRetries) {
		retries++
		g.logger.Error().
			Msgf("Error in post %s: %s. Retrying (%d/%d)", url, err, retries, g.maxRetries)
		time.Sleep(time.Duration(g.retryDelayMillis) * time.Millisecond)
		resp, err = http.Post(url, "text/plain", bytes.NewBuffer([]byte{}))
	}

	if err != nil {
		return endGameResp{}, errors.Wrap(err, "Error from /end-game")
	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		msg := "Cannot read response from /end-game"
		g.logger.Error().Msgf(msg)
		return endGameResp{}, errors.Wrap(err, msg)
	}
	g.logger.Info().Msgf("Response from /end-game: %s", string(bodyBytes))

	if resp.StatusCode != http.StatusOK {
		return endGameResp{}, fmt.Errorf("/end-game returned HTTP status %d", resp.StatusCode)
	}

	var body endGameResp
	err = json.Unmarshal(bodyBytes, &body)
	if err != nil {
		return endGameResp{}, errors.Wrap(err, "Unable to unmarshal json body")
	}

	return body, nil
}
