package game

import (
	"time"

	"voyager.com/logging"
	"voyager.com/server/timer"
)

func (g *Game) resetTimer(seatNo uint32, playerID uint64, canCheck bool, expireAt time.Time) {
	g.logger.Debug().
		Msgf("Resetting timer. Current timer seat: %d, player ID: %d, expires at %s (%.3f seconds from now)", seatNo, playerID, expireAt.Format(time.RFC3339), expireAt.Sub(time.Now()).Seconds())
	g.actionTimer.NewAction(timer.TimerMsg{
		SeatNo:   seatNo,
		PlayerID: playerID,
		ExpireAt: expireAt,
		CanCheck: canCheck,
	})
}

func (g *Game) resetTime(seatNo uint32, playerID uint64, remainingTime time.Duration) error {
	g.logger.Debug().
		Msgf("Resetting time for the current timer. Seat: %d, player ID: %d, remaining: %s", seatNo, playerID, remainingTime)
	_, err := g.actionTimer.ResetTime(timer.TimerResetTimeMsg{
		SeatNo:        seatNo,
		PlayerID:      playerID,
		RemainingTime: remainingTime,
	})
	return err
}

func (g *Game) extendTimer(seatNo uint32, playerID uint64, extendBy time.Duration) (uint32, error) {
	g.logger.Debug().
		Msgf("Extending timer. Seat: %d, Extend by %s", seatNo, extendBy)
	return g.actionTimer.Extend(timer.TimerExtendMsg{
		SeatNo:   seatNo,
		PlayerID: playerID,
		ExtendBy: extendBy,
	})
}

func (g *Game) runItTwiceTimer(seatNo uint32, playerID uint64, seatNo2 uint32, playerID2 uint64, expireAt time.Time) {
	g.logger.Debug().
		Msgf("Resetting timers for run-it-twice prompt. SeatNo 1: %d SeatNo 2: %d expires at %s (%.3f seconds from now)", seatNo, seatNo2, expireAt.Format(time.RFC3339), expireAt.Sub(time.Now()).Seconds())
	g.actionTimer.NewAction(timer.TimerMsg{
		SeatNo:     seatNo,
		PlayerID:   playerID,
		ExpireAt:   expireAt,
		RunItTwice: true,
	})
	g.actionTimer2.NewAction(timer.TimerMsg{
		SeatNo:     seatNo2,
		PlayerID:   playerID2,
		ExpireAt:   expireAt,
		RunItTwice: true,
	})
}

func (g *Game) pausePlayTimer(seatNo uint32) {
	actionResponseTime := g.actionTimer.GetElapsedTime()

	g.logger.Debug().
		Uint32(logging.SeatNumKey, seatNo).
		Msgf("Pausing timer. Player responded in: %.3fs", actionResponseTime.Seconds())
	g.actionTimer.Pause()
}

func (g *Game) pausePlayTimer2(seatNo uint32) {
	actionResponseTime := g.actionTimer2.GetElapsedTime()

	g.logger.Debug().
		Uint32(logging.SeatNumKey, seatNo).
		Msgf("Pausing timer 2. Player responded in: %.3fs", actionResponseTime.Seconds())
	g.actionTimer2.Pause()
}

func (g *Game) queueActionTimeoutMsg(msg timer.TimerMsg) {
	g.chPlayTimedOut <- msg
}

func (g *Game) handlePlayTimeout(timeoutMsg timer.TimerMsg) error {
	handState, err := g.loadHandState()
	if err != nil {
		return err
	}

	if timeoutMsg.RunItTwice {
		// The players did not respond to run it twice prompt
		// Force a default action for the timed-out player.
		handAction := HandAction{
			SeatNo:   timeoutMsg.SeatNo,
			Action:   ACTION_RUN_IT_TWICE_NO,
			TimedOut: true,
		}

		handMessage := HandMessage{
			HandNum:    handState.HandNum,
			HandStatus: handState.CurrentState,
			PlayerId:   timeoutMsg.PlayerID,
			SeatNo:     timeoutMsg.SeatNo,
			Messages: []*HandMessageItem{
				{
					MessageType: HandPlayerActed,
					Content:     &HandMessageItem_PlayerActed{PlayerActed: &handAction},
				},
			},
		}
		g.QueueHandMessage(&handMessage)
	} else {
		// Force a default action for the timed-out player.
		handAction := HandAction{
			SeatNo:   timeoutMsg.SeatNo,
			Action:   ACTION_FOLD,
			Amount:   0.0,
			TimedOut: true,
		}
		if timeoutMsg.CanCheck {
			handAction.Action = ACTION_CHECK
		}

		handMessage := HandMessage{
			HandNum:    handState.HandNum,
			HandStatus: handState.CurrentState,
			PlayerId:   timeoutMsg.PlayerID,
			SeatNo:     timeoutMsg.SeatNo,
			Messages: []*HandMessageItem{
				{
					MessageType: HandPlayerActed,
					Content:     &HandMessageItem_PlayerActed{PlayerActed: &handAction},
				},
			},
		}
		g.QueueHandMessage(&handMessage)
	}

	return nil
}
