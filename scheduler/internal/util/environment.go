package util

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func init() {

}

var envLogger = log.With().Str("logger_name", "util::environment").Logger()

type environment struct {
	APIServerInternalURL      string
	PostProcessingTimeoutSec  string
	PostProcessingIntervalSec string
	ExpireGamesIntervalSec    string
	ExpireGamesTimeoutSec     string
	DataRetentionIntervalMin  string
	DataRetentionTimeoutMin   string
	LogLevel                  string
}

// Env is a helper object for accessing environment variables.
var Env = &environment{
	APIServerInternalURL:      "API_SERVER_INTERNAL_URL",
	LogLevel:                  "LOG_LEVEL",
	PostProcessingTimeoutSec:  "POST_PROCESSING_TIMEOUT_SEC",
	PostProcessingIntervalSec: "POST_PROCESSING_INTERVAL_SEC",
	ExpireGamesIntervalSec:    "EXPIRE_GAMES_INTERVAL_SEC",
	ExpireGamesTimeoutSec:     "EXPIRE_GAMES_TIMEOUT_SEC",
	DataRetentionIntervalMin:  "DATA_RETENTION_INTERVAL_MIN",
	DataRetentionTimeoutMin:   "DATA_RETENTION_TIMEOUT_MIN",
}

func (e *environment) GetAPIServerInternalURL() string {
	url := os.Getenv(e.APIServerInternalURL)
	if url == "" {
		msg := fmt.Sprintf("%s is not defined", e.APIServerInternalURL)
		envLogger.Error().Msg(msg)
		panic(msg)
	}
	return url
}

func (e *environment) GetPostProcessingTimeoutSec() int {
	v := os.Getenv(e.PostProcessingTimeoutSec)
	if v == "" {
		msg := fmt.Sprintf("%s is not defined", e.PostProcessingTimeoutSec)
		envLogger.Error().Msg(msg)
		panic(msg)
	}
	timeoutSec, err := strconv.Atoi(v)
	if err != nil {
		msg := fmt.Sprintf("Invalid value for %s: %s", e.PostProcessingTimeoutSec, v)
		envLogger.Error().Msg(msg)
		panic(msg)
	}
	return timeoutSec
}

func (e *environment) GetPostProcessingIntervalSec() int {
	v := os.Getenv(e.PostProcessingIntervalSec)
	if v == "" {
		msg := fmt.Sprintf("%s is not defined", e.PostProcessingIntervalSec)
		envLogger.Error().Msg(msg)
		panic(msg)
	}
	sec, err := strconv.Atoi(v)
	if err != nil {
		msg := fmt.Sprintf("Invalid value for %s: %s", e.PostProcessingIntervalSec, v)
		envLogger.Error().Msg(msg)
		panic(msg)
	}
	return sec
}

func (e *environment) GetExpireGamesTimeoutSec() int {
	v := os.Getenv(e.ExpireGamesTimeoutSec)
	if v == "" {
		msg := fmt.Sprintf("%s is not defined", e.ExpireGamesTimeoutSec)
		envLogger.Error().Msg(msg)
		panic(msg)
	}
	sec, err := strconv.Atoi(v)
	if err != nil {
		msg := fmt.Sprintf("Invalid value for %s: %s", e.ExpireGamesTimeoutSec, v)
		envLogger.Error().Msg(msg)
		panic(msg)
	}
	return sec
}

func (e *environment) GetExpireGamesIntervalSec() int {
	v := os.Getenv(e.ExpireGamesIntervalSec)
	if v == "" {
		msg := fmt.Sprintf("%s is not defined", e.ExpireGamesIntervalSec)
		envLogger.Error().Msg(msg)
		panic(msg)
	}
	sec, err := strconv.Atoi(v)
	if err != nil {
		msg := fmt.Sprintf("Invalid value for %s: %s", e.ExpireGamesIntervalSec, v)
		envLogger.Error().Msg(msg)
		panic(msg)
	}
	return sec
}

func (e *environment) GetDataRetentionTimeoutMin() int {
	v := os.Getenv(e.DataRetentionTimeoutMin)
	if v == "" {
		msg := fmt.Sprintf("%s is not defined", e.DataRetentionTimeoutMin)
		envLogger.Error().Msg(msg)
		panic(msg)
	}
	sec, err := strconv.Atoi(v)
	if err != nil {
		msg := fmt.Sprintf("Invalid value for %s: %s", e.DataRetentionTimeoutMin, v)
		envLogger.Error().Msg(msg)
		panic(msg)
	}
	return sec
}

func (e *environment) GetDataRetentionIntervalMin() int {
	v := os.Getenv(e.DataRetentionIntervalMin)
	if v == "" {
		msg := fmt.Sprintf("%s is not defined", e.DataRetentionIntervalMin)
		envLogger.Error().Msg(msg)
		panic(msg)
	}
	sec, err := strconv.Atoi(v)
	if err != nil {
		msg := fmt.Sprintf("Invalid value for %s: %s", e.DataRetentionIntervalMin, v)
		envLogger.Error().Msg(msg)
		panic(msg)
	}
	return sec
}

func (e *environment) GetLogLevel() string {
	v := os.Getenv(e.LogLevel)
	if v == "" {
		defaultVal := "info"
		envLogger.Warn().Msgf("%s is not defined. Using default %s", e.LogLevel, defaultVal)
		return defaultVal
	}
	return v
}

func (e *environment) GetZeroLogLogLevel() zerolog.Level {
	l := e.GetLogLevel()
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
		panic(fmt.Sprintf("Unsupported %s: %s", e.LogLevel, l))
	}
}
