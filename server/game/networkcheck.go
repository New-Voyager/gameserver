package game

import (
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"voyager.com/server/util"
)

type playerPingState struct {
	pongSeq      uint32
	pongRecvTime time.Time
	connLost     bool
}

type NetworkCheck struct {
	logger                 *zerolog.Logger
	gameID                 uint64
	gameCode               string
	chEndLoop              chan bool
	playerIDsToPing        atomic.Value // []uint64
	pingTimeoutSec         uint32
	pingStates             map[uint64]*playerPingState
	pingStatesLock         sync.Mutex
	debugConnectivityCheck bool
	messageSender          *MessageSender
	crashHandler           func()
}

func NewNetworkCheck(
	logger *zerolog.Logger,
	gameID uint64,
	gameCode string,
	messageReceiver *MessageSender,
	crashHandler func(),
) *NetworkCheck {
	n := NetworkCheck{
		logger:                 logger,
		gameID:                 gameID,
		gameCode:               gameCode,
		chEndLoop:              make(chan bool, 10),
		pingTimeoutSec:         uint32(util.Env.GetPingTimeout()),
		debugConnectivityCheck: util.Env.ShouldDebugConnectivityCheck(),
		messageSender:          messageReceiver,
		crashHandler:           crashHandler,
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

	var paused bool
	var currentPingSeq uint32
	for {
		select {
		case <-n.chEndLoop:
			return
		default:
			if paused {
				break
			}
			currentPingSeq++

			n.doPingCheck(currentPingSeq, n.getPlayerIDs())
		}
		time.Sleep(1 * time.Second)
	}
}

func (n *NetworkCheck) SetPlayerIDs(playerIDs []uint64) {
	n.playerIDsToPing.Store(playerIDs)
}

func (n *NetworkCheck) getPlayerIDs() []uint64 {
	var playerIDs []uint64
	v := n.playerIDsToPing.Load()
	if v != nil {
		playerIDs = v.([]uint64)
	}
	return playerIDs
}

func (n *NetworkCheck) doPingCheck(pingSeq uint32, playerIDs []uint64) {
	if playerIDs == nil || len(playerIDs) == 0 {
		return
	}

	// Broadcast the ping to players.
	pingSentTime := func() time.Time {
		n.pingStatesLock.Lock()
		defer n.pingStatesLock.Unlock()
		pingStates := make(map[uint64]*playerPingState)
		for _, playerID := range playerIDs {
			ps, exists := n.pingStates[playerID]
			if exists {
				// This player was there for the previous ping. Continue the existing state.
				pingStates[playerID] = ps
			} else {
				// This is a new player that did not exist in the previous ping.
				// Start a new state for him.
				ps = &playerPingState{}
				pingStates[playerID] = ps
			}
		}
		n.pingStates = pingStates
		n.broadcastPing(pingSeq)
		return time.Now()
	}()

	// Give some time for all players to respond back.
	time.Sleep(time.Duration(n.pingTimeoutSec) * time.Second)

	// Verify the responses (pong) have been received.
	var connLostPlayers []uint64
	func() {
		n.pingStatesLock.Lock()
		defer n.pingStatesLock.Unlock()
		for _, playerID := range playerIDs {
			ps := n.pingStates[playerID]
			if ps.pongSeq == pingSeq {
				// Pong is received as expected.
				if n.debugConnectivityCheck {
					n.logger.Info().Msgf("Player %d pong response time: %.3f seconds\n", playerID, ps.pongRecvTime.Sub(pingSentTime).Seconds())
				}
			} else {
				// Response (pong) not received in time.
				ps.connLost = true
				connLostPlayers = append(connLostPlayers, playerID)
			}
		}
	}()

	// Announce the players who did not respond.
	if len(connLostPlayers) > 0 {
		n.broadcastConnectivityLost(connLostPlayers)
	}
}

func (n *NetworkCheck) broadcastPing(pingSeq uint32) error {
	msg := PingPongMessage{
		GameId: n.gameID,
		Seq:    pingSeq,
	}
	n.broadcastPingMessage(&msg)
	return nil
}

func (n *NetworkCheck) broadcastConnectivityLost(playerIDs []uint64) {
	gameMessage := GameMessage{
		MessageType: GamePlayerConnectivityLost,
		GameId:      n.gameID,
		PlayerId:    0,
	}
	gameMessage.GameMessage = &GameMessage_NetworkConnectivity{
		NetworkConnectivity: &GameNetworkConnectivityMessage{
			PlayerIds: playerIDs,
		},
	}
	n.broadcastGameMessage(&gameMessage)
}

func (n *NetworkCheck) broadcastPingMessage(msg *PingPongMessage) error {
	if *n.messageSender != nil {
		msg.GameCode = n.gameCode
		(*n.messageSender).BroadcastPingMessage(msg)
	}
	return nil
}

func (n *NetworkCheck) broadcastGameMessage(msg *GameMessage) error {
	if *n.messageSender != nil {
		msg.GameCode = n.gameCode
		skipLog := !n.debugConnectivityCheck
		(*n.messageSender).BroadcastGameMessage(msg, skipLog)
	}
	return nil
}

func (n *NetworkCheck) handlePongMessage(message *PingPongMessage) {
	err := n.onPlayerResponse(message)
	if err != nil {
		n.logger.Error().Msgf("Error while processing pong message. Error: %s", err.Error())
	}
}

// Triggered when a player response (pong) comes back.
func (n *NetworkCheck) onPlayerResponse(playerPongMsg *PingPongMessage) error {
	playerID := playerPongMsg.GetPlayerId()
	pongSeq := playerPongMsg.GetSeq()
	pongRecvTime := time.Now()

	if n.debugConnectivityCheck {
		n.logger.Info().Msgf("PONG %d from player %d at %s\n", pongSeq, playerID, pongRecvTime.Format(time.RFC3339))
	}

	n.pingStatesLock.Lock()
	defer n.pingStatesLock.Unlock()

	ps, exists := n.pingStates[playerID]
	if !exists {
		return nil
	}

	if pongSeq > ps.pongSeq {
		ps.pongSeq = pongSeq
		ps.pongRecvTime = pongRecvTime
	}

	if ps.connLost {
		// Player had previously lost connectivity, but is back online.
		ps.connLost = false

		// Immediately notify that this player is back on.
		n.broadcastConnectivityRestored([]uint64{playerID})
	}
	return nil
}

func (n *NetworkCheck) broadcastConnectivityRestored(playerIDs []uint64) {
	gameMessage := GameMessage{
		MessageType: GamePlayerConnectivityRestored,
		GameId:      n.gameID,
		PlayerId:    0,
	}
	gameMessage.GameMessage = &GameMessage_NetworkConnectivity{
		NetworkConnectivity: &GameNetworkConnectivityMessage{
			PlayerIds: playerIDs,
		},
	}
	n.broadcastGameMessage(&gameMessage)
}
