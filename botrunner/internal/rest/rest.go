package rest

import (
	"fmt"
	"io/ioutil"
	"net/http"
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
