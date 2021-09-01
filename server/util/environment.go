package util

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var environmentLogger = log.With().Str("logger_name", "util::environment").Logger()
var natsUrl string

type gameServerEnvironment struct {
	PersistMethod          string
	RedisHost              string
	RedisPort              string
	RedisUser              string
	RedisPW                string
	RedisDB                string
	APIServerUrl           string
	PlayTimeout            string
	PingTimeout            string
	DisableDelays          string
	PostgresHost           string
	PostgresPort           string
	PostgresDB             string
	PostgresUser           string
	PostgresPW             string
	PostgresSSLMode        string
	EnableEncryption       string
	DebugConnectivityCheck string
	SystemTest             string
	LogLevel               string
}

// Env is a helper object for accessing environment variables.
var Env = &gameServerEnvironment{
	PersistMethod:          "PERSIST_METHOD",
	RedisHost:              "REDIS_HOST",
	RedisPort:              "REDIS_PORT",
	RedisUser:              "REDIS_USER",
	RedisPW:                "REDIS_PASSWORD",
	RedisDB:                "REDIS_DB",
	APIServerUrl:           "API_SERVER_URL",
	PlayTimeout:            "PLAY_TIMEOUT",
	PingTimeout:            "PING_TIMEOUT",
	DisableDelays:          "DISABLE_DELAYS",
	PostgresHost:           "POSTGRES_HOST",
	PostgresPort:           "POSTGRES_PORT",
	PostgresDB:             "POSTGRES_DB",
	PostgresUser:           "POSTGRES_USER",
	PostgresPW:             "POSTGRES_PASSWORD",
	PostgresSSLMode:        "POSTGRES_SSL_MODE",
	EnableEncryption:       "ENABLE_ENCRYPTION",
	DebugConnectivityCheck: "DEBUG_CONNECTIVITY_CHECK",
	SystemTest:             "SYSTEM_TEST",
	LogLevel:               "LOG_LEVEL",
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

func (e *gameServerEnvironment) GetRedisUser() string {
	v := os.Getenv(e.RedisUser)
	return v
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

func (g *gameServerEnvironment) GetPostgresSSLMode() string {
	v := os.Getenv(g.PostgresSSLMode)
	if v == "" {
		defaultVal := "disable"
		msg := fmt.Sprintf("%s is not defined. Using default '%s'", g.PostgresSSLMode, defaultVal)
		environmentLogger.Warn().Msg(msg)
		return defaultVal
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

func (g *gameServerEnvironment) GetEnableEncryption() string {
	v := os.Getenv(g.EnableEncryption)
	if v == "" {
		return "false"
	}
	return v
}

func (g *gameServerEnvironment) IsEncryptionEnabled() bool {
	return g.GetEnableEncryption() == "1" || strings.ToLower(g.GetEnableEncryption()) == "true"
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

func (g *gameServerEnvironment) GetSystemTest() string {
	v := os.Getenv(g.SystemTest)
	if v == "" {
		return "false"
	}
	return v
}

func (g *gameServerEnvironment) IsSystemTest() bool {
	return g.GetSystemTest() == "1" || strings.ToLower(g.GetSystemTest()) == "true"
}

func (g *gameServerEnvironment) GetLogLevel() string {
	v := os.Getenv(g.LogLevel)
	if v == "" {
		defaultVal := "info"
		environmentLogger.Warn().Msgf("%s is not defined. Using default %s", g.LogLevel, defaultVal)
		return defaultVal
	}
	return v
}

func (g *gameServerEnvironment) GetZeroLogLogLevel() zerolog.Level {
	l := g.GetLogLevel()
	switch strings.ToLower(l) {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn":
		fallthrough
	case "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "disabled":
		return zerolog.Disabled
	default:
		panic(fmt.Sprintf("Unsupported %s: %s", g.LogLevel, l))
	}
}
