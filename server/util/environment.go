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

var environmentLogger = log.With().Str("logger_name", "util::environment").Logger()
var natsUrl string

type gameServerEnvironment struct {
	PersistMethod             string
	RedisHost                 string
	RedisPort                 string
	RedisPW                   string
	RedisDB                   string
	APIServerUrl              string
	PlayTimeout               string
	PingTimeout               string
	DisableDelays             string
	PostgresHost              string
	PostgresPort              string
	PostgresDB                string
	PostgresUser              string
	PostgresPW                string
	EnablePlayerMsgEncryption string
	DebugConnectivityCheck    string
}

// GameServerEnvironment is a helper object for accessing environment variables.
var GameServerEnvironment = &gameServerEnvironment{
	PersistMethod:             "PERSIST_METHOD",
	RedisHost:                 "REDIS_HOST",
	RedisPort:                 "REDIS_PORT",
	RedisPW:                   "REDIS_PW",
	RedisDB:                   "REDIS_DB",
	APIServerUrl:              "API_SERVER_URL",
	PlayTimeout:               "PLAY_TIMEOUT",
	PingTimeout:               "PING_TIMEOUT",
	DisableDelays:             "DISABLE_DELAYS",
	PostgresHost:              "POSTGRES_HOST",
	PostgresPort:              "POSTGRES_PORT",
	PostgresDB:                "POSTGRES_DB",
	PostgresUser:              "POSTGRES_USER",
	PostgresPW:                "POSTGRES_PASSWORD",
	EnablePlayerMsgEncryption: "ENABLE_PLAYER_MSG_ENCRYPTION",
	DebugConnectivityCheck:    "DEBUG_CONNECTIVITY_CHECK",
}

func (g *gameServerEnvironment) GetNatsURL() string {
	if natsUrl == "" {
		// get from the API server
		type Url struct {
			Urls string `json:"urls"`
		}

		url := fmt.Sprintf("%s/nats-urls", g.GetApiServerUrl())
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

func (g *gameServerEnvironment) GetPersistMethod() string {
	method := os.Getenv(g.PersistMethod)
	if method == "" {
		msg := fmt.Sprintf("%s is not defined", g.PersistMethod)
		environmentLogger.Error().Msg(msg)
		panic(msg)
	}
	return method
}

func (g *gameServerEnvironment) GetRedisHost() string {
	host := os.Getenv(g.RedisHost)
	if host == "" {
		msg := fmt.Sprintf("%s is not defined", g.RedisHost)
		environmentLogger.Error().Msg(msg)
		panic(msg)
	}
	return host
}

func (g *gameServerEnvironment) GetRedisPort() int {
	portStr := os.Getenv(g.RedisPort)
	if portStr == "" {
		msg := fmt.Sprintf("%s is not defined", g.RedisPort)
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

func (g *gameServerEnvironment) GetRedisPW() string {
	pw := os.Getenv(g.RedisPW)
	return pw
}

func (g *gameServerEnvironment) GetRedisDB() int {
	dbStr := os.Getenv(g.RedisDB)
	if dbStr == "" {
		msg := fmt.Sprintf("%s is not defined", g.RedisDB)
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

func (g *gameServerEnvironment) GetPostgresHost() string {
	host := os.Getenv(g.PostgresHost)
	if host == "" {
		msg := fmt.Sprintf("%s is not defined", g.PostgresHost)
		environmentLogger.Error().Msg(msg)
		panic(msg)
	}
	return host
}

func (g *gameServerEnvironment) GetPostgresPort() int {
	portStr := os.Getenv(g.PostgresPort)
	if portStr == "" {
		msg := fmt.Sprintf("%s is not defined", g.PostgresPort)
		environmentLogger.Error().Msg(msg)
		panic(msg)
	}
	portNum, err := strconv.Atoi(portStr)
	if err != nil {
		msg := fmt.Sprintf("Invalid Postgres port %s", portStr)
		environmentLogger.Error().Msg(msg)
		panic(msg)
	}
	return portNum
}

func (g *gameServerEnvironment) GetPostgresUser() string {
	v := os.Getenv(g.PostgresUser)
	if v == "" {
		msg := fmt.Sprintf("%s is not defined", g.PostgresUser)
		environmentLogger.Error().Msg(msg)
		panic(msg)
	}
	return v
}

func (g *gameServerEnvironment) GetPostgresPW() string {
	v := os.Getenv(g.PostgresPW)
	if v == "" {
		msg := fmt.Sprintf("%s is not defined", g.PostgresPW)
		environmentLogger.Error().Msg(msg)
		panic(msg)
	}
	return v
}

func (g *gameServerEnvironment) GetPostgresDB() string {
	v := os.Getenv(g.PostgresDB)
	if v == "" {
		msg := fmt.Sprintf("%s is not defined", g.PostgresDB)
		environmentLogger.Error().Msg(msg)
		panic(msg)
	}
	return v
}

func (g *gameServerEnvironment) GetApiServerUrl() string {
	url := os.Getenv(g.APIServerUrl)
	if url == "" {
		msg := fmt.Sprintf("%s is not defined", g.APIServerUrl)
		environmentLogger.Error().Msg(msg)
		panic(msg)
	}
	return url
}

func (g *gameServerEnvironment) GetPlayTimeout() int {
	s := os.Getenv(g.PlayTimeout)
	if s == "" {
		// 1 minute + a few seconds for slow network
		return 62
	}
	timeoutSec, err := strconv.Atoi(s)
	if err != nil {
		msg := fmt.Sprintf("Invalid integer [%s] for play timeout value", s)
		environmentLogger.Error().Msg(msg)
		panic(msg)
	}
	return timeoutSec
}

func (g *gameServerEnvironment) GetPingTimeout() int {
	s := os.Getenv(g.PingTimeout)
	if s == "" {
		return 3
	}
	timeoutSec, err := strconv.Atoi(s)
	if err != nil {
		msg := fmt.Sprintf("Invalid integer [%s] for ping timeout value", s)
		environmentLogger.Error().Msg(msg)
		panic(msg)
	}
	return timeoutSec
}

func (g *gameServerEnvironment) GetDisableDelays() string {
	v := os.Getenv(g.DisableDelays)
	if v == "" {
		return "false"
	}
	return v
}

func (g *gameServerEnvironment) ShouldDisableDelays() bool {
	return g.GetDisableDelays() == "1" || strings.ToLower(g.GetDisableDelays()) == "true"
}

func (g *gameServerEnvironment) GetEnablePlayerMsgEncryption() string {
	v := os.Getenv(g.EnablePlayerMsgEncryption)
	if v == "" {
		return "false"
	}
	return v
}

func (g *gameServerEnvironment) ShouldEncryptPlayerMsg() bool {
	return g.GetEnablePlayerMsgEncryption() == "1" || strings.ToLower(g.GetEnablePlayerMsgEncryption()) == "true"
}

func (g *gameServerEnvironment) GetDebugConnectivityCheck() string {
	v := os.Getenv(g.DebugConnectivityCheck)
	if v == "" {
		return "false"
	}
	return v
}

func (g *gameServerEnvironment) ShouldDebugConnectivityCheck() bool {
	return g.GetDebugConnectivityCheck() == "1" || strings.ToLower(g.GetDebugConnectivityCheck()) == "true"
}
