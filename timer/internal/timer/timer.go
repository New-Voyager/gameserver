package timer

import (
	"fmt"
	"sync"
	"time"

	"voyager.com/logging"
)

var timerLogger = logging.GetZeroLogger("timer::timer", nil)
var controllerOnce sync.Once
var controller *Controller

// GetController returns the singleton instance of the Controller.
func GetController() *Controller {
	controllerOnce.Do(func() {
		c := NewController()
		c.Start()
		controller = c
	})
	return controller
}

// Controller runs multiple timers and notifies for timeout.
type Controller struct {
	// key: expiration time (unix ts)
	// value: a list of timers that share the same expiration time
	timersByExpiration     map[int64]*Timers
	timersByExpirationLock sync.RWMutex

	// key: gameID|playerID|purpose
	// value: the timer
	timerByKey     map[string]*Timer
	timerByKeyLock sync.RWMutex

	// flag to exit the main goroutine
	end bool
}

// Timers is a list of Timer's.
type Timers struct {
	data []*Timer
	sync.RWMutex
}

// Timer is an instance of a notifiable timer.
type Timer struct {
	gameID      uint64
	playerID    uint64
	purpose     string
	expireTs    int64
	isCancelled bool
	key         string
}

// NewController creates an instance of Controller.
func NewController() *Controller {
	c := Controller{
		timersByExpiration: make(map[int64]*Timers),
		timerByKey:         make(map[string]*Timer),
	}
	return &c
}

// Start starts the main loop goroutine.
func (c *Controller) Start() {
	c.end = false
	go c.runMainLoop()
}

// Stop exits the main loop goroutine.
func (c *Controller) Stop() {
	c.end = true
}

// AddTimer creates a new Timer to track.
func (c *Controller) AddTimer(gameID uint64, playerID uint64, purpose string, expireTs int64) {
	timerKey := c.getTimerKey(gameID, playerID, purpose)
	timer := Timer{
		gameID:   gameID,
		playerID: playerID,
		purpose:  purpose,
		expireTs: expireTs,
		key:      timerKey,
	}

	currentTs := time.Now().Unix()
	if expireTs <= currentTs {
		// This is already expired. Notify immediately without queueing.
		go notifyTimeout(&timer)
		return
	}

	if expireTs == currentTs+1 {
		// Since unix timestamp is one second resolution, this could have less than
		// a second left to expire. Add one second to make sure it doesn't
		// miss the loop.
		expireTs++
		timer.expireTs = expireTs
	}

	c.timersByExpirationLock.Lock()
	timers, exists := c.timersByExpiration[expireTs]
	if !exists {
		timers = &Timers{
			data: make([]*Timer, 0),
		}
		c.timersByExpiration[expireTs] = timers
	}
	c.timersByExpirationLock.Unlock()

	c.timerByKeyLock.Lock()
	c.timerByKey[timerKey] = &timer
	c.timerByKeyLock.Unlock()

	timers.Lock()
	timers.data = append(timers.data, &timer)
	timers.Unlock()
}

// CancelTimer marks a timer as cancelled.
func (c *Controller) CancelTimer(gameID uint64, playerID uint64, purpose string) {
	timerKey := c.getTimerKey(gameID, playerID, purpose)
	c.timerByKeyLock.RLock()
	timer, exists := c.timerByKey[timerKey]
	c.timerByKeyLock.RUnlock()
	if exists {
		timer.isCancelled = true
	} else {
		timerLogger.Error().Msgf("Unable to find timer to cancel - %s", timerKey)
	}
}

func (c *Controller) runMainLoop() {
	lastProcessedTs := time.Now().Unix()

	for {
		if c.end {
			timerLogger.Info().Msg("Exiting timer loop")
			return
		}

		time.Sleep(100 * time.Millisecond)

		currentTs := time.Now().Unix()
		if currentTs <= lastProcessedTs {
			if currentTs < lastProcessedTs {
				// Should not be here.
				timerLogger.Error().Msgf("currentTs < lastProcessedTs (%d < %d)", currentTs, lastProcessedTs)
			}
			continue
		}

		// Go through all the running timers and notify the ones that have not been cancelled.
		timestampsToProcess := c.getTimestampsToProcess(currentTs, lastProcessedTs)
		for _, ts := range timestampsToProcess {
			c.processExpiration(ts)
			lastProcessedTs = ts
		}
	}
}

func (c *Controller) getTimerKey(gameID uint64, playerID uint64, purpose string) string {
	return fmt.Sprintf("%d|%d|%s", gameID, playerID, purpose)
}

// getTimestampsToProcess returns a list of timestamps in range (lastProcessTs, currentTs]
// in the ascending order (oldest first).
func (c *Controller) getTimestampsToProcess(currentTs int64, lastProcessedTs int64) []int64 {
	timestampsToProcess := make([]int64, 0)
	for ts := lastProcessedTs + 1; ts <= currentTs; ts++ {
		timestampsToProcess = append(timestampsToProcess, ts)
	}
	return timestampsToProcess
}

func (c *Controller) processExpiration(ts int64) {
	c.timersByExpirationLock.RLock()
	timers, exists := c.timersByExpiration[ts]
	c.timersByExpirationLock.RUnlock()
	if !exists {
		// No timer scheduled to expire at this time.
		return
	}

	timerLogger.Info().Msgf("Processing timers for %d (%s)", ts, time.Unix(ts, 0).Format(time.RFC3339))

	timers.RLock()
	defer timers.RUnlock()
	for _, t := range timers.data {
		c.timerByKeyLock.RLock()
		currentTimer := c.timerByKey[t.key]
		c.timerByKeyLock.RUnlock()
		if currentTimer != t {
			// A new timer was created with a different expiration time.
			// Ignore this timer.
			continue
		}

		if !t.isCancelled {
			go notifyTimeout(t)
		}
		c.timerByKeyLock.Lock()
		delete(c.timerByKey, t.key)
		c.timerByKeyLock.Unlock()
	}

	c.timersByExpirationLock.Lock()
	delete(c.timersByExpiration, ts)
	c.timersByExpirationLock.Unlock()
}
