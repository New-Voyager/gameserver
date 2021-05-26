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

var envLogger = log.With().Str("logger_name", "util::environment").Logger()

type environment struct {
	RedisHost    string
	RedisPort    string
	RedisPW      string
	RedisDB      string
	PostgresHost string
	PostgresPort string
	PostgresUser string
	PostgresPW   string
	PostgresDB   string
	APIServerURL string
}

// Env is a helper object for accessing environment variables.
var Env = &environment{
	RedisHost:    "REDIS_HOST",
	RedisPort:    "REDIS_PORT",
	RedisPW:      "REDIS_PW",
	RedisDB:      "REDIS_DB",
	PostgresHost: "POSTGRES_HOST",
	PostgresPort: "POSTGRES_PORT",
	PostgresUser: "POSTGRES_USER",
	PostgresPW:   "POSTGRES_PASSWORD",
	PostgresDB:   "POSTGRES_DB",
	APIServerURL: "API_SERVER_URL",
}

func (e *environment) GetRedisHost() string {
	host := os.Getenv(e.RedisHost)
	if host == "" {
		msg := fmt.Sprintf("%s is not defined", e.RedisHost)
		envLogger.Error().Msg(msg)
		panic(msg)
	}
	return host
}

func (e *environment) GetRedisPort() int {
	portStr := os.Getenv(e.RedisPort)
	if portStr == "" {
		msg := fmt.Sprintf("%s is not defined", e.RedisPort)
		envLogger.Error().Msg(msg)
		panic(msg)
	}
	portNum, err := strconv.Atoi(portStr)
	if err != nil {
		msg := fmt.Sprintf("Invalid Redis port %s", portStr)
		envLogger.Error().Msg(msg)
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
		envLogger.Error().Msg(msg)
		panic(msg)
	}
	dbNum, err := strconv.Atoi(dbStr)
	if err != nil {
		msg := fmt.Sprintf("Invalid Redis db %s", dbStr)
		envLogger.Error().Msg(msg)
		panic(msg)
	}
	return dbNum
}

func (e *environment) GetPostgresHost() string {
	v := os.Getenv(e.PostgresHost)
	if v == "" {
		msg := fmt.Sprintf("%s is not defined", e.PostgresHost)
		envLogger.Error().Msg(msg)
		panic(msg)
	}
	return v
}

func (e *environment) GetPostgresPort() int {
	v := os.Getenv(e.PostgresPort)
	if v == "" {
		msg := fmt.Sprintf("%s is not defined", e.PostgresPort)
		envLogger.Error().Msg(msg)
		panic(msg)
	}
	portNum, err := strconv.Atoi(v)
	if err != nil {
		msg := fmt.Sprintf("Invalid Postgres port %s", v)
		envLogger.Error().Msg(msg)
		panic(msg)
	}
	return portNum
}

func (e *environment) GetPostgresUser() string {
	v := os.Getenv(e.PostgresUser)
	if v == "" {
		msg := fmt.Sprintf("%s is not defined", e.PostgresUser)
		envLogger.Error().Msg(msg)
		panic(msg)
	}
	return v
}

func (e *environment) GetPostgresPW() string {
	v := os.Getenv(e.PostgresPW)
	return v
}

func (e *environment) GetPostgresDB() string {
	v := os.Getenv(e.PostgresDB)
	if v == "" {
		msg := fmt.Sprintf("%s is not defined", e.PostgresDB)
		envLogger.Error().Msg(msg)
		panic(msg)
	}
	return v
}

func (e *environment) GetPostgresConnStr() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		e.GetPostgresHost(),
		e.GetPostgresPort(),
		e.GetPostgresUser(),
		e.GetPostgresPW(),
		e.GetPostgresDB(),
	)
}

func (e *environment) GetAPIServerURL() string {
	url := os.Getenv(e.APIServerURL)
	if url == "" {
		msg := fmt.Sprintf("%s is not defined", e.APIServerURL)
		envLogger.Error().Msg(msg)
		panic(msg)
	}
	return url
}

func (e *environment) GetGameServerURL(gameCode string) string {
	// get from the API server
	type payload struct {
		Server struct {
			URL string `json:"url"`
		} `json:"server"`
	}

	url := fmt.Sprintf("%s/internal/get-game-server/game_num/%s", e.GetAPIServerURL(), gameCode)
	response, err := http.Get(url)
	if err != nil {
		panic(fmt.Sprintf("HTTP GET %s returned an error: %s", url, err))
	}
	defer response.Body.Close()
	data, err := ioutil.ReadAll(response.Body)
	if err != nil {
		panic(fmt.Sprintf("Failed to read response body from %s: %s", url, err))
	}
	body := string(data)
	if strings.Contains(body, "errors") {
		panic(fmt.Sprintf("Response from %s contains errors. Response: %s", url, body))
	}
	var p payload
	json.Unmarshal(data, &p)
	if p.Server.URL == "" {
		envLogger.Error().Msgf("Unable to parse game server URL from response: %s", body)
	}
	return p.Server.URL
}
