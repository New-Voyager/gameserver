package game

import (
	"fmt"
	"time"
)

type playerPingState struct {
	pingSeq      uint32
	pingSentTime time.Time

	pongSeq      uint32
	pongRecvTime time.Time

	numMissedPings uint32
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
	// Send ping to players.
	func() {
		g.pingStatesLock.Lock()
		defer g.pingStatesLock.Unlock()
		for _, seatPlayer := range g.PlayersInSeats {
			playerID := seatPlayer.PlayerID
			if playerID == 0 {
				continue
			}
			ps, exists := g.pingStates[playerID]
			if !exists {
				ps = &playerPingState{}
				g.pingStates[playerID] = ps
			}
			ps.pingSeq = pingSeq
			ps.pingSentTime = time.Now()
			g.sendPing(pingSeq, playerID)
		}
	}()
	time.Sleep(time.Duration(g.pingTimeoutSec) * time.Second)

	// Verify the responses (pong) have been received.
	var noPongPlayers []uint64
	func() {
		g.pingStatesLock.Lock()
		defer g.pingStatesLock.Unlock()
		for _, seatPlayer := range g.PlayersInSeats {
			playerID := seatPlayer.PlayerID
			if playerID == 0 {
				continue
			}
			player, exists := g.pingStates[playerID]
			if !exists {
				continue
			}
			if player.pongSeq != pingSeq {
				// Response (pong) not received in time.
				player.numMissedPings++
				channelGameLogger.Info().
					Uint32("club", g.config.ClubId).
					Str("game", g.config.GameCode).
					Msgf("Player %d missed %d ping(s)", playerID, player.numMissedPings)
				if player.numMissedPings > g.maxMissedPings {
					noPongPlayers = append(noPongPlayers, playerID)
				}
			} else {
				player.numMissedPings = 0
			}
		}
	}()

	// Announce (broadcast) network issue.
	if len(noPongPlayers) > 0 {
		g.broadcastNetworkIssue(noPongPlayers)
	}
}

func (g *Game) sendPing(pingSeq uint32, playerID uint64) error {
	msg := PingPongMessage{
		GameId:   g.config.GameId,
		PlayerId: playerID,
		Seq:      pingSeq,
	}
	g.sendPingMessageToPlayer(&msg, playerID)
	return nil
}

func (g *Game) broadcastNetworkIssue(networkIssuePlayers []uint64) {
	gameMessage := GameMessage{MessageType: GameNetworkIssue, GameId: g.config.GameId, PlayerId: 0}
	gameMessage.GameMessage = &GameMessage_NetworkIssue{
		NetworkIssue: &GameNetworkIssueMessage{
			PlayerIds: networkIssuePlayers,
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

func (g *Game) onPlayerPong(playerPongMsg *PingPongMessage) error {
	playerID := playerPongMsg.GetPlayerId()
	pongSeq := playerPongMsg.GetSeq()
	pongRecvTime := time.Now()

	fmt.Printf("PONG %d from player %d at %s", pongSeq, playerID, pongRecvTime.Format(time.RFC3339))

	g.pingStatesLock.Lock()
	defer g.pingStatesLock.Unlock()

	ps, exists := g.pingStates[playerID]
	if !exists {
		fmt.Println()
		return nil
	}

	if pongSeq > ps.pongSeq {
		if pongSeq > ps.pingSeq {
			// Should't happen.
		} else {
			if pongSeq == ps.pingSeq {
				fmt.Printf(" (received in %.3f seconds)", pongRecvTime.Sub(ps.pingSentTime).Seconds())
			}
			ps.pongSeq = pongSeq
			ps.pongRecvTime = pongRecvTime
		}
	}

	fmt.Println()
	return nil
}
