package game

import (
	"runtime/debug"
	"time"

	"github.com/rs/zerolog"
	"voyager.com/logging"
	"voyager.com/server/util"
)

type pingPongState struct {
	playerID       uint64
	pingSeq        uint32
	pongSeq        uint32
	pingSentTime   time.Time
	pingTimesoutAt time.Time
	connLost       bool
}

type NewAction struct {
	PlayerID        uint64
	SendInitialPing bool
}

type NetworkCheck2 struct {
	logger                 *zerolog.Logger
	gameID                 uint64
	gameCode               string
	chEndLoop              chan bool
	chPause                chan bool
	chNewAction            chan NewAction
	chPong                 chan *PingPongMessage
	pingTimeoutSec         uint32
	pingPongState          *pingPongState
	paused                 bool
	debugConnectivityCheck bool
	messageSender          *MessageSender
	crashHandler           func()
}

func NewNetworkCheck2(
	logger *zerolog.Logger,
	gameID uint64,
	gameCode string,
	messageReceiver *MessageSender,
	crashHandler func(),
) *NetworkCheck2 {
	n := NetworkCheck2{
		logger:                 logger,
		gameID:                 gameID,
		gameCode:               gameCode,
		chEndLoop:              make(chan bool, 10),
		chPause:                make(chan bool, 10),
		chNewAction:            make(chan NewAction, 10),
		chPong:                 make(chan *PingPongMessage, 10),
		pingTimeoutSec:         uint32(util.Env.GetPingTimeout()),
		pingPongState:          nil,
		debugConnectivityCheck: util.Env.ShouldDebugConnectivityCheck(),
		messageSender:          messageReceiver,
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
		case msg := <-n.chPong:
			n.handlePongMsg(msg)
		case <-n.chPause:
			n.handlePause()
		case <-n.chEndLoop:
			return
		default:
			n.processPeriodic()
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (n *NetworkCheck2) handlePause() {
	n.paused = true
}

func (n *NetworkCheck2) processPeriodic() {
	if n.paused {
		return
	}

	now := time.Now()

	if now.Before(n.pingPongState.pingTimesoutAt) {
		return
	}

	if n.pingPongState.pongSeq != n.pingPongState.pingSeq {
		// Did not receive pong in this round.
		// Broadcast connection issue if not already done.
		if !n.pingPongState.connLost {
			if n.debugConnectivityCheck {
				n.logger.Info().
					Uint64(logging.PlayerIDKey, n.pingPongState.playerID).
					Msg("Player connectivity lost")
			}
			n.pingPongState.connLost = true
			n.broadcastConnectivityLost([]uint64{n.pingPongState.playerID})
		}
	}

	// Send next ping.
	n.pingPongState.pingSeq++
	n.sendPing(n.pingPongState.pingSeq, n.pingPongState.playerID)

	now = time.Now()
	n.pingPongState.pingSentTime = now
	n.pingPongState.pingTimesoutAt = now.Add(time.Duration(n.pingTimeoutSec) * time.Second)
}

func (n *NetworkCheck2) handleNewAction(action NewAction) {
	var firstPingSeq uint32 = 1

	if action.SendInitialPing {
		n.sendPing(firstPingSeq, n.pingPongState.playerID)
	}
	now := time.Now()
	n.pingPongState = &pingPongState{
		playerID:       action.PlayerID,
		pingSeq:        firstPingSeq,
		pongSeq:        0,
		pingSentTime:   now,
		pingTimesoutAt: now.Add(time.Duration(n.pingTimeoutSec) * time.Second),
		connLost:       false,
	}

	n.paused = false
}

// Handle the response from the client.
func (n *NetworkCheck2) handlePongMsg(msg *PingPongMessage) {
	if n.paused {
		return
	}

	if msg.PlayerId != n.pingPongState.playerID {
		n.logger.Info().Msgf("Ignoring pong msg from unexpected player. Current player: %d, msg Player: %d", n.pingPongState.playerID, msg.PlayerId)
		return
	}

	if msg.Seq <= n.pingPongState.pongSeq {
		n.logger.Info().
			Uint64(logging.PlayerIDKey, msg.PlayerId).
			Msgf("Ignoring expired/duplicate pong msg. Seq: %d, last seq: %d", msg.Seq, n.pingPongState.pongSeq)
		return
	}

	n.pingPongState.pongSeq = msg.Seq

	if n.debugConnectivityCheck {
		responseTime := time.Now().Sub(n.pingPongState.pingSentTime)
		n.logger.Info().
			Uint64(logging.PlayerIDKey, msg.PlayerId).
			Msgf("Pong response time: %.3f seconds", responseTime.Seconds())
	}

	if msg.Seq == n.pingPongState.pingSeq && n.pingPongState.connLost {
		n.pingPongState.connLost = false
		n.logger.Info().
			Uint64(logging.PlayerIDKey, msg.PlayerId).
			Msgf("Player connectivity restored")

		n.broadcastConnectivityRestored([]uint64{msg.PlayerId})
	}
}

func (n *NetworkCheck2) sendPing(pingSeq uint32, playerID uint64) error {
	msg := PingPongMessage{
		GameId:   n.gameID,
		GameCode: n.gameCode,
		PlayerId: playerID,
		Seq:      pingSeq,
	}
	n.SendPingMessageToPlayer(&msg, playerID)
	return nil
}

func (n *NetworkCheck2) broadcastConnectivityLost(playerIDs []uint64) {
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

func (n *NetworkCheck2) SendPingMessageToPlayer(msg *PingPongMessage, playerID uint64) error {
	if *n.messageSender != nil {
		(*n.messageSender).SendPingMessageToPlayer(msg, playerID)
	}
	return nil
}

func (n *NetworkCheck2) broadcastGameMessage(msg *GameMessage) error {
	if *n.messageSender != nil {
		msg.GameCode = n.gameCode
		skipLog := !n.debugConnectivityCheck
		(*n.messageSender).BroadcastGameMessage(msg, skipLog)
	}
	return nil
}

func (n *NetworkCheck2) broadcastConnectivityRestored(playerIDs []uint64) {
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
