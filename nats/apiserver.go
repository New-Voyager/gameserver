package nats

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"

	"github.com/rs/zerolog/log"
	"voyager.com/server/game"
)

var logger = log.With().Str("logger_name", "server::apiserver").Logger()

// Subscribes to messages coming from apiserver and act on the messages
// that is sent to this game server.
var apiServerUrl = ""

type GameStatus struct {
	GameId     uint64 `json:"gameId"`
	GameStatus uint32 `json:"gameStatus"`
}

type PlayerUpdate struct {
	GameId    uint64            `json:"gameId"`
	PlayerId  uint64            `json:"playerId"`
	SeatNo    uint64            `json:"seatNo"`
	Stack     float64           `json:"Stack"`
	Status    game.PlayerStatus `json:"status"`
	BuyIn     float64           `json:"buyIn"`
	GameToken string            `json:"gameToken"`
	NewUpdate uint32            `json:"newUpdate"`
}

type TableUpdate struct {
	GameId                  uint64   `json:"gameId"`
	SeatNo                  uint64   `json:"seatNo"`
	Type                    string   `json:"type"`
	SeatChangePlayers       []uint64 `json:"seatChangePlayers"`
	SeatChangeSeatNos       []uint64 `json:"seatChangeSeatNos"`
	SeatChangeRemainingTime uint32   `json:"seatChangeRemainingTime"`
	WaitlistRemainingTime   uint32   `json:"waitlistRemainingTime"`
	WaitlistPlayerId        uint64   `json:"waitlistPlayerId"`
	WaitlistPlayerUuid      string   `json:"waitlisttPlayerUuid"`
	WaitlistPlayerName      string   `json:"waitlistPlayerName"`
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

func UpdateTableStatus(gameID uint64, status game.TableStatus) error {
	// update table status
	var reqData []byte
	var err error
	payload := map[string]interface{}{"gameId": gameID, "status": status}
	reqData, err = json.Marshal(payload)
	if err != nil {
		return err
	}

	statusUrl := fmt.Sprintf("%s/internal/update-table-status", apiServerUrl)
	resp, err := http.Post(statusUrl, "application/json", bytes.NewBuffer(reqData))
	if resp.StatusCode != 200 {
		logger.Fatal().Uint64("game", gameID).Msg(fmt.Sprintf("Failed to update table status. Error: %d", resp.StatusCode))
	}
	defer resp.Body.Close()
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
		panic(fmt.Sprintf("Could not get ip address of the server"))
	}

	hostname, err := getFqdn()
	if err != nil {
		return err
	}
	url := fmt.Sprintf("http://%s:8080", hostname)
	payload := map[string]interface{}{"ipAddress": ip, "currentMemory": 10000, "status": "ACTIVE", "url": url}
	reqData, err = json.Marshal(payload)
	if err != nil {
		return err
	}

	statusUrl := fmt.Sprintf("%s/internal/register-game-server", apiServerUrl)
	resp, err := http.Post(statusUrl, "application/json", bytes.NewBuffer(reqData))
	if err != nil {
		logger.Fatal().Msg(fmt.Sprintf("Failed to register server. Error: %s", err.Error()))
		panic("Failed when registering game server")
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		logger.Fatal().Msg(fmt.Sprintf("Failed to register server. Error: %d", resp.StatusCode))
		panic("Failed when registering game server")
	}
	return err
}

func requestRestartGames(apiServerURL string) error {
	var reqData []byte
	var err error

	hostname, err := getFqdn()
	if err != nil {
		return err
	}
	url := fmt.Sprintf("http://%s:8080", hostname)
	payload := map[string]interface{}{"url": url}
	reqData, err = json.Marshal(payload)
	if err != nil {
		return err
	}

	restartURL := fmt.Sprintf("%s/internal/restart-games", apiServerURL)
	resp, err := http.Post(restartURL, "application/json", bytes.NewBuffer(reqData))
	if err != nil {
		logger.Fatal().Msg(fmt.Sprintf("Failed to restart games. Error: %s", err.Error()))
		panic("Failed when restarting games")
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		logger.Fatal().Msg(fmt.Sprintf("Failed to restart games. Error: %d", resp.StatusCode))
		panic("Failed when restarting games")
	}
	return err
}

func getFqdn() (string, error) {
	cmd := exec.Command("/bin/hostname", "-f")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("Error when getting hostname: %v", err)
	}
	fqdn := out.String()
	fqdn = fqdn[:len(fqdn)-1] // removing EOL

	return fqdn, nil
}
