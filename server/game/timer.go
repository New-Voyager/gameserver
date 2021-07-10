package game

import (
	"fmt"
	"time"

	"voyager.com/server/timer"
)

func (g *Game) resetTimer(seatNo uint32, playerID uint64, canCheck bool, expireAt time.Time) {
	channelGameLogger.Info().Msgf("Resetting timer. Current timer seat: %d expires at %s (%f seconds from now)", seatNo, expireAt, expireAt.Sub(time.Now()).Seconds())
	fmt.Printf("Resetting timer. Current timer seat: %d timer: %d\n", seatNo, g.config.ActionTime)
	g.actionTimer.Reset(timer.TimerMsg{
		SeatNo:   seatNo,
		PlayerID: playerID,
		ExpireAt: expireAt,
		CanCheck: canCheck,
	})
}

func (g *Game) runItTwiceTimer(seatNo uint32, playerID uint64, seatNo2 uint32, playerID2 uint64, expireAt time.Time) {
	channelGameLogger.Info().Msgf("Resetting timer for run-it-twice prompt. SeatNo 1: %d SeatNo 2: %d expires at %s (%f seconds from now)", seatNo, seatNo2, expireAt, expireAt.Sub(time.Now()).Seconds())
	fmt.Printf("Resetting timer. Current timer seat: %d timer: %d\n", seatNo, g.config.ActionTime)
	g.actionTimer.Reset(timer.TimerMsg{
		SeatNo:     seatNo,
		PlayerID:   playerID,
		ExpireAt:   expireAt,
		SeatNo2:    seatNo2,
		PlayerID2:  playerID2,
		RunItTwice: true,
	})
}

func (g *Game) pausePlayTimer(seatNo uint32) {
	actionResponseTime := g.actionTimer.GetElapsedTime()

	fmt.Printf("Pausing timer. Seat responded seat: %d Responded in: %fs \n", seatNo, actionResponseTime.Seconds())
	g.actionTimer.Pause()
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
		// the players did not respond to run it twice prompt
		g.handleRunitTwiceTimeout(handState)
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
			GameId:     g.config.GameId,
			ClubId:     g.config.ClubId,
			HandNum:    handState.HandNum,
			HandStatus: handState.CurrentState,
			PlayerId:   timeoutMsg.PlayerID,
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
