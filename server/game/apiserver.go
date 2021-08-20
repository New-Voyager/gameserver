package game

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
)

func (g *Game) saveHandResult2ToAPIServer(result2 *HandResultServer) (*SaveHandResult, error) {
	// call the API server to save the hand result
	var m protojson.MarshalOptions
	m.EmitUnpopulated = true
	data, _ := m.Marshal(result2)
	fmt.Printf("%s\n", string(data))
	url := fmt.Sprintf("%s/internal/save-hand/gameId/%d/handNum/%d", g.apiServerURL, result2.GameId, result2.HandNum)
	retries := 0
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	for err != nil && retries < int(g.maxRetries) {
		retries++
		channelGameLogger.Error().Msgf("Error in post %s: %s. Retrying (%d/%d)", url, err, retries, g.maxRetries)
		time.Sleep(time.Duration(g.retryDelayMillis) * time.Millisecond)
		resp, err = http.Post(url, "application/json", bytes.NewBuffer(data))
	}
	// if the api server returns nil, do nothing
	if resp == nil {
		return nil, fmt.Errorf("saving hand failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		bodyBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			channelGameLogger.Error().Msgf("Failed to read save result for hand num: %d", result2.HandNum)
		}
		var saveResult SaveHandResult
		json.Unmarshal(bodyBytes, &saveResult)
		return &saveResult, nil
	} else {
		return nil, fmt.Errorf("faile to save hand")
	}
}
func (g *Game) getNewHandInfo() (*NewHandInfo, error) {
	// TODO: Implement retry.

	url := fmt.Sprintf("%s/internal/next-hand-info/game_num/%s", g.apiServerURL, g.config.GameCode)

	retries := 0
	resp, err := http.Get(url)
	for err != nil && retries < int(g.maxRetries) {
		retries++
		fmt.Printf("Error in get %s: %s. Retrying (%d/%d)", url, err, retries, g.maxRetries)
		time.Sleep(time.Duration(g.retryDelayMillis) * time.Millisecond)
		resp, err = http.Get(url)
	}

	// if the api server returns nil, do nothing
	if resp == nil {
		return nil, fmt.Errorf("[%s] Cannot get new hand information", g.config.GameCode)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		bodyBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			channelGameLogger.Error().Msgf("[%s] Cannot get new hand information", g.config.GameCode)
		}
		var newHandInfo NewHandInfo
		json.Unmarshal(bodyBytes, &newHandInfo)
		return &newHandInfo, nil
	}
	return nil, fmt.Errorf("[%s] Cannot get new hand information", g.config.GameCode)
}

func (g *Game) moveAPIServerToNextHand(gameServerHandNum uint32) error {
	// TODO: Implement retry.

	url := fmt.Sprintf("%s/internal/move-to-next-hand/game_num/%s/hand_num/%d", g.apiServerURL, g.config.GameCode, gameServerHandNum)

	retries := 0
	resp, err := http.Post(url, "text/plain", bytes.NewBuffer([]byte{}))
	for err != nil && retries < int(g.maxRetries) {
		retries++
		channelGameLogger.Error().Msgf("Error in post %s: %s. Retrying (%d/%d)", url, err, retries, g.maxRetries)
		time.Sleep(time.Duration(g.retryDelayMillis) * time.Millisecond)
		resp, err = http.Post(url, "text/plain", bytes.NewBuffer([]byte{}))
	}

	// if the api server returns nil, do nothing
	if resp == nil {
		return fmt.Errorf("[%s] Cannot move API server to next hand", g.config.GameCode)
	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		channelGameLogger.Error().Msgf("[%s] Cannot read response from /move-to-next-hand", g.config.GameCode)
		return err
	}
	channelGameLogger.Info().Msgf("[%s] Response from /move-to-next-hand: %s", g.config.GameCode, bodyBytes)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("[%s] /move-to-next-hand returned %d", g.config.GameCode, resp.StatusCode)
	}
	return nil
}
