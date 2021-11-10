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

type Action struct {
	PlayerID uint64
}

type AliveMsg struct {
	PlayerID uint64
}

type NetworkCheck struct {
	logger                 *zerolog.Logger
	gameID                 uint64
	gameCode               string
	chEndLoop              chan bool
	chPause                chan bool
	chNewAction            chan Action
	chClientAlive          chan *AliveMsg
	clientDeadThresholdSec uint32
	clientState            *ClientAliveState
	currentAction          Action
	paused                 bool
	debugConnectivityCheck bool
	crashHandler           func()
	connLost               func(Action)
	connRestored           func(Action)
}

func NewNetworkCheck(
	logger *zerolog.Logger,
	gameID uint64,
	gameCode string,
	crashHandler func(),
	connLostCallback func(Action),
	connRestoredCallback func(Action),
) *NetworkCheck {
	if connLostCallback == nil {
		panic("connLostCallback is nil in NewNetworkCheck")
	}
	if connRestoredCallback == nil {
		panic("connRestoredCallback is nil in NewNetworkCheck")
	}
	n := NetworkCheck{
		logger:                 logger,
		gameID:                 gameID,
		gameCode:               gameCode,
		chEndLoop:              make(chan bool, 10),
		chPause:                make(chan bool, 10),
		chNewAction:            make(chan Action, 10),
		chClientAlive:          make(chan *AliveMsg, 10),
		clientDeadThresholdSec: uint32(util.Env.GetPingTimeout()),
		clientState:            nil,
		debugConnectivityCheck: util.Env.ShouldDebugConnectivityCheck(),
		crashHandler:           crashHandler,
		connLost:               connLostCallback,
		connRestored:           connRestoredCallback,
	}
	return &n
}

func (n *NetworkCheck) Run() {
	go n.loop()
}
func (n *NetworkCheck) Destroy() {
	n.chEndLoop <- true
}

func (n *NetworkCheck) loop() {
	n.logger.Info().Msg("Networkcheck loop starting")

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
			if n.debugConnectivityCheck {
				n.logger.Info().Uint64(logging.PlayerIDKey, action.PlayerID).Msg("New Action 2")
			}
			n.handleNewAction(action)
		case msg := <-n.chClientAlive:
			if n.debugConnectivityCheck {
				n.logger.Info().Uint64(logging.PlayerIDKey, msg.PlayerID).Msg("Client Alive 2")
			}
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

func (n *NetworkCheck) NewAction(a Action) {
	if n.debugConnectivityCheck {
		n.logger.Info().
			Uint64(logging.PlayerIDKey, a.PlayerID).
			Msg("New Action 1")
	}
	n.chNewAction <- a
}

func (n *NetworkCheck) ClientAlive(msg *AliveMsg) {
	if n.debugConnectivityCheck {
		n.logger.Info().
			Uint64(logging.PlayerIDKey, msg.PlayerID).
			Msg("Client Alive 1")
	}
	n.chClientAlive <- msg
}

func (n *NetworkCheck) Pause() {
	n.chPause <- true
}

func (n *NetworkCheck) handleNewAction(action Action) {
	if n.debugConnectivityCheck {
		n.logger.Info().
			Uint64(logging.PlayerIDKey, action.PlayerID).
			Msg("Handling new action player")
	}
	now := time.Now()
	n.clientState = &ClientAliveState{
		playerID:      action.PlayerID,
		lastAliveTime: now,
		connLost:      false,
	}
	n.currentAction = action

	n.paused = false
}

// Handle the alive msg from the client.
func (n *NetworkCheck) handleClientAlive(msg *AliveMsg) {
	if n.clientState == nil {
		n.logger.Warn().
			Uint64(logging.PlayerIDKey, msg.PlayerID).
			Msgf("handleClientAlive called when n.clientState is nil")
		return
	}

	if n.debugConnectivityCheck {
		if msg.PlayerID != n.clientState.playerID {
			n.logger.Info().Msgf("Ignoring alive msg from unexpected player. Current action player: %d, msg Player: %d", n.clientState.playerID, msg.PlayerID)
			return
		}
	}

	if n.debugConnectivityCheck {
		n.logger.Info().
			Uint64(logging.PlayerIDKey, msg.PlayerID).
			Msg("Handling client alive")
	}

	n.clientState.lastAliveTime = time.Now()
}

func (n *NetworkCheck) handlePause() {
	n.paused = true
}

func (n *NetworkCheck) handleTimeout() {
	if n.paused {
		return
	}
	if n.clientState == nil {
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

			n.clientState.connLost = false

			// Notify the game.
			n.connRestored(n.currentAction)
		}
	} else {
		if now.After(timeoutAt) {
			// Connection lost.
			n.logger.Info().
				Uint64(logging.PlayerIDKey, n.clientState.playerID).
				Msg("Player connectivity lost")

			n.clientState.connLost = true

			// Notify the game.
			n.connLost(n.currentAction)
		}
	}
}
