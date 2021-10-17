package scheduler

import (
	"time"

	"voyager.com/logging"
	"voyager.com/scheduler/internal/util"
)

var (
	postProcessingLogger = logging.GetZeroLogger("scheduler::post_processing", nil)
)

// GetController creates an instance of the Controller.
func GetController() *Controller {
	c := NewController()
	c.Start()
	return c
}

type Controller struct {
	// flag to exit the main goroutine
	end bool

	// channel for queueing the game IDs to be processed
	gameIDChan chan uint64

	// set of game IDs recently processed
	recentGameIDs *Cache
}

// NewController creates an instance of Controller.
func NewController() *Controller {
	cache, err := NewCache(256)
	if err != nil {
		if err != nil || cache == nil {
			panic("Unable to create game ID cache")
		}
	}
	c := Controller{
		// Should be able to queue many games so that we don't hold up incoming requests.
		gameIDChan:    make(chan uint64, 1000),
		recentGameIDs: cache,
	}
	return &c
}

// Start starts the main loop goroutine.
func (c *Controller) Start() {
	c.end = false
	go c.runMainLoop()
	go c.runPeriodic()
}

// Stop exits the main loop goroutine.
func (c *Controller) Stop() {
	c.end = true
}

// SchedulePostProcess queues a game ID for post processing.
func (c *Controller) SchedulePostProcess(gameID uint64) {
	c.gameIDChan <- gameID
}

func (c *Controller) runMainLoop() {
	for {
		if c.end {
			postProcessingLogger.Info().Msg("Exiting post processing loop")
			return
		}

		select {
		case gameID := <-c.gameIDChan:
			c.processGameID(gameID)
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (c *Controller) processGameID(gameID uint64) {
	if !c.recentGameIDs.Exists(gameID) {
		processedGameIDs, _, err := requestPostProcess(gameID)
		if err != nil {
			postProcessingLogger.Error().
				Uint64(logging.GameIDKey, gameID).
				Msgf("Error while processing game: %s", err)
			return
		}
		for _, gameID := range processedGameIDs {
			c.recentGameIDs.Add(gameID)
		}
	}
}

func (c *Controller) runPeriodic() {
	interval := time.Duration(util.Env.GetPostProcessingIntervalSec()) * time.Second
	for range time.NewTicker(interval).C {
		postProcessingLogger.Debug().Msg("Queueing periodic post processing")
		c.gameIDChan <- 0
	}
}
