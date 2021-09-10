package player

import (
	"fmt"

	"voyager.com/botrunner/internal/game"
	"voyager.com/gamescript"
)

// ScriptBasedDecision decides the bot's next move based on the pre-defined scenario.
type ScriptBasedDecision struct{}

// GetNextAction returns the bot's next move based on the bot's script.
func (s *ScriptBasedDecision) GetNextAction(bot *BotPlayer, availableActions []game.ACTION) (gamescript.SeatAction, error) {
	script := bot.config.Script
	handNum := bot.game.handNum
	handStatus := bot.game.handStatus
	actionHistory := bot.game.table.actionTracker.GetActions(handStatus)
	nextActionIdx := len(actionHistory)
	nextSeatAction := s.getNthActionFromScript(script, handNum, handStatus, nextActionIdx)
	return nextSeatAction, nil
}

func (s *ScriptBasedDecision) getNthActionFromScript(script *gamescript.Script, handNum uint32, handStatus game.HandStatus, n int) gamescript.SeatAction {
	hand := script.GetHand(handNum)
	var scriptActions []gamescript.SeatAction
	switch handStatus {
	case game.HandStatus_PREFLOP:
		scriptActions = hand.Preflop.SeatActions
	case game.HandStatus_FLOP:
		scriptActions = hand.Flop.SeatActions
	case game.HandStatus_TURN:
		scriptActions = hand.Turn.SeatActions
	case game.HandStatus_RIVER:
		scriptActions = hand.River.SeatActions
	default:
		panic(fmt.Sprintf("Invalid hand status [%s] in getNthActionFromScript", handStatus))
	}
	return scriptActions[n]
}

// GetPrevActionToVerify returns previous action of the bot for verification
func (s *ScriptBasedDecision) GetPrevActionToVerify(bot *BotPlayer) (*gamescript.VerifyAction, error) {
	script := bot.config.Script
	handNum := bot.game.handNum
	hand := script.GetHand(handNum)
	if hand.Setup.Auto {
		return nil, nil
	}
	handStatus := bot.game.handStatus
	actionHistory := bot.game.table.actionTracker.GetActions(handStatus)
	prevActionIndex := len(actionHistory) - 1
	seatAction := s.getNthActionFromScript(script, handNum, handStatus, prevActionIndex)
	return seatAction.Verify, nil
}
