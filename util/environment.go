package util

import (
	"fmt"
	"os"
	"strconv"

	"github.com/rs/zerolog/log"
)

var environmentLogger = log.With().Str("logger_name", "util::environment").Logger()

type gameServerEnvironment struct {
	NatsHost       string
	NatsClientPort string
	PersistMethod  string
	RedisHost      string
	RedisPort      string
	RedisPW        string
	RedisDB        string
	APIServerUrl   string
	PlayTimeout    string
}

// GameServerEnvironment is a helper object for accessing environment variables.
var GameServerEnvironment = &gameServerEnvironment{
	NatsHost:       "NATS_HOST",
	NatsClientPort: "NATS_CLIENT_PORT",
	PersistMethod:  "PERSIST_METHOD",
	RedisHost:      "REDIS_HOST",
	RedisPort:      "REDIS_PORT",
	RedisPW:        "REDIS_PW",
	RedisDB:        "REDIS_DB",
	APIServerUrl:   "API_SERVER_URL",
	PlayTimeout:    "PLAY_TIMEOUT",
}

func (g *gameServerEnvironment) GetNatsHost() string {
	host := os.Getenv(g.NatsHost)
	if host == "" {
		msg := fmt.Sprintf("%s is not defined", g.NatsHost)
		environmentLogger.Error().Msg(msg)
		panic(msg)
	}
	return host
}

func (g *gameServerEnvironment) GetNatsClientPort() int {
	portStr := os.Getenv(g.NatsClientPort)
	if portStr == "" {
		msg := fmt.Sprintf("%s is not defined", g.NatsClientPort)
		environmentLogger.Error().Msg(msg)
		panic(msg)
	}
	portNum, err := strconv.Atoi(portStr)
	if err != nil {
		msg := fmt.Sprintf("Invalid NATS client port %s", portStr)
		environmentLogger.Error().Msg(msg)
		panic(msg)
	}
	return portNum
}

func (g *gameServerEnvironment) GetNatsClientConnURL() string {
	return fmt.Sprintf("nats://%s:%d", g.GetNatsHost(), g.GetNatsClientPort())
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

func (g *gameServerEnvironment) GetApiServerUrl() string {
	host := os.Getenv(g.APIServerUrl)
	if host == "" {
		msg := fmt.Sprintf("%s is not defined", g.APIServerUrl)
		environmentLogger.Error().Msg(msg)
		return ""
	}
	return host
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
