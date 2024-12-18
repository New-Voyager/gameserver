package timer

import (
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

type TimerMsg struct {
	SeatNo           uint32
	PlayerID         uint64
	CurrentActionNum uint32
	CanCheck         bool
	ExpireAt         time.Time
	RunItTwice       bool
	ActionID         string
}

type TimerExtendMsg struct {
	SeatNo   uint32
	PlayerID uint64
	ExtendBy time.Duration
	ActionID string
}

type TimerResetTimeMsg struct {
	SeatNo        uint32
	PlayerID      uint64
	RemainingTime time.Duration
	ActionID      string
}

type ActionTimer struct {
	logger *zerolog.Logger

	chReset        chan TimerMsg
	chExtend       chan TimerExtendMsg
	chResetTime    chan TimerResetTimeMsg
	chPause        chan bool
	chRemainingIn  chan bool
	chRemainingOut chan time.Duration
	chEndLoop      chan bool

	currentTimerMsg TimerMsg
	expirationTime  time.Time
	lastResetAt     time.Time

	timedOut     func(TimerMsg)
	crashHandler func()
}

func NewActionTimer(logger *zerolog.Logger, callback func(TimerMsg), crashHandler func()) *ActionTimer {
	at := ActionTimer{
		logger:         logger,
		chReset:        make(chan TimerMsg),
		chResetTime:    make(chan TimerResetTimeMsg),
		chExtend:       make(chan TimerExtendMsg),
		chPause:        make(chan bool),
		chRemainingIn:  make(chan bool),
		chRemainingOut: make(chan time.Duration),
		chEndLoop:      make(chan bool, 10),
		timedOut:       callback,
		crashHandler:   crashHandler,
	}
	return &at
}

func (a *ActionTimer) Run() {
	go a.loop()
}

func (a *ActionTimer) Destroy() {
	a.chEndLoop <- true
}

func (a *ActionTimer) loop() {
	defer func() {
		err := recover()
		if err != nil {
			// Panic occurred.
			debug.PrintStack()
			a.logger.Error().
				Msgf("Action timer loop returning due to panic: %s\nStack Trace:\n%s", err, string(debug.Stack()))

			a.crashHandler()
		} else {
			a.logger.Info().Msg("Action timer loop returning")
		}
	}()

	paused := true
	for {
		select {
		case <-a.chEndLoop:
			return
		case <-a.chPause:
			paused = true
		case msg := <-a.chReset:
			// Start the new timer.
			a.currentTimerMsg = msg
			a.expirationTime = msg.ExpireAt
			paused = false
		case msg := <-a.chResetTime:
			if msg.PlayerID != a.currentTimerMsg.PlayerID {
				a.logger.Info().Msgf("Player ID (%d) does not match the existing timer (%d). Ignoring the request to reset the action timer.", msg.PlayerID, a.currentTimerMsg.PlayerID)
				break
			}
			a.expirationTime = time.Now().Add(msg.RemainingTime)
		case msg := <-a.chExtend:
			// Extend the existing timer.
			if msg.PlayerID != a.currentTimerMsg.PlayerID {
				a.logger.Info().Msgf("Player ID (%d) does not match the existing timer (%d). Ignoring the request to extend the action timer.", msg.PlayerID, a.currentTimerMsg.PlayerID)
				break
			}
			a.expirationTime = a.expirationTime.Add(msg.ExtendBy)
		case <-a.chRemainingIn:
			remaining := a.expirationTime.Sub(time.Now())
			a.chRemainingOut <- remaining
		default:
			if !paused {
				remainingSec := a.expirationTime.Sub(time.Now()).Seconds()
				if remainingSec < 0 {
					remainingSec = 0
				}

				if remainingSec <= 0 {
					// The player timed out.
					a.timedOut(a.currentTimerMsg)
					a.expirationTime = time.Time{}
					paused = true
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (a *ActionTimer) Pause() {
	a.chPause <- true
}

func (a *ActionTimer) NewAction(t TimerMsg) error {
	var errMsgs []string
	if t.SeatNo == 0 {
		errMsgs = append(errMsgs, "invalid seatNo")
	}
	if t.PlayerID == 0 {
		errMsgs = append(errMsgs, "invalid playerID")
	}
	if time.Time.IsZero(t.ExpireAt) {
		errMsgs = append(errMsgs, "invalid expireAt")
	}
	if len(errMsgs) > 0 {
		return fmt.Errorf(strings.Join(errMsgs, "; "))
	}
	a.lastResetAt = time.Now()
	a.chReset <- t
	return nil
}

func (a *ActionTimer) Extend(t TimerExtendMsg) (uint32, error) {
	var errMsgs []string
	if t.SeatNo == 0 {
		errMsgs = append(errMsgs, "invalid seatNo")
	}
	if t.PlayerID == 0 {
		errMsgs = append(errMsgs, "invalid playerID")
	}
	if t.ActionID != a.currentTimerMsg.ActionID {
		errMsgs = append(errMsgs, fmt.Sprintf("invalid action ID %s expected %s", t.ActionID, a.currentTimerMsg.ActionID))
	}
	if len(errMsgs) > 0 {
		return 0, fmt.Errorf(strings.Join(errMsgs, "; "))
	}
	a.chExtend <- t
	return a.GetRemainingSec(), nil
}

func (a *ActionTimer) ResetTime(t TimerResetTimeMsg) (uint32, error) {
	var errMsgs []string
	if t.SeatNo == 0 {
		errMsgs = append(errMsgs, "invalid seatNo")
	}
	if t.PlayerID == 0 {
		errMsgs = append(errMsgs, "invalid playerID")
	}
	if t.ActionID != a.currentTimerMsg.ActionID {
		errMsgs = append(errMsgs, fmt.Sprintf("invalid action ID %s expected %s", t.ActionID, a.currentTimerMsg.ActionID))
	}
	if len(errMsgs) > 0 {
		return 0, fmt.Errorf(strings.Join(errMsgs, "; "))
	}
	a.chResetTime <- t
	return a.GetRemainingSec(), nil
}

func (a *ActionTimer) GetElapsedTime() time.Duration {
	return time.Now().Sub(a.lastResetAt)
}

func (a *ActionTimer) GetRemainingSec() uint32 {
	a.chRemainingIn <- true
	remaining := <-a.chRemainingOut
	remainingSec := remaining.Seconds()
	if remainingSec < 0 {
		remainingSec = 0
	}
	return uint32(remainingSec)
}

func (a *ActionTimer) GetCurrentTimerMsg() TimerMsg {
	return a.currentTimerMsg
}
