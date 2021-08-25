package nats

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"voyager.com/server/game"
)

var logger = log.With().Str("logger_name", "server::apiserver").Logger()

// Subscribes to messages coming from apiserver and act on the messages
// that is sent to this game server.
var apiServerUrl = ""

type GameStatus struct {
	GameId      uint64           `json:"gameId"`
	GameStatus  game.GameStatus  `json:"gameStatus"`
	TableStatus game.TableStatus `json:"tableStatus"`
}

type PlayerUpdate struct {
	GameId           uint64            `json:"gameId"`
	PlayerId         uint64            `json:"playerId"`
	SeatNo           uint64            `json:"seatNo"`
	Stack            float64           `json:"Stack"`
	Status           game.PlayerStatus `json:"status"`
	BuyIn            float64           `json:"buyIn"`
	GameToken        string            `json:"gameToken"`
	OldSeatNo        uint64            `json:"oldSeatNo"`
	NewUpdate        game.NewUpdate    `json:"newUpdate"`
	MuckLosingHand   bool              `json:"muckLosinghand"`
	RunItTwicePrompt bool              `json:"runItTwicePrompt"`
}

type PlayerConfigUpdate struct {
	GameId           uint64 `json:"gameId"`
	PlayerId         uint64 `json:"playerId"`
	MuckLosingHand   bool   `json:"muckLosinghand"`
	RunItTwicePrompt bool   `json:"runItTwicePrompt"`
}

type TableUpdate struct {
	GameId                  uint64       `json:"gameId"`
	SeatNo                  uint64       `json:"seatNo"`
	Type                    string       `json:"type"`
	SeatChangePlayers       []uint64     `json:"seatChangePlayers"`
	SeatChangeSeatNos       []uint64     `json:"seatChangeSeatNos"`
	SeatChangeRemainingTime uint32       `json:"seatChangeRemainingTime"`
	WaitlistRemainingTime   uint32       `json:"waitlistRemainingTime"`
	WaitlistPlayerId        uint64       `json:"waitlistPlayerId"`
	WaitlistPlayerUuid      string       `json:"waitlisttPlayerUuid"`
	WaitlistPlayerName      string       `json:"waitlistPlayerName"`
	SeatMoves               []SeatMove   `json:"seatMoves"`
	SeatUpdates             []SeatUpdate `json:"seatUpdates"`
	SeatChangeHostId        uint64       `json:"seatChangeHostId"`
}

type SeatMove struct {
	PlayerId   uint64  `json:"playerId"`
	PlayerUuid string  `json:"playerUuid"`
	Name       string  `json:"name"`
	Stack      float64 `json:"stack"`
	OldSeatNo  uint32  `json:"oldSeatNo"`
	NewSeatNo  uint32  `json:"newSeatNo"`
}

type SeatUpdate struct {
	SeatNo       uint32            `json:"seatNo"`
	PlayerId     uint64            `json:"playerId"`
	PlayerUuid   string            `json:"playerUuid"`
	Name         string            `json:"name"`
	Stack        float64           `json:"stack"`
	PlayerStatus game.PlayerStatus `json:"playerStatus"`
	OpenSeat     bool              `json:"openSeat"`
}

// RegisterGameServer registers game server with the API server
func RegisterGameServer(url string, gameManager *GameManager) {
	apiServerUrl = url
	registerGameServer()
}

// RequestRestartGames requests api server to restart the games that were running on this game server
// before crash.
func RequestRestartGames(apiServerURL string) error {
	return requestRestartGames(apiServerURL)
}

func UpdateTableStatus(gameID uint64, status game.TableStatus, maxRetries uint32, retryDelayMillis uint32) error {
	// update table status
	var reqData []byte
	var err error
	payload := map[string]interface{}{"gameId": gameID, "status": status}
	reqData, err = json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/internal/update-table-status", apiServerUrl)

	retries := 0
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(reqData))
	for err != nil && retries < int(maxRetries) {
		retries++
		logger.Error().Msgf("Error in post %s: %s. Retrying (%d/%d)", url, err, retries, maxRetries)
		time.Sleep(time.Duration(retryDelayMillis) * time.Millisecond)
		resp, err = http.Post(url, "application/json", bytes.NewBuffer(reqData))
	}

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		logger.Fatal().Uint64("game", gameID).Msgf("Failed to update table status. Error: %d", resp.StatusCode)
	}
	return err
}

func getIp() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", nil
	}
	var ip net.IP
	// handle err
	for _, i := range ifaces {
		addrs, err := i.Addrs()
		if err != nil {
			continue
		}

		// handle err
		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip.IsLoopback() {
				continue
			}
			ipStr := ip.String()
			if strings.Contains(ipStr, ":") {
				continue
			}
			// process IP address
			if ip != nil {
				break
			}
			ip = nil
		}
		if ip != nil {
			break
		}
	}
	if ip == nil {
		return "", fmt.Errorf("Could not get ip address")
	}
	return ip.String(), nil
}

func registerGameServer() error {
	var reqData []byte
	var err error
	ip, err := getIp()
	if err != nil {
		return fmt.Errorf("Could not get ip address of the server")
	}

	hostname, err := getFqdn()
	if err != nil {
		return err
	}
	gameServerURL := fmt.Sprintf("http://%s:8080", hostname)
	payload := map[string]interface{}{"ipAddress": ip, "currentMemory": 10000, "status": "ACTIVE", "url": gameServerURL}
	reqData, err = json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/internal/register-game-server", apiServerUrl)

	retries := 0
	maxRetries := 5
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(reqData))
	for err != nil && retries < maxRetries {
		retries++
		logger.Error().Msgf("Error in post %s: %s. Retrying (%d/%d)", url, err, retries, maxRetries)
		time.Sleep(time.Duration(1000) * time.Millisecond)
		resp, err = http.Post(url, "application/json", bytes.NewBuffer(reqData))
	}

	if err != nil {
		logger.Error().Msgf("Failed to register server. Error: %s", err.Error())
		return errors.Wrap(err, "Error from post request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		logger.Error().Msgf("Failed to register server. Error: %d", resp.StatusCode)
		return fmt.Errorf("Received HTTP status %d", resp.StatusCode)
	}

	return nil
}

func requestRestartGames(apiServerURL string) error {
	var reqData []byte

	hostname, err := getFqdn()
	if err != nil {
		return errors.Wrap(err, "Unable to get game server fqdn")
	}
	gameServerURL := fmt.Sprintf("http://%s:8080", hostname)
	payload := map[string]interface{}{"url": gameServerURL}
	reqData, err = json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "Unable to create payload")
	}

	url := fmt.Sprintf("%s/internal/restart-games", apiServerURL)

	retries := 0
	maxRetries := 5
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(reqData))
	for err != nil && retries < maxRetries {
		retries++
		logger.Error().Msgf("Error in post %s: %s. Retrying (%d/%d)", url, err, retries, maxRetries)
		time.Sleep(time.Duration(1000) * time.Millisecond)
		resp, err = http.Post(url, "application/json", bytes.NewBuffer(reqData))
	}

	if err != nil {
		return errors.Wrap(err, "Error while sending post request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("Received HTTP status %d", resp.StatusCode)
	}

	return nil
}

func getFqdn() (string, error) {
	cmd := exec.Command("/bin/hostname", "-f")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return "", errors.Wrap(err, "Error while getting hostname")
	}
	fqdn := out.String()
	fqdn = fqdn[:len(fqdn)-1] // removing EOL

	return fqdn, nil
}
