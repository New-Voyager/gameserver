package timer

import (
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

var actionTimerLogger = log.With().Str("logger_name", "game::action_timer").Logger()

type TimerMsg struct {
	SeatNo           uint32
	PlayerID         uint64
	CurrentActionNum uint32
	CanCheck         bool
	ExpireAt         time.Time
	RunItTwice       bool
}

type ActionTimer struct {
	gameCode string

	chReset   chan TimerMsg
	chPause   chan bool
	chEndLoop chan bool

	callback        func(TimerMsg)
	currentTimerMsg TimerMsg

	secondsTillTimeout uint32
	lastResetAt        time.Time

	crashHandler func()
}

func NewActionTimer(gameCode string, callback func(TimerMsg), crashHandler func()) *ActionTimer {
	at := ActionTimer{
		gameCode:     gameCode,
		chReset:      make(chan TimerMsg),
		chPause:      make(chan bool),
		chEndLoop:    make(chan bool, 10),
		callback:     callback,
		crashHandler: crashHandler,
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
			actionTimerLogger.Error().
				Str("game", a.gameCode).
				Msgf("Action timer loop returning due to panic: %s\nStack Trace:\n%s", err, string(debug.Stack()))

			a.crashHandler()
		} else {
			actionTimerLogger.Info().Str("game", a.gameCode).Msg("Action timer loop returning")
		}
	}()

	var expirationTime time.Time
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
			expirationTime = msg.ExpireAt
			paused = false
		default:
			if !paused {
				remainingSec := expirationTime.Sub(time.Now()).Seconds()
				if remainingSec < 0 {
					remainingSec = 0
				}
				// track remainingActionTime to show the new observer how much time the current player has to act
				a.secondsTillTimeout = uint32(remainingSec)

				if remainingSec <= 0 {
					// The player timed out.
					a.callback(a.currentTimerMsg)
					expirationTime = time.Time{}
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

func (a *ActionTimer) Reset(t TimerMsg) error {
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

func (a *ActionTimer) GetElapsedTime() time.Duration {
	return time.Now().Sub(a.lastResetAt)
}

func (a *ActionTimer) GetRemainingSec() uint32 {
	return a.secondsTillTimeout
}

func (a *ActionTimer) GetCurrentTimerMsg() TimerMsg {
	return a.currentTimerMsg
}
