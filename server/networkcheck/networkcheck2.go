package networkcheck

import (
	"runtime/debug"
	"time"

	"github.com/rs/zerolog"
	"voyager.com/logging"
	"voyager.com/server/util"
)

type ClientAliveState struct {
	playerID      uint64
	lastAliveTime time.Time
	connLost      bool
}

type NewAction struct {
	PlayerID uint64
}

type AliveMsg struct {
	PlayerID uint64
	Seq      uint32
}

type NetworkCheck2 struct {
	logger                 *zerolog.Logger
	gameID                 uint64
	gameCode               string
	chEndLoop              chan bool
	chPause                chan bool
	chNewAction            chan NewAction
	chClientAlive          chan *AliveMsg
	clientDeadThresholdSec uint32
	clientState            *ClientAliveState
	paused                 bool
	debugConnectivityCheck bool
	crashHandler           func()
	connLostCallback       func()
	conoRestoredCallback   func()
}

func NewNetworkCheck2(
	logger *zerolog.Logger,
	gameID uint64,
	gameCode string,
	crashHandler func(),
) *NetworkCheck2 {
	n := NetworkCheck2{
		logger:                 logger,
		gameID:                 gameID,
		gameCode:               gameCode,
		chEndLoop:              make(chan bool, 10),
		chPause:                make(chan bool, 10),
		chNewAction:            make(chan NewAction, 10),
		chClientAlive:          make(chan *AliveMsg, 10),
		clientDeadThresholdSec: uint32(util.Env.GetPingTimeout()),
		clientState:            nil,
		debugConnectivityCheck: util.Env.ShouldDebugConnectivityCheck(),
		crashHandler:           crashHandler,
	}
	return &n
}

func (n *NetworkCheck2) Run() {
	go n.loop()
}
func (n *NetworkCheck2) Destroy() {
	n.chEndLoop <- true
}

func (n *NetworkCheck2) loop() {
	defer func() {
		err := recover()
		if err != nil {
			// Panic occurred.
			debug.PrintStack()
			n.logger.Error().
				Msgf("network check loop returning due to panic: %s\nStack Trace:\n%s", err, string(debug.Stack()))

			n.crashHandler()
		} else {
			n.logger.Info().Msg("Network check loop returning")
		}
	}()

	for {
		select {
		case action := <-n.chNewAction:
			n.handleNewAction(action)
		case msg := <-n.chClientAlive:
			n.handleClientAlive(msg)
		case <-n.chPause:
			n.handlePause()
		case <-n.chEndLoop:
			return
		default:
			n.handleTimeout()
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (n *NetworkCheck2) NewAction(a NewAction) {
	n.chNewAction <- a
}

func (n *NetworkCheck2) handlePause() {
	n.paused = true
}

func (n *NetworkCheck2) handleTimeout() {
	if n.paused {
		return
	}

	now := time.Now()
	deadThreshold := time.Duration(n.clientDeadThresholdSec) * time.Second
	timeoutAt := n.clientState.lastAliveTime.Add(deadThreshold)

	if n.clientState.connLost {
		if now.Before(timeoutAt) {
			// Connection restored.
			n.logger.Info().
				Uint64(logging.PlayerIDKey, n.clientState.playerID).
				Msg("Player connectivity restored")

			// TODO: Notify main loop.

			n.clientState.connLost = false
		}
	} else {
		if now.After(timeoutAt) {
			// Connection lost.
			n.logger.Info().
				Uint64(logging.PlayerIDKey, n.clientState.playerID).
				Msg("Player connectivity lost")

			// TODO: Notify main loop.

			n.clientState.connLost = true
		}
	}
}

func (n *NetworkCheck2) handleNewAction(action NewAction) {
	now := time.Now()
	n.clientState = &ClientAliveState{
		playerID:      action.PlayerID,
		lastAliveTime: now,
		connLost:      false,
	}

	n.paused = false
}

// Handle the alive msg from the client.
func (n *NetworkCheck2) handleClientAlive(msg *AliveMsg) {
	if msg.PlayerID != n.clientState.playerID {
		n.logger.Info().Msgf("Ignoring alive msg from unexpected player. Current action player: %d, msg Player: %d", n.clientState.playerID, msg.PlayerID)
		return
	}

	n.clientState.lastAliveTime = time.Now()
}
