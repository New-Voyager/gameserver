package nats

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"

	jsoniter "github.com/json-iterator/go"
	natsgo "github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"voyager.com/server/game"
)

var logger = log.With().Str("logger_name", "server::apiserver").Logger()

// Subscribes to messages coming from apiserver and act on the messages
// that is sent to this game server.
var gameServerId = 1
var topic = "apiserver.gameserver"
var apiServerUrl = ""
var natsGameManager *GameManager
var apiServerch chan game.GameMessage // channel to listen for apiserver game messages
var stopApiCh chan bool
var stoppedApiCh chan bool

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
func RegisterGameServer(url string, gameManager *GameManager) *chan game.GameMessage {
	apiServerUrl = url
	natsGameManager = gameManager
	apiServerch = make(chan game.GameMessage, 20)
	stopApiCh = make(chan bool)
	stoppedApiCh = make(chan bool)
	go listenForGameMessage()
	registerGameServer()
	return &apiServerch
}

// RequestRestartGames requests api server to restart the games that were running on this game server
// before crash.
func RequestRestartGames(apiServerURL string) error {
	return requestRestartGames(apiServerURL)
}

func Stop() {
	stopApiCh <- true
	<-stoppedApiCh
}

func SubscribeToNats(nc *natsgo.Conn) {
	log.Info().Msg(fmt.Sprintf("Subscribing to nats topic %s", topic))
	nc.Subscribe(topic, handleApiServerMessages)
}

func listenForGameMessage() {
	stopped := false
	for !stopped {
		select {
		case <-stopApiCh:
			stopped = true
		case message := <-apiServerch:
			handleGameMessage(&message)
		}
	}
	stoppedApiCh <- true
}

func handleGameMessage(message *game.GameMessage) {

}

func handleApiServerMessages(msg *natsgo.Msg) {
	// unmarshal the message
	var data map[string]interface{}
	err := jsoniter.Unmarshal(msg.Data, &data)
	if err != nil {
		return
	}
	var targetGameServer int
	targetGameServer = int(data["gameServer"].(float64))
	if gameServerId != targetGameServer {
		return
	}
	log.Info().Msg(fmt.Sprintf("Message received :- %s", string(msg.Data)))

	messageType := data["type"].(string)
	switch messageType {
	case "NewGame":
		handleNewGame(msg.Data)
	case "GameStatus":
		handleGameStatus(msg.Data)
	case "PlayerUpdate":
		handlePlayerUpdate(msg.Data)
	}
}

func handleNewGame(data []byte) {
	var gameConfig game.GameConfig
	var err error
	err = jsoniter.Unmarshal(data, &gameConfig)
	if err != nil {
		logger.Error().Msg(fmt.Sprintf("New game message cannot be unmarshalled. Error: %s", err.Error()))
		return
	}

	log.Info().Msg(fmt.Sprintf("New game is started. ClubId: %d, gameId: %d", gameConfig.ClubId, gameConfig.GameId))

	// get game configuration

	// initialize nats game
	_, e := natsGameManager.NewGame(gameConfig.ClubId, gameConfig.GameId, &gameConfig)
	if e != nil {
		msg := fmt.Sprintf("Unable to initialize nats game: %v", e)
		logger.Error().Msg(msg)
		panic(msg)
	}

	// update table status
	UpdateTableStatus(gameConfig.GameId, game.TableStatus_WAITING_TO_BE_STARTED)
}

func handleGameStatus(data []byte) {
	var gameStatus GameStatus
	var err error
	err = jsoniter.Unmarshal(data, &gameStatus)
	if err != nil {
		logger.Error().Msg(fmt.Sprintf("Game  message cannot be unmarshalled. Error: %s", err.Error()))
		return
	}

	log.Info().Uint64("gameId", gameStatus.GameId).Msg(fmt.Sprintf("New game status: %d", gameStatus.GameStatus))
	natsGameManager.GameStatusChanged(gameStatus.GameId, game.GameStatus(gameStatus.GameStatus))
}

func handlePlayerUpdate(data []byte) {
	var playerUpdate PlayerUpdate
	var err error
	err = jsoniter.Unmarshal(data, &playerUpdate)
	if err != nil {
		logger.Error().Msg(fmt.Sprintf("Game message cannot be unmarshalled. Error: %s", err.Error()))
		return
	}

	log.Info().Uint64("gameId", playerUpdate.GameId).Msg(fmt.Sprintf("Player: %d seatNo: %d is updated: %v", playerUpdate.PlayerId, playerUpdate.SeatNo, playerUpdate))
	natsGameManager.PlayerUpdate(playerUpdate.GameId, &playerUpdate)
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
