package util

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

func GetZeroLogger(name string, out io.Writer) *zerolog.Logger {
	if out == nil {
		out = os.Stdout
	}
	output := zerolog.ConsoleWriter{Out: out, TimeFormat: time.RFC3339}
	logger := zerolog.New(output).With().Timestamp().Str("logger", name).Logger()
	return &logger
}

// func init() {
// 	logrus.SetFormatter(&ConsoleFormatter{
// 		TimestampFormat: time.RFC3339,
// 	})
// }

// type ConsoleFormatter struct {
// 	TimestampFormat string
// }

// func (f *ConsoleFormatter) Format(entry *logrus.Entry) ([]byte, error) {
// 	var sb strings.Builder

// 	// Timestamp
// 	timeStr := "[" + entry.Time.Format(f.TimestampFormat) + "]"
// 	sb.WriteString(timeStr)

// 	// Log level
// 	levelStr := " [" + entry.Level.String() + "]"
// 	sb.WriteString(levelStr)

// 	// Logger name
// 	loggerName, ok := entry.Data["name"].(string)
// 	if !ok {
// 		loggerName = ""
// 	}
// 	loggerNameStr := " [" + loggerName + "]"
// 	sb.WriteString(loggerNameStr)

// 	// Game ID or code
// 	game, ok := entry.Data["game"].(string)
// 	if ok {
// 		gameStr := " [" + game + "]"
// 		sb.WriteString(gameStr)
// 	}

// 	// Message
// 	sb.WriteString(" " + entry.Message + " ")

// 	for k, v := range entry.Data {
// 		if k == "name" || k == "game" {
// 			// Already written
// 			continue
// 		}
// 		stringVal, ok := v.(string)
// 		if !ok {
// 			stringVal = fmt.Sprint(v)
// 		}
// 		sb.WriteString(k + "=" + stringVal + " ")
// 	}
// 	sb.WriteString("\n")
// 	return []byte(sb.String()), nil
// }

// func GetLogrusLogger(name string, out io.Writer) *logrus.Entry {
// 	logger := logrus.WithField("name", name)
// 	// gameLogger := logger.WithFields(logrus.Fields{
// 	// 	"game": "ganolsa",
// 	// })
// 	// gameLogger.Debug("Something happened")
// 	// gameLogger.Info("Something else happened")
// 	// gameLogger.Error("Some other thing happened")
// 	// gameLogger.WithField("seat", 3).Info("Invalid seat acted")

// 	// nonGameLogger := logger
// 	// nonGameLogger.Info("Without game")
// 	return logger
// }
