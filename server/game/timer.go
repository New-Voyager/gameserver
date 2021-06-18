package game

import (
	"fmt"
	"time"
)

type timerMsg struct {
	seatNo           uint32
	playerID         uint64
	currentActionNum uint32
	canCheck         bool
	expireAt         time.Time
	seatNo2          uint32
	playerID2        uint64
	runItTwice       bool
}

func (g *Game) timerLoop(stop <-chan bool, pause <-chan bool) {
	var currentTimerMsg timerMsg
	var expirationTime time.Time
	paused := true
	for {
		select {
		case <-stop:
			return
		case <-pause:
			paused = true
		case msg := <-g.chResetTimer:
			// Start the new timer.
			currentTimerMsg = msg
			expirationTime = msg.expireAt
			paused = false
		default:
			if !paused {
				remainingTime := expirationTime.Sub(time.Now()).Seconds()
				if remainingTime < 0 {
					remainingTime = 0
				}
				// track remainingActionTime to show the new observer how much time the current player has to act
				g.RemainingActionTime = uint32(remainingTime)

				if remainingTime <= 0 {
					// The player timed out.
					g.chPlayTimedOut <- currentTimerMsg
					expirationTime = time.Time{}
					paused = true
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (g *Game) resetTimer(seatNo uint32, playerID uint64, canCheck bool, expireAt time.Time) {
	channelGameLogger.Info().Msgf("Resetting timer. Current timer seat: %d expires at %s (%f seconds from now)", seatNo, expireAt, expireAt.Sub(time.Now()).Seconds())
	fmt.Printf("Resetting timer. Current timer seat: %d timer: %d\n", seatNo, g.config.ActionTime)
	g.timerSeatNo = seatNo
	g.actionTimeStart = time.Now()
	g.chResetTimer <- timerMsg{
		seatNo:   seatNo,
		playerID: playerID,
		expireAt: expireAt,
		canCheck: canCheck,
	}
}

func (g *Game) runItTwiceTimer(seatNo uint32, playerID uint64, seatNo2 uint32, playerID2 uint64, expireAt time.Time) {
	channelGameLogger.Info().Msgf("Resetting timer for run-it-twice prompt. SeatNo 1: %d SeatNo 2: %d expires at %s (%f seconds from now)", seatNo, seatNo2, expireAt, expireAt.Sub(time.Now()).Seconds())
	fmt.Printf("Resetting timer. Current timer seat: %d timer: %d\n", seatNo, g.config.ActionTime)
	g.timerSeatNo = seatNo
	g.actionTimeStart = time.Now()
	g.chResetTimer <- timerMsg{
		seatNo:     seatNo,
		playerID:   playerID,
		expireAt:   expireAt,
		seatNo2:    seatNo2,
		playerID2:  playerID2,
		runItTwice: true,
	}
}

func (g *Game) pausePlayTimer(seatNo uint32) {
	actionResponseTime := time.Now().Sub(g.actionTimeStart)

	fmt.Printf("Pausing timer. Seat responded seat: %d Responded in: %fs \n", seatNo, actionResponseTime.Seconds())
	g.chPauseTimer <- true
}

func (g *Game) handlePlayTimeout(timeoutMsg timerMsg) error {
	handState, err := g.loadHandState()
	if err != nil {
		return err
	}

	if timeoutMsg.runItTwice {
		// the players did not respond to run it twice prompt
		g.handleRunitTwiceTimeout(handState)
	} else {
		// Force a default action for the timed-out player.
		handAction := HandAction{
			SeatNo:   timeoutMsg.seatNo,
			Action:   ACTION_FOLD,
			Amount:   0.0,
			TimedOut: true,
		}
		if timeoutMsg.canCheck {
			handAction.Action = ACTION_CHECK
		}

		handMessage := HandMessage{
			GameId:     g.config.GameId,
			ClubId:     g.config.ClubId,
			HandNum:    handState.HandNum,
			HandStatus: handState.CurrentState,
			PlayerId:   timeoutMsg.playerID,
			Messages: []*HandMessageItem{
				{
					MessageType: HandPlayerActed,
					Content:     &HandMessageItem_PlayerActed{PlayerActed: &handAction},
				},
			},
		}
		g.SendHandMessage(&handMessage)
	}

	return nil
}
