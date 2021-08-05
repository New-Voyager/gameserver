package rest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"voyager.com/gamescript"
)

type RestClient struct {
	url        string
	timeoutSec uint32
	authToken  string
}

func NewRestClient(url string, timeoutSec uint32, authToken string) *RestClient {
	return &RestClient{
		url:        url,
		timeoutSec: timeoutSec,
		authToken:  authToken,
	}
}

func (rc *RestClient) UpdateButtonPos(gameCode string, buttonPos uint32) error {
	url := fmt.Sprintf("%s/game-code/%s/button-pos/%d", rc.url, gameCode, buttonPos)
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		//handle read response error
		return nil
	}

	fmt.Printf("%s\n", string(body))
	return nil
}

func (rc *RestClient) SetServerSettings(serverSettings *gamescript.ServerSettings) error {
	var reqData []byte
	var err error
	reqData, err = json.Marshal(serverSettings)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/server-settings", rc.url)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(reqData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (rc *RestClient) ResetServerSettings() error {
	url := fmt.Sprintf("%s/reset-server-settings", rc.url)
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (rc *RestClient) BuyAppCoins(playerUuid string, amount int) error {
	type BuyAppCoinStruct struct {
		PlayerUuid string `json:"player-uuid"`
		Amount     int    `json:"coins"`
	}
	payload := BuyAppCoinStruct{
		PlayerUuid: playerUuid,
		Amount:     int(amount),
	}
	var reqData []byte
	var err error
	reqData, err = json.Marshal(payload)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/buy-bot-coins", rc.url)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(reqData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
