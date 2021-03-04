package app

import (
	"fmt"
	"sync"

	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"voyager.com/gamescript"
)

var launcherLogger = log.With().Str("logger_name", "app::launcher").Logger()
var launcherOnce sync.Once
var launcher *Launcher

// GetLauncher returns the single instance of the Launcher.
func GetLauncher() *Launcher {
	launcherOnce.Do(func() {
		l := NewLauncher()
		launcher = l
	})
	return launcher
}

// NewLauncher creates an instance of Launcher.
func NewLauncher() *Launcher {
	return &Launcher{
		batches:    make(map[string]*BotRunnerBatch),
		humanGames: make(map[string]*HumanGame),
	}
}

// Launcher manages starting and stopping multiple botrunners.
type Launcher struct {
	// Key: batch ID
	// Value: A group of BotRunner's that share the same batch ID and the game script.
	batches map[string]*BotRunnerBatch

	// Key: game code
	// Value: Single bot runner to join the human game.
	humanGames map[string]*HumanGame
}

// ApplyToBatch schedules the requested number of games to be applied to the batch.
func (l *Launcher) ApplyToBatch(batchID string, players *gamescript.Players, script *gamescript.Script, desiredNumGames uint32, launchInterval *float32) error {
	b, exists := l.batches[batchID]
	if exists {
		var launchIntervalMsg string
		if launchInterval != nil {
			launchIntervalMsg = fmt.Sprintf(", LaunchInterval: %f", *launchInterval)
		}
		launcherLogger.Info().Msgf("Updating batch [%s]. NumGames: %d%s", batchID, desiredNumGames, launchIntervalMsg)
		b.Apply(desiredNumGames, launchInterval)
	} else {
		if players == nil || script == nil {
			return fmt.Errorf("There is no existing batch with ID [%s]. Player and script config must be provided to start a new batch", batchID)
		}

		var launchIntervalMsg string
		if launchInterval != nil {
			launchIntervalMsg = fmt.Sprintf(", LaunchInterval: %f", *launchInterval)
		}
		launcherLogger.Info().Msgf("Creating batch [%s]. NumGames: %d%s, Players: %+v, Script: %+v", batchID, desiredNumGames, launchIntervalMsg, players, script)
		b, err := NewBotRunnerBatch(batchID, players, script)
		if err != nil {
			return errors.Wrap(err, "Unable to create a new BotRunnerBatch")
		}
		err = b.Apply(desiredNumGames, launchInterval)
		if err != nil {
			return errors.Wrap(err, "Unable to apply the desired number of games and launch interval")
		}
		l.batches[batchID] = b
	}

	return nil
}

// StopBatch schedules to stop the specified batch of botrunners.
func (l *Launcher) StopBatch(batchID string) error {
	launcherLogger.Info().Msgf("Stopping botrunners in batch [%s]", batchID)
	b, exists := l.batches[batchID]
	if !exists {
		return fmt.Errorf("Batch [%s] does not exist", batchID)
	}
	err := b.Apply(0, nil)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Unable to schedule the number of botrunners to 0 for batch [%s]", batchID))
	}
	return nil
}

// StopAll schedules to stop all running botrunner batches.
func (l *Launcher) StopAll() error {
	launcherLogger.Info().Msg("Stopping all botrunner batches")
	for batchID := range l.batches {
		l.StopBatch(batchID)
	}
	return nil
}

// BatchExists checks if there is a batch of botrunners with the specified ID.
func (l *Launcher) BatchExists(batchID string) bool {
	_, exists := l.batches[batchID]
	return exists
}

// JoinHumanGame starts a BotRunner that joins a human-created game.
func (l *Launcher) JoinHumanGame(clubCode string, gameCode string, players *gamescript.Players, script *gamescript.Script) error {
	_, exists := l.humanGames[gameCode]
	if exists {
		return fmt.Errorf("There is already an existing BotRunner for game [%s]", gameCode)
	}
	h, err := NewHumanGame(clubCode, gameCode, players, script)
	if err != nil {
		return err
	}
	h.Launch()
	l.humanGames[gameCode] = h
	return nil
}

// DeleteHumanGame deletes the BotRunner that is running with a human game.
func (l *Launcher) DeleteHumanGame(gameCode string) error {
	_, exists := l.humanGames[gameCode]
	if !exists {
		return fmt.Errorf("There is no existing BotRunner for game [%s]", gameCode)
	}
	l.humanGames[gameCode] = nil
	delete(l.humanGames, gameCode)
	return nil
}
