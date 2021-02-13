package util

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
)

func init() {

}

var environmentLogger = log.With().Str("logger_name", "util::environment").Logger()
var natsUrl string

type environment struct {
	RedisHost     string
	RedisPort     string
	RedisPW       string
	RedisDB       string
	APIServerURL  string
	PrintGameMsg  string
	PrintHandMsg  string
	PrintStateMsg string
}

// Env is a helper object for accessing environment variables.
var Env = &environment{
	RedisHost:     "REDIS_HOST",
	RedisPort:     "REDIS_PORT",
	RedisPW:       "REDIS_PW",
	RedisDB:       "REDIS_DB",
	APIServerURL:  "API_SERVER_URL",
	PrintGameMsg:  "PRINT_GAME_MSG",
	PrintHandMsg:  "PRINT_HAND_MSG",
	PrintStateMsg: "PRINT_STATE_MSG",
}

func (e *environment) GetNatsURL() string {
	if natsUrl == "" {
		// get from the API server
		type Url struct {
			Urls string `json:"urls"`
		}

		url := fmt.Sprintf("%s/nats-urls", e.GetAPIServerURL())
		response, err := http.Get(url)
		if err != nil {
			panic("Failed to get NATS urls")
		}
		defer response.Body.Close()
		data, err := ioutil.ReadAll(response.Body)
		if err != nil {
			panic("Failed to get NATS urls")
		}
		body := string(data)
		if strings.Contains(body, "errors") {
			panic("Failed to get NATS urls")
		}
		var urls Url
		json.Unmarshal(data, &urls)
		natsUrl = urls.Urls
	}
	return natsUrl
}

func (e *environment) GetRedisHost() string {
	host := os.Getenv(e.RedisHost)
	if host == "" {
		msg := fmt.Sprintf("%s is not defined", e.RedisHost)
		environmentLogger.Error().Msg(msg)
		panic(msg)
	}
	return host
}

func (e *environment) GetRedisPort() int {
	portStr := os.Getenv(e.RedisPort)
	if portStr == "" {
		msg := fmt.Sprintf("%s is not defined", e.RedisPort)
		environmentLogger.Error().Msg(msg)
		panic(msg)
	}
	portNum, err := strconv.Atoi(portStr)
	if err != nil {
		msg := fmt.Sprintf("Invalid Redis port %s", portStr)
		environmentLogger.Error().Msg(msg)
		panic(msg)
	}
	return portNum
}

func (e *environment) GetRedisPW() string {
	pw := os.Getenv(e.RedisPW)
	return pw
}

func (e *environment) GetRedisDB() int {
	dbStr := os.Getenv(e.RedisDB)
	if dbStr == "" {
		msg := fmt.Sprintf("%s is not defined", e.RedisDB)
		environmentLogger.Error().Msg(msg)
		panic(msg)
	}
	dbNum, err := strconv.Atoi(dbStr)
	if err != nil {
		msg := fmt.Sprintf("Invalid Redis db %s", dbStr)
		environmentLogger.Error().Msg(msg)
		panic(msg)
	}
	return dbNum
}

func (e *environment) GetAPIServerURL() string {
	host := os.Getenv(e.APIServerURL)
	if host == "" {
		msg := fmt.Sprintf("%s is not defined", e.APIServerURL)
		environmentLogger.Error().Msg(msg)
		panic(msg)
	}
	return host
}

func (e *environment) GetPrintHandMsg() string {
	v := os.Getenv(e.PrintHandMsg)
	if v == "" {
		return "false"
	}
	return v
}

func (e *environment) GetPrintGameMsg() string {
	v := os.Getenv(e.PrintGameMsg)
	if v == "" {
		return "false"
	}
	return v
}

func (e *environment) GetPrintStateMsg() string {
	v := os.Getenv(e.PrintStateMsg)
	if v == "" {
		return "false"
	}
	return v
}

func (e *environment) ShouldPrintGameMsg() bool {
	return e.GetPrintGameMsg() == "1" || strings.ToLower(e.GetPrintGameMsg()) == "true"
}

func (e *environment) ShouldPrintHandMsg() bool {
	return e.GetPrintHandMsg() == "1" || strings.ToLower(e.GetPrintHandMsg()) == "true"
}

func (e *environment) ShouldPrintStateMsg() bool {
	return e.GetPrintStateMsg() == "1" || strings.ToLower(e.GetPrintStateMsg()) == "true"
}
