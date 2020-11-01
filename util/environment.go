package util

import (
	"fmt"
	"os"
	"strconv"

	"github.com/rs/zerolog/log"
)

var environmentLogger = log.With().Str("logger_name", "util::environment").Logger()

type gameServerEnvironment struct {
	RedisHost string
	RedisPort string
	RedisPW   string
	RedisDB   string
}

// GameServerEnvironment is a helper object for accessing environment variables.
var GameServerEnvironment = &gameServerEnvironment{
	RedisHost: "REDIS_HOST",
	RedisPort: "REDIS_PORT",
	RedisPW:   "REDIS_PW",
	RedisDB:   "REDIS_DB",
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
