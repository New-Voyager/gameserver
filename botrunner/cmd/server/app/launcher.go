package app

import (
	"fmt"
	"sync"

	"github.com/pkg/errors"
	"voyager.com/gamescript"
	"voyager.com/logging"
)

var launcherLogger = logging.GetZeroLogger("app::launcher", nil)
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
		batches:     make(map[string]*BotRunnerBatch),
		humanGames:  make(map[string]*HumanGame),
		tournaments: make(map[uint64]*Tournament),
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

	// tournaments
	tournaments map[uint64]*Tournament
}

// ApplyToBatch schedules the requested number of games to be applied to the batch.
func (l *Launcher) ApplyToBatch(batchID string, players *gamescript.Players, script *gamescript.Script, desiredNumGames uint32, launchInterval *float32) error {
	b, exists := l.batches[batchID]
	if exists {
		var launchIntervalMsg string
		if launchInterval != nil {
			launchIntervalMsg = fmt.Sprintf(", LaunchInterval: %v", *launchInterval)
		}
		launcherLogger.Info().Msgf("Updating batch [%s]. NumGames: %d%s", batchID, desiredNumGames, launchIntervalMsg)
		b.Apply(desiredNumGames, launchInterval)
	} else {
		if players == nil || script == nil {
			return fmt.Errorf("There is no existing batch with ID [%s]. Player and script config must be provided to start a new batch", batchID)
		}

		var launchIntervalMsg string
		if launchInterval != nil {
			launchIntervalMsg = fmt.Sprintf(", LaunchInterval: %v", *launchInterval)
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
func (l *Launcher) JoinHumanGame(clubCode string, gameID uint64, gameCode string, players *gamescript.Players, script *gamescript.Script, demoGame bool) error {
	_, exists := l.humanGames[gameCode]
	if exists {
		return fmt.Errorf("There is already an existing BotRunner for game [%s]", gameCode)
	}
	h, err := NewHumanGame(clubCode, gameID, gameCode, players, script, demoGame)
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

// StartAppGame starts a script game for an existing club.
func (l *Launcher) StartAppGame(clubCode string, name string, players *gamescript.Players, script *gamescript.Script) error {
	h, err := NewAppGame(clubCode, name, players, script)
	if err != nil {
		return err
	}
	h.Launch()
	return nil
}

// RegisterTournament starts a BotRunner and register bots to play in tournament
func (l *Launcher) RegisterTournament(clubCode string, tournamentID uint64, botCount int32) error {
	_, exists := l.tournaments[tournamentID]
	if exists {
		return fmt.Errorf("There is already a tournament registered with id [%d]", tournamentID)
	}
	h, err := NewTournament(clubCode, tournamentID, botCount)
	if err != nil {
		return err
	}
	// will register bots and launch bots
	// bots will listen on the tournament channel for tournament messages
	h.Launch(botCount)
	l.tournaments[tournamentID] = h
	return nil
}

// JoinTournament calls the registered bots to join the tournament
func (l *Launcher) JoinTournament(tournamentID uint64) error {
	tournament, exists := l.tournaments[tournamentID]
	if !exists {
		return fmt.Errorf("There is no tournament registered with id [%d]", tournamentID)
	}
	tournament.JoinTournament()
	return nil
}

// EndTournament stops all the registered bots and unregisters the tournament
func (l *Launcher) EndTournament(tournamentID uint64) error {
	tournament, exists := l.tournaments[tournamentID]
	if !exists {
		return fmt.Errorf("There is no tournament registered with id [%d]", tournamentID)
	}
	tournament.EndTournament()
	return nil
}
