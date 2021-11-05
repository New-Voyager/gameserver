package game

import (
	"sync"
)

// HandActionTracker is meant for remembering who did what at each stage of a hand.
// I.e., during pre-flop, seat 1 raised by 4, then seat 3 folded, etc.
type HandActionTracker struct {
	data map[HandStatus][]SeatAction
	sync.RWMutex
}

// SeatAction is a record of an action taken by a player.
type SeatAction struct {
	SeatNo   uint32
	Action   ACTION
	Amount   int64
	TimedOut bool
}

// NewHandActionTracker creates an instance of HandActionRecord.
func NewHandActionTracker() *HandActionTracker {
	d := make(map[HandStatus][]SeatAction)
	return &HandActionTracker{
		data: d,
	}
}

// RecordAction stores the action into the tracker.
func (h *HandActionTracker) RecordAction(seatNo uint32, action ACTION, amount int64, timedOut bool, handStatus HandStatus) {
	h.Lock()
	defer h.Unlock()
	actions, ok := h.data[handStatus]
	if !ok {
		actions = make([]SeatAction, 0)
	}
	actions = append(actions, SeatAction{
		SeatNo:   seatNo,
		Action:   action,
		Amount:   amount,
		TimedOut: timedOut,
	})
	h.data[handStatus] = actions
}

// GetActions returns all the seat actions that has been recorded so far for the requested handStatus.
func (h *HandActionTracker) GetActions(handStatus HandStatus) []SeatAction {
	h.Lock()
	defer h.Unlock()
	actions, ok := h.data[handStatus]
	if !ok {
		return make([]SeatAction, 0)
	}
	return actions
}

// GetActionsForSeat returns all the seat actions for one seat.
func (h *HandActionTracker) GetActionsForSeat(seatNo uint32, handStatus HandStatus) []SeatAction {
	h.RLock()
	defer h.RUnlock()
	seatActions := make([]SeatAction, 0)
	allSeatActions, ok := h.data[handStatus]
	if !ok {
		return seatActions
	}
	for _, action := range allSeatActions {
		if action.SeatNo == seatNo {
			seatActions = append(seatActions, action)
		}
	}
	return seatActions
}
