package game

import (
	"fmt"
	"time"
)

type playerPingState struct {
	pongSeq      uint32
	pongRecvTime time.Time
	connLost     bool
}

func (g *Game) startPingLoop(stop <-chan bool) {
	var paused bool
	var currentPingSeq uint32
	for {
		select {
		case <-stop:
			return
		default:
			if paused {
				break
			}
			currentPingSeq++
			g.doPingCheck(currentPingSeq)
		}
		time.Sleep(1 * time.Second)
	}
}

func (g *Game) doPingCheck(pingSeq uint32) {
	// Get the list of player ID's that we are interested in getting the response (pong) back.
	// Those are the players that are currently sitting in the table.
	var seatedPlayerIds []uint64
	for i := 0; i < len(g.PlayersInSeats); i++ {
		playerID := g.PlayersInSeats[i].PlayerID
		if playerID == 0 {
			continue
		}
		seatedPlayerIds = append(seatedPlayerIds, playerID)
	}

	if seatedPlayerIds == nil {
		return
	}

	// Broadcast the ping to players.
	pingSentTime := func() time.Time {
		g.pingStatesLock.Lock()
		defer g.pingStatesLock.Unlock()
		pingStates := make(map[uint64]*playerPingState)
		for _, playerID := range seatedPlayerIds {
			ps, exists := g.pingStates[playerID]
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
		g.pingStates = pingStates
		g.broadcastPing(pingSeq)
		return time.Now()
	}()

	// Give some time for all players to respond back.
	time.Sleep(time.Duration(g.pingTimeoutSec) * time.Second)

	// Verify the responses (pong) have been received.
	var connLostPlayers []uint64
	func() {
		g.pingStatesLock.Lock()
		defer g.pingStatesLock.Unlock()
		for _, playerID := range seatedPlayerIds {
			ps := g.pingStates[playerID]
			if ps.pongSeq == pingSeq {
				// Pong is received as expected.
				if g.debugConnectivityCheck {
					fmt.Printf("Player %d pong response time: %.3f seconds\n", playerID, ps.pongRecvTime.Sub(pingSentTime).Seconds())
				}
			} else {
				// Response (pong) not received in time.
				ps.connLost = true
				connLostPlayers = append(connLostPlayers, playerID)
			}
		}
	}()

	// Announce the players who lost connectivity.
	if len(connLostPlayers) > 0 {
		g.broadcastConnectivityLost(connLostPlayers)
	}
}

func (g *Game) broadcastPing(pingSeq uint32) error {
	msg := PingPongMessage{
		GameId: g.config.GameId,
		Seq:    pingSeq,
	}
	g.BroadcastPingMessage(&msg)
	return nil
}

func (g *Game) broadcastConnectivityLost(connectivityLostPlayers []uint64) {
	gameMessage := GameMessage{
		MessageType: GamePlayerConnectivityLost,
		GameId:      g.config.GameId,
		PlayerId:    0,
	}
	gameMessage.GameMessage = &GameMessage_NetworkConnectivity{
		NetworkConnectivity: &GameNetworkConnectivityMessage{
			PlayerIds: connectivityLostPlayers,
		},
	}
	g.broadcastGameMessage(&gameMessage)
}

func (g *Game) broadcastConnectivityRestored(connectivityRestoredPlayers []uint64) {
	gameMessage := GameMessage{
		MessageType: GamePlayerConnectivityRestored,
		GameId:      g.config.GameId,
		PlayerId:    0,
	}
	gameMessage.GameMessage = &GameMessage_NetworkConnectivity{
		NetworkConnectivity: &GameNetworkConnectivityMessage{
			PlayerIds: connectivityRestoredPlayers,
		},
	}
	g.broadcastGameMessage(&gameMessage)
}

func (g *Game) handlePongMessage(message *PingPongMessage) {
	err := g.onPlayerPong(message)
	if err != nil {
		channelGameLogger.Error().Msgf("Error while processing pong message. Error: %s", err.Error())
	}
}

// Triggered when a player response (pong) comes back.
func (g *Game) onPlayerPong(playerPongMsg *PingPongMessage) error {
	playerID := playerPongMsg.GetPlayerId()
	pongSeq := playerPongMsg.GetSeq()
	pongRecvTime := time.Now()

	if g.debugConnectivityCheck {
		fmt.Printf("PONG %d from player %d at %s\n", pongSeq, playerID, pongRecvTime.Format(time.RFC3339))
	}

	g.pingStatesLock.Lock()
	defer g.pingStatesLock.Unlock()

	ps, exists := g.pingStates[playerID]
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
		g.broadcastConnectivityRestored([]uint64{playerID})
	}
	return nil
}
