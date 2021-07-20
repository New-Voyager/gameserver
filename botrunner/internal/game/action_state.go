package game

import "fmt"

// ActionToActionState converts ACTION to the corresponding PlayerActState.
func ActionToActionState(action ACTION) (PlayerActState, error) {
	var state PlayerActState
	switch action {
	case ACTION_BB:
		state = PlayerActState_PLAYER_ACT_BB
	case ACTION_SB:
		state = PlayerActState_PLAYER_ACT_NOT_ACTED
	case ACTION_STRADDLE:
		state = PlayerActState_PLAYER_ACT_STRADDLE
	case ACTION_CHECK:
		state = PlayerActState_PLAYER_ACT_CHECK
	case ACTION_CALL:
		state = PlayerActState_PLAYER_ACT_CALL
	case ACTION_FOLD:
		state = PlayerActState_PLAYER_ACT_FOLDED
	case ACTION_BET:
		state = PlayerActState_PLAYER_ACT_BET
	case ACTION_RAISE:
		state = PlayerActState_PLAYER_ACT_RAISE
	case ACTION_ALLIN:
		state = PlayerActState_PLAYER_ACT_ALL_IN
	default:
		return state, fmt.Errorf("Unknown action - %s", action)
	}
	return state, nil
}
