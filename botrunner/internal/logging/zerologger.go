package logging

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

const (
	GameIDKey   string = "gameID"
	GameCodeKey string = "gameCode"
)

func GetZeroLogger(name string, out io.Writer) *zerolog.Logger {
	if out == nil {
		out = os.Stdout
	}
	output := zerolog.ConsoleWriter{Out: out, TimeFormat: time.RFC3339}
	logger := zerolog.New(output).With().Timestamp().Str("logger", name).Logger()
	return &logger
}
