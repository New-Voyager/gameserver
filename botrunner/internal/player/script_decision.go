package player

import (
	"fmt"

	"voyager.com/botrunner/internal/game"
	"voyager.com/gamescript"
)

// ScriptBasedDecision decides the bot's next move based on the pre-defined scenario.
type ScriptBasedDecision struct{}

type scriptSeatAction struct {
	seatNo uint32
	action game.ACTION
	amount float32
}

// GetNextAction returns the bot's next move based on the bot's script.
func (s *ScriptBasedDecision) GetNextAction(bot *BotPlayer, availableActions []game.ACTION) (game.ACTION, float32, error) {
	handStatus := bot.game.table.handStatus
	handNumIdx := bot.handNum - 1
	handScript := bot.config.Script.Hands[handNumIdx]
	playersActed := bot.game.table.playersActed
	var scriptSeatActions []gamescript.SeatAction
	switch handStatus {
	case game.HandStatus_PREFLOP:
		scriptSeatActions = handScript.Preflop.SeatActions
	case game.HandStatus_FLOP:
		scriptSeatActions = handScript.Flop.SeatActions
	case game.HandStatus_TURN:
		scriptSeatActions = handScript.Turn.SeatActions
	case game.HandStatus_RIVER:
		scriptSeatActions = handScript.River.SeatActions
	}

	var scriptActionEntries []scriptSeatAction
	for _, entry := range scriptSeatActions {
		scriptActionEntries = append(scriptActionEntries, scriptSeatAction{
			seatNo: entry.Action.SeatNo,
			action: game.ActionStringToAction(entry.Action.Action),
			amount: entry.Action.Amount,
		})
	}

	var nextSeat uint32
	var nextAction game.ACTION
	var nextAmount float32
	var err error
	var idx int
	// fmt.Printf("bot.seatNo: %d\n", bot.seatNo)
	for idx = 0; idx < len(scriptActionEntries); idx++ {
		if scriptActionEntries[idx].seatNo != bot.seatNo {
			continue
		}
		roundScriptActions := s.getScriptActionsForRound(scriptActionEntries, idx)
		if s.scriptRoundMatchPlayersActed(roundScriptActions, playersActed) {
			nextSeat = scriptActionEntries[idx].seatNo
			nextAction = scriptActionEntries[idx].action
			nextAmount = scriptActionEntries[idx].amount
		}
	}

	if nextSeat == 0 {
		err = fmt.Errorf("%s: Unable to find next action from script", bot.logPrefix)
	} else if nextSeat != bot.seatNo {
		err = fmt.Errorf("%s: Scripted next action seat number [%d] does not match the bot's seat [%d]", bot.logPrefix, nextSeat, bot.seatNo)
	} else if !game.ActionContains(availableActions, nextAction) {
		err = fmt.Errorf("%s: Scripted next action [%s] is not one of the available actions [%v]", bot.logPrefix, nextAction, availableActions)
	}

	return nextAction, nextAmount, err
}

func (s *ScriptBasedDecision) getScriptActionsForRound(scriptActionEntries []scriptSeatAction, lastPlayerIdx int) []scriptSeatAction {
	var roundScriptActions []scriptSeatAction
	lastPlayerInRound := scriptActionEntries[lastPlayerIdx]
	idx := lastPlayerIdx - 1
	for idx >= 0 {
		roundScriptActions = append(roundScriptActions, scriptActionEntries[idx])
		if scriptActionEntries[idx].seatNo == lastPlayerInRound.seatNo {
			break
		}
		idx--
	}
	return roundScriptActions
}

func (s *ScriptBasedDecision) scriptRoundMatchPlayersActed(roundScriptActions []scriptSeatAction, playersActed map[uint32]*game.PlayerActRound) bool {
	// fmt.Printf("roundScriptActions: %+v\nplayersActed: %+v\n", roundScriptActions, playersActed)
	scriptActionState := make(map[uint32]*game.PlayerActRound)
	for _, scriptAction := range roundScriptActions {
		playerActRound, ok := playersActed[scriptAction.seatNo]
		if !ok {
			return false
		}
		if game.ActionToActionState(scriptAction.action) != playerActRound.GetState() {
			return false
		}
		if scriptAction.action != game.ACTION_FOLD && scriptAction.amount != playerActRound.GetAmount() {
			return false
		}
		scriptActionState[scriptAction.seatNo] = &game.PlayerActRound{
			State:  game.ActionToActionState(scriptAction.action),
			Amount: scriptAction.amount,
		}
	}

	for seatNo, playerActRound := range playersActed {
		_, present := scriptActionState[seatNo]
		state := playerActRound.GetState()
		if !present && state != game.PlayerActState_PLAYER_ACT_BB && state != game.PlayerActState_PLAYER_ACT_EMPTY_SEAT && state != game.PlayerActState_PLAYER_ACT_FOLDED && state != game.PlayerActState_PLAYER_ACT_NOT_ACTED {
			return false
		}
	}

	return true
}
