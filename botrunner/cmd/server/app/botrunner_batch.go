package app

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"voyager.com/botrunner/internal/driver"
	"voyager.com/botrunner/internal/util"
	"voyager.com/gamescript"
)

// BotRunnerBatch is a group of BotRunner's that are given the same batch ID.
// All botrunners in a batch use the same botrunner script.
type BotRunnerBatch struct {
	logger           zerolog.Logger
	botRunnerLogDir  string
	batchID          string
	instances        []*driver.BotRunner
	players          *gamescript.Players
	script           *gamescript.Script
	launchInterval   float32
	desiredInstances uint32
	terminate        bool
}

// NewBotRunnerBatch creates a new instance of BotRunnerBatch.
func NewBotRunnerBatch(batchID string, players *gamescript.Players, script *gamescript.Script) (*BotRunnerBatch, error) {
	b := BotRunnerBatch{
		logger:          log.With().Str("logger_name", "BotRunnerBatch").Logger(),
		batchID:         batchID,
		botRunnerLogDir: filepath.Join(baseLogDir, batchID),
		players:         players,
		script:          script,
	}
	go b.mainLoop()
	return &b, nil
}

// Destroy stops all bot runners and cleans up any running goroutine.
func (b *BotRunnerBatch) Destroy() {
	b.terminate = true
}

// Apply changes the desired number of bot runner instances, and optionally the launch interval.
func (b *BotRunnerBatch) Apply(desiredInstances uint32, launchInterval *float32) error {
	if launchInterval != nil && *launchInterval >= 0 {
		b.launchInterval = *launchInterval
	}

	b.desiredInstances = desiredInstances
	return nil
}

// Continue checking and try to reach the desired instances.
func (b *BotRunnerBatch) mainLoop() {
	var lastLaunchTime time.Time

	for {
		numInstances := uint32(len(b.instances))
		numDesiredInstances := b.desiredInstances

		if numDesiredInstances == numInstances {
			// We have desired number of botrunners running. Don't do anything.
			time.Sleep(100 * time.Millisecond)
			continue
		}

		if numDesiredInstances < numInstances {
			// We have more botrunners than desired. Remove some of them.
			b.logger.Info().Msgf("Reducing the number of botrunners (%d => %d) for batch [%s].", numInstances, numDesiredInstances, b.batchID)
			for numDesiredInstances < uint32(len(b.instances)) {
				b.instances[len(b.instances)-1].Terminate()
				b.instances[len(b.instances)-1] = nil
				b.instances = b.instances[:len(b.instances)-1]
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}

		nextLaunchTime := lastLaunchTime.Add(util.FloatSecToDuration(b.launchInterval))
		now := time.Now()
		if now.Before(nextLaunchTime) {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		nextInstanceNo := numInstances + 1
		loggerName := fmt.Sprintf("BotRunner%d", nextInstanceNo)
		err := os.MkdirAll(b.botRunnerLogDir, os.ModePerm)
		if err != nil {
			b.logger.Error().Msgf("Unable to create log directory %s: %s", b.botRunnerLogDir, err)
			time.Sleep(2 * time.Second)
			continue
		}
		logFileName := b.getLogFileName(b.botRunnerLogDir, nextInstanceNo)
		f, err := os.Create(logFileName)
		if err != nil {
			b.logger.Error().Msgf("Unable to create log file %s: %s", logFileName, err)
			time.Sleep(2 * time.Second)
			continue
		}
		botRunnerLogger := zerolog.New(f).With().Timestamp().Str("logger_name", loggerName).Logger()
		botPlayerLogger := zerolog.New(f).With().Timestamp().Str("logger_name", "BotPlayer").Logger()

		b.logger.Info().Msgf("Launching bot runner instance [%d]. Logging to %s.", nextInstanceNo, logFileName)
		botRunner, err := driver.NewBotRunner("", "", b.script, b.players, &botRunnerLogger, &botPlayerLogger, false, false)
		if err != nil {
			b.logger.Error().Msgf("Error while creating a BotRunner: %s", err)
			time.Sleep(2 * time.Second)
			continue
		}
		go func() {
			err := botRunner.Run()
			if err != nil {
				errMsg := fmt.Sprintf("Error from botrunner: %s", err)
				botRunnerLogger.Error().Msg(errMsg)
				fmt.Println(errMsg)
			}
		}()
		b.instances = append(b.instances, botRunner)
		lastLaunchTime = now
	}
}

func (b *BotRunnerBatch) getLogFileName(baseDir string, instanceNo uint32) string {
	return filepath.Join(baseDir, fmt.Sprintf("botrunner_%d.log", instanceNo))
}
