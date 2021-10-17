package logging

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

const (
	GameIDKey       string = "gameID"
	GameCodeKey     string = "gameCode"
	HandNumKey      string = "handNo"
	SeatNumKey      string = "seatNo"
	PlayerNameKey   string = "playerName"
	PlayerIDKey     string = "playerID"
	MsgTypeKey      string = "msgType"
	TimerPurposeKey string = "purpose"
)

func getEnableColorLog() string {
	v := os.Getenv("COLORIZE_LOG")
	if v == "" {
		// Use colorized logging by default.
		return "true"
	}
	return v
}

func IsColorLoggingEnabled() bool {
	return getEnableColorLog() == "1" || strings.ToLower(getEnableColorLog()) == "true"
}

func GetZeroLogger(name string, out io.Writer) *zerolog.Logger {
	if out == nil {
		out = os.Stdout
	}
	noColor := !IsColorLoggingEnabled()
	output := zerolog.ConsoleWriter{Out: out, NoColor: noColor, TimeFormat: time.RFC3339}
	logger := zerolog.New(output).With().Timestamp().Str("logger", name).Logger()
	return &logger
}
