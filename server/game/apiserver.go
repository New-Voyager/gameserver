package game

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"google.golang.org/protobuf/encoding/protojson"
)

func (g *Game) saveHandResultToAPIServer(result *HandResult) (*SaveHandResult, error) {
	// call the API server to save the hand result
	var m protojson.MarshalOptions
	m.EmitUnpopulated = true
	data, _ := m.Marshal(result)
	fmt.Printf("%s\n", string(data))

	url := fmt.Sprintf("%s/internal/post-hand/gameId/%d/handNum/%d", g.apiServerUrl, result.GameId, result.HandNum)
	resp, _ := http.Post(url, "application/json", bytes.NewBuffer(data))
	// if the api server returns nil, do nothing
	if resp == nil {
		return nil, fmt.Errorf("Saving hand failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		bodyBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			channelGameLogger.Error().Msgf("Failed to read save result for hand num: %d", result.HandNum)
		}
		bodyString := string(bodyBytes)
		fmt.Printf(bodyString)
		fmt.Printf("\n")
		fmt.Printf("Posted successfully")

		var saveResult SaveHandResult
		json.Unmarshal(bodyBytes, &saveResult)
		return &saveResult, nil
	} else {
		return nil, fmt.Errorf("faile to save hand")
	}
}

func (g *Game) getNewHandInfo() (*NewHandInfo, error) {
	// TODO: Implement retry.

	url := fmt.Sprintf("%s/internal/next-hand-info/game_num/%s", g.apiServerUrl, g.config.GameCode)

	resp, _ := http.Get(url)
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

	url := fmt.Sprintf("%s/internal/move-to-next-hand/game_num/%s/hand_num/%d", g.apiServerUrl, g.config.GameCode, gameServerHandNum)

	resp, _ := http.Post(url, "text/plain", bytes.NewBuffer([]byte{}))
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
