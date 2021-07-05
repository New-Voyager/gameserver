package test

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"voyager.com/server/game"
	"voyager.com/server/poker"
)

type TestHand struct {
	hand       *game.Hand
	gameScript *TestGameScript

	noMoreActions bool // set when HandNoMoreAction message is received
}

func NewTestHand(hand *game.Hand, gameScript *TestGameScript) *TestHand {
	return &TestHand{
		hand:       hand,
		gameScript: gameScript,
	}
}
func (h *TestHand) run(t *TestDriver) error {
	// setup hand
	err := h.setup(t)
	if err != nil {
		return err
	}

	err = h.dealHand(t)
	if err != nil {
		return err
	}

	// pre-flop actions
	err = h.preflopActions(t)
	if err != nil {
		return err
	}
	lastMsgItem := h.gameScript.observer.lastHandMessageItem
	result := false
	if lastMsgItem.MessageType == "RESULT" {
		result = true
	}

	if !result {
		// go to flop
		err = h.flopActions(t)
		if err != nil {
			return err
		}
		lastMsgItem := h.gameScript.observer.lastHandMessageItem
		result = false
		if lastMsgItem.MessageType == "RESULT" {
			result = true
		}
	}

	if !result {
		// go to turn
		err = h.turnActions(t)
		if err != nil {
			return err
		}
		lastMsgItem := h.gameScript.observer.lastHandMessageItem
		result = false
		if lastMsgItem.MessageType == "RESULT" {
			result = true
		}
	}

	if !result {
		// go to river
		err = h.riverActions(t)
		if err != nil {
			return err
		}
		lastMsgItem := h.gameScript.observer.lastHandMessageItem
		result = false
		if lastMsgItem.MessageType == "RESULT" {
			result = true
		} else {
			// we didn't get any results after the river
			e := fmt.Errorf("No results found after the river")
			h.gameScript.result.addError(e)
			return e
		}
	}

	// verify results
	if result {
		lastMsgItem := h.gameScript.observer.lastHandMessageItem
		handResult := lastMsgItem.GetHandResult()
		_ = handResult

		err = h.verifyHandResult(t, handResult)
		if err != nil {
			return err
		}
	}

	return nil
}

func (h *TestHand) performBettingRound(t *TestDriver, bettingRound *game.BettingRound) error {
	if !h.noMoreActions {
		if bettingRound.SeatActions != nil {
			bettingRound.Actions = make([]game.TestHandAction, len(bettingRound.SeatActions))
			for i, actionStr := range bettingRound.SeatActions {
				// split the string
				s := strings.Split(strings.Trim(actionStr, " "), ",")
				if len(s) == 3 {
					seatNo, _ := strconv.Atoi(strings.Trim(s[0], " "))
					action := strings.Trim(s[1], " ")
					amount, _ := strconv.ParseFloat(strings.Trim(s[2], " "), 32)
					bettingRound.Actions[i] = game.TestHandAction{
						SeatNo: uint32(seatNo),
						Action: action,
						Amount: float32(amount),
					}
				} else if len(s) == 2 {
					seatNo, _ := strconv.Atoi(strings.Trim(s[0], " "))
					action := strings.Trim(s[1], " ")
					bettingRound.Actions[i] = game.TestHandAction{
						SeatNo: uint32(seatNo),
						Action: action,
					}
				} else {
					e := fmt.Errorf("Invalid action found: %s", bettingRound.SeatActions)
					return e
				}
			}
		}

		for _, action := range bettingRound.Actions {
			player := h.gameScript.playerFromSeat(action.SeatNo)

			if action.VerifyAction != nil {
				// verify available action here
				nextSeatAction := player.yourAction.GetSeatAction()
				actionsStr := convertActions(nextSeatAction.AvailableActions)
				if !IsEqual(actionsStr, action.VerifyAction.Actions) {
					return fmt.Errorf("Actions does not match. Expected: %+v Actual: %+v", action.VerifyAction.Actions, actionsStr)
				}
				callAvailable := false
				for _, action := range nextSeatAction.AvailableActions {
					if action == game.ACTION_CALL {
						callAvailable = true
						break
					}
				}

				if callAvailable && nextSeatAction.CallAmount != action.VerifyAction.CallAmount {
					return fmt.Errorf("Call amount does not match. Expected: %+v Actual: %+v", action.VerifyAction.CallAmount, nextSeatAction.CallAmount)
				}

				if nextSeatAction.AllInAmount != action.VerifyAction.AllInAmount {
					return fmt.Errorf("All in amount does not match. Expected: %+v Actual: %+v", action.VerifyAction.AllInAmount, nextSeatAction.AllInAmount)
				}

				if nextSeatAction.MinRaiseAmount != action.VerifyAction.MinRaiseAmount {
					return fmt.Errorf("Min raise amount does not match. Expected: %+v Actual: %+v", action.VerifyAction.MinRaiseAmount, nextSeatAction.MinRaiseAmount)
				}

				if nextSeatAction.MaxRaiseAmount != action.VerifyAction.MaxRaiseAmount {
					return fmt.Errorf("Max raise amount does not match. Expected: %+v Actual: %+v", action.VerifyAction.MaxRaiseAmount, nextSeatAction.MaxRaiseAmount)
				}

				// verify bet options
				if action.VerifyAction.BetAmounts != nil {
					if len(action.VerifyAction.BetAmounts) != len(nextSeatAction.BetOptions) {
						return fmt.Errorf("Bet options do not match. Expected: %+v Actual: %+v", action.VerifyAction.BetAmounts, nextSeatAction.BetOptions)
					}
					for i, _ := range nextSeatAction.BetOptions {
						if action.VerifyAction.BetAmounts[i].Text != nextSeatAction.BetOptions[i].Text {
							return fmt.Errorf("Bet options do not match. Expected: %+v Actual: %+v", action.VerifyAction.BetAmounts, nextSeatAction.BetOptions)
						}
						if action.VerifyAction.BetAmounts[i].Amount != nextSeatAction.BetOptions[i].Amount {
							return fmt.Errorf("Bet options do not match. Expected: %+v Actual: %+v", action.VerifyAction.BetAmounts, nextSeatAction.BetOptions)
						}
					}
				}
			}
			// send handmessage
			actionType := game.ACTION(game.ACTION_value[action.Action])
			handAction := game.HandAction{SeatNo: action.SeatNo, Action: actionType, Amount: action.Amount}
			message := game.HandMessage{
				ClubId:  h.gameScript.testGame.clubID,
				GameId:  h.gameScript.testGame.gameID,
				HandNum: h.hand.Num,
				SeatNo:  action.SeatNo,
				Messages: []*game.HandMessageItem{
					{
						MessageType: game.HandPlayerActed,
						Content:     &game.HandMessageItem_PlayerActed{PlayerActed: &handAction},
					},
				},
			}
			player.player.HandProtoMessageFromAdapter(&message)
			h.gameScript.waitForObserver()
		}
	}

	h.waitForRunItTwicePrompt()
	h.waitForRunItTwicePrompt()

	lastHandMsgItem := h.getObserverLastHandMessageItem()
	// if last hand message was no more downs, there will be no more actions from the players
	if lastHandMsgItem.MessageType == game.HandNoMoreActions {
		h.noMoreActions = true
		// wait for betting round message (flop, turn, river, showdown)
		h.gameScript.waitForObserver()
	} else if lastHandMsgItem.MessageType == game.HandRunItTwice {
		// verify run it twice
		fmt.Printf("Run it twice")
		// wait for the result
		h.gameScript.waitForObserver()
	} else if lastHandMsgItem.MessageType != game.HandResultMessage {
		// wait for betting round message (flop, turn, river, showdown)
		h.gameScript.waitForObserver()
	}

	// verify next action is correct
	verify := bettingRound.Verify
	err := h.verifyBettingRound(t, &verify)
	if err != nil {
		return err
	}
	return nil
}

func (h *TestHand) waitForRunItTwicePrompt() {
	lastHandMsgItem := h.getObserverLastHandMessageItem()
	if lastHandMsgItem.MessageType == game.HandPlayerAction {
		seatAction := lastHandMsgItem.GetSeatAction()
		if seatAction.AvailableActions != nil && len(seatAction.AvailableActions) >= 1 {
			if seatAction.AvailableActions[0] == game.ACTION_RUN_IT_TWICE_PROMPT {
				// confirm the player wants to run it twice
				actionType := game.ACTION_RUN_IT_TWICE_YES
				player := h.gameScript.playerFromSeat(seatAction.SeatNo)
				if !player.player.RunItTwicePromptResponse {
					actionType = game.ACTION_RUN_IT_TWICE_NO
				}

				handAction := game.HandAction{SeatNo: seatAction.SeatNo, Action: actionType}
				message := game.HandMessage{
					ClubId:  h.gameScript.testGame.clubID,
					GameId:  h.gameScript.testGame.gameID,
					HandNum: h.hand.Num,
					SeatNo:  seatAction.SeatNo,
					Messages: []*game.HandMessageItem{
						{
							MessageType: game.HandPlayerActed,
							Content:     &game.HandMessageItem_PlayerActed{PlayerActed: &handAction},
						},
					},
				}
				player.player.HandProtoMessageFromAdapter(&message)
				h.gameScript.waitForObserver()
			}
		}
	}
}

func convertActions(actions []game.ACTION) []string {
	actionsStr := make([]string, len(actions))
	for i, action := range actions {
		actionsStr[i] = action.String()
	}
	return actionsStr
}

func IsEqual(a1 []string, a2 []string) bool {
	sort.Strings(a1)
	sort.Strings(a2)
	if len(a1) == len(a2) {
		for i, v := range a1 {
			if v != a2[i] {
				return false
			}
		}
	} else {
		return false
	}
	return true
}

func (h *TestHand) preflopActions(t *TestDriver) error {
	e := h.performBettingRound(t, &h.hand.PreflopAction)
	return e
}

func (h *TestHand) flopActions(t *TestDriver) error {
	e := h.performBettingRound(t, &h.hand.FlopAction)
	return e
}

func (h *TestHand) turnActions(t *TestDriver) error {
	e := h.performBettingRound(t, &h.hand.TurnAction)
	return e
}

func (h *TestHand) riverActions(t *TestDriver) error {
	e := h.performBettingRound(t, &h.hand.RiverAction)
	return e
}

func (h *TestHand) dealHand(t *TestDriver) error {
	// deal new hand
	h.gameScript.testGame.Observer().dealNextHand()

	// wait for confirmation from the observer
	// new hand
	h.gameScript.waitForObserver()

	for _, player := range h.gameScript.testGame.players {
		_ = player
		// wait for dealing to complete for each player
		h.gameScript.waitForObserver()
	}

	// verify current hand player position and cards dealt
	actual := h.gameScript.observer.currentHand.GetNewHand()
	verify := h.hand.Setup.Verify
	passed := true
	if verify.Button != 0 && actual.ButtonPos != verify.Button {
		h.addError(fmt.Errorf("Button position did not match. Expected: %d actual: %d", verify.Button, actual.ButtonPos))
		passed = false
	}

	if verify.SB != 0 && actual.SbPos != verify.SB {
		h.addError(fmt.Errorf("SB position did not match. Expected: %d actual: %d", verify.SB, actual.SbPos))
		passed = false
	}

	if verify.BB != 0 && actual.BbPos != verify.BB {
		h.addError(fmt.Errorf("BB position did not match. Expected: %d actual: %d", verify.BB, actual.BbPos))
		passed = false
	}

	if verify.NextActionPos != 0 && actual.NextActionSeat != verify.NextActionPos {
		h.addError(fmt.Errorf("Next action position did not match. Expected: %d actual: %d", verify.NextActionPos, actual.NextActionSeat))
		passed = false
	}

	// verify hand status
	handState := h.gameScript.observer.lastHandMessage.HandStatus.String()
	if len(verify.State) != 0 && verify.State != handState {
		h.addError(fmt.Errorf("Hand state does not match. Expected: %s actual: %s", verify.State, handState))
		passed = false
	}

	if !passed {
		return fmt.Errorf("Failed to verify at hand setup step")
	}

	// verify players cards
	for _, seat := range verify.DealtCards {
		player := h.gameScript.playerFromSeat(seat.SeatNo)
		playerCards := poker.ByteCardsToStringArray(player.cards)
		dealtCards := seat.Cards
		if !reflect.DeepEqual(playerCards, dealtCards) {
			h.addError(fmt.Errorf("Player cards and dealt cards don't match. Player ID: %d, seat pos: %d Expected: %v actual: %v",
				player.player.PlayerID, player.seatNo, dealtCards, playerCards))
			passed = false
		}
	}

	return nil
}

func (h *TestHand) setup(t *TestDriver) error {
	playerCards := make([]poker.CardsInAscii, 0)
	for _, cards := range h.hand.Setup.SeatCards {
		playerCards = append(playerCards, cards.Cards)
	}
	var deck *poker.Deck
	if !h.hand.Setup.AutoDeal {

		if h.hand.Setup.Board != nil {
			deck = poker.DeckFromBoard(playerCards, h.hand.Setup.Board, h.hand.Setup.Board2, false)
		} else {
			// arrange deck
			deck = poker.DeckFromScript(playerCards, h.hand.Setup.Flop, poker.NewCard(h.hand.Setup.Turn), poker.NewCard(h.hand.Setup.River), false)
		}
	}

	// setup hand
	h.gameScript.testGame.Observer().setupNextHand(deck, h.hand.Setup.AutoDeal, h.hand.Setup.ButtonPos, h.hand.Num)
	return nil
}

func (h *TestHand) verifyHandResult(t *TestDriver, handResult *game.HandResult) error {
	passed := true

	if h.hand.Result.Winners != nil {
		pot := 0
		potWinner := handResult.HandLog.PotWinners[uint32(pot)]
		hiWinners := potWinner.GetHiWinners()
		if len(hiWinners) != len(h.hand.Result.Winners) {
			passed = false
		}
		if passed {
			for i, expectedWinner := range h.hand.Result.Winners {
				handWinner := hiWinners[i]
				if handWinner.SeatNo != expectedWinner.Seat {
					h.addError(fmt.Errorf("Winner seat no didn't match. Expected %d, actual: %d",
						expectedWinner.Seat, handWinner.SeatNo))
					passed = false
				}

				if handWinner.Amount != expectedWinner.Receive {
					h.addError(fmt.Errorf("Winner winning didn't match. Expected %f, actual: %f",
						expectedWinner.Receive, handWinner.Amount))
					passed = false
				}
			}
		}
	}

	if h.hand.Result.LoWinners != nil {
		pot := 0
		potWinner := handResult.HandLog.PotWinners[uint32(pot)]
		loWinners := potWinner.GetLowWinners()

		if len(loWinners) != len(h.hand.Result.LoWinners) {
			passed = false
		}

		if passed {
			for i, expectedWinner := range h.hand.Result.LoWinners {
				handWinner := loWinners[i]
				if handWinner.SeatNo != expectedWinner.Seat {
					h.addError(fmt.Errorf("Winner seat no didn't match. Expected %d, actual: %d",
						expectedWinner.Seat, handWinner.SeatNo))
					passed = false
				}

				if handWinner.Amount != expectedWinner.Receive {
					h.addError(fmt.Errorf("Winner winning didn't match. Expected %f, actual: %f",
						expectedWinner.Receive, handWinner.Amount))
					passed = false
				}
			}
		}
	}

	if h.hand.Result.ActionEndedAt != "" {
		actualActionEndedAt := game.HandStatus_name[int32(handResult.HandLog.WonAt)]
		if h.hand.Result.ActionEndedAt != actualActionEndedAt {
			h.addError(fmt.Errorf("Action won at is not matching. Expected %s, actual: %s",
				h.hand.Result.ActionEndedAt, actualActionEndedAt))
			passed = false
		}
	}

	// now verify players stack
	expectedStacks := h.hand.Result.Stacks
	for _, expectedStack := range expectedStacks {
		for seatNo, player := range handResult.Players {

			if seatNo == expectedStack.Seat {
				if player.Balance.After != expectedStack.Stack {
					h.addError(fmt.Errorf("Player %d seatNo: %d is not matching. Expected %f, actual: %f", player.Balance.After, seatNo,
						expectedStack.Stack, player.Balance.After))
					passed = false
				}
			}
		}
	}

	if !passed {
		return fmt.Errorf("Failed when verifying the hand result")
	}
	return nil
}

func (h *TestHand) addError(e error) {
	h.gameScript.result.addError(e)
}

func (h *TestHand) getObserverLastHandMessage() *game.HandMessage {
	return h.gameScript.observerLastHandMessage
}

func (h *TestHand) getObserverLastHandMessageItem() *game.HandMessageItem {
	return h.gameScript.observerLastHandMessageItem
}

func (h *TestHand) verifyBettingRound(t *TestDriver, verify *game.VerifyBettingRound) error {
	lastHandMessage := h.getObserverLastHandMessage()
	// lastHandMsgItem := h.getObserverLastHandMessageItem()
	if verify.State != "" {
		if verify.State == "FLOP" {
			// make sure the hand state is set correctly
			if lastHandMessage.HandStatus != game.HandStatus_FLOP {
				h.addError(fmt.Errorf("Expected hand status as FLOP Actual: %s", game.HandStatus_name[int32(lastHandMessage.HandStatus)]))
				return fmt.Errorf("Expected hand state as FLOP")
			}

			// verify the board has the correct cards
			if verify.Board != nil {
				flopMessage := h.gameScript.observer.flop
				boardCardsFromGame := poker.ByteCardsToStringArray(flopMessage.Board)
				expectedCards := verify.Board
				if !reflect.DeepEqual(boardCardsFromGame, expectedCards) {
					e := fmt.Errorf("Flopped cards did not match with expected cards. Expected: %s actual: %s",
						poker.CardsToString(expectedCards), poker.CardsToString(flopMessage.Board))
					h.addError(e)
					return e
				}
			}
		} else if verify.State == "TURN" {
			// make sure the hand state is set correctly
			if lastHandMessage.HandStatus != game.HandStatus_TURN {
				h.addError(fmt.Errorf("Expected hand status as TURN Actual: %s", game.HandStatus_name[int32(lastHandMessage.HandStatus)]))
				return fmt.Errorf("Expected hand state as TURN")
			}

			// verify the board has the correct cards
			if verify.Board != nil {
				turnMessage := h.gameScript.observer.turn
				boardCardsFromGame := poker.ByteCardsToStringArray(turnMessage.Board)
				expectedCards := verify.Board
				if !reflect.DeepEqual(boardCardsFromGame, expectedCards) {
					e := fmt.Errorf("Flopped cards did not match with expected cards. Expected: %s actual: %s",
						poker.CardsToString(expectedCards), poker.CardsToString(turnMessage.Board))
					h.addError(e)
					return e
				}
			}
		} else if verify.State == "RESULT" {
			// if lastHandMsgItem.MessageType != "RESULT" {
			if lastHandMessage.HandStatus != game.HandStatus_RESULT {
				h.addError(fmt.Errorf("Expected result after the betting round. Actual: %s", game.HandStatus_name[int32(lastHandMessage.HandStatus)]))
				return fmt.Errorf("Failed at preflop verification step")
			}
		}
	}

	if verify.Pots != nil {

		// get pot information from the observer
		gamePots := h.gameScript.observer.actionChange.GetActionChange().SeatsPots

		switch verify.State {
		case "FLOP":
			gamePots = h.gameScript.observer.flop.SeatsPots
		case "TURN":
			gamePots = h.gameScript.observer.turn.SeatsPots
		case "RIVER":
			gamePots = h.gameScript.observer.river.SeatsPots
		case "SHOWDOWN":
			gamePots = h.gameScript.observer.showdown.GetSeatsPots()
		}

		if h.gameScript.observer.noMoreActions != nil {
			gamePots = h.gameScript.observer.noMoreActions.GetNoMoreActions().Pots
		}

		if h.gameScript.observer.runItTwice != nil {
			gamePots = h.gameScript.observer.runItTwice.GetRunItTwice().SeatsPots
		}

		if len(verify.Pots) != len(gamePots) {
			e := fmt.Errorf("Pot count does not match. Expected: %d actual: %d", len(verify.Pots), len(gamePots))
			h.gameScript.result.addError(e)
			return e
		}

		for i, expectedPot := range verify.Pots {
			actualPot := gamePots[i]
			if expectedPot.Pot != actualPot.Pot {
				e := fmt.Errorf("Pot [%d] amount does not match. Expected: %f actual: %f",
					i, expectedPot.Pot, actualPot.Pot)
				h.gameScript.result.addError(e)
				return e
			}

			if expectedPot.SeatsInPot != nil {
				// verify the seats are in the pot
				for _, seatNo := range expectedPot.SeatsInPot {
					found := false
					for _, actualSeat := range actualPot.Seats {
						if actualSeat == seatNo {
							found = true
							break
						}
					}
					if !found {
						e := fmt.Errorf("Pot [%d] seat %d is not in the pot", i, seatNo)
						h.gameScript.result.addError(e)
					}
				}
			}
		}
	}

	if verify.RunItTwice {
		if h.gameScript.observer.runItTwice == nil {
			e := fmt.Errorf("Expected to run it twice")
			h.gameScript.result.addError(e)
		}
	}

	if verify.Stacks != nil {
		var stacks map[uint32]float32
		switch verify.State {
		case "FLOP":
			stacks = h.gameScript.observer.flop.PlayerBalance
		case "TURN":
			stacks = h.gameScript.observer.turn.PlayerBalance
		case "RIVER":
			stacks = h.gameScript.observer.river.PlayerBalance
		case "SHOWDOWN":
			stacks = h.gameScript.observer.showdown.PlayerBalance
		}

		for _, stack := range verify.Stacks {
			if playerStack, ok := stacks[stack.Seat]; ok {
				if playerStack != stack.Stack {
					e := fmt.Errorf("Player at seatNo [%d] stack did not match. Expected: %f Actual %f found at state: %s",
						stack.Seat, stack.Stack, playerStack, verify.State)
					h.gameScript.result.addError(e)
				}
			} else {
				if stack.Stack != 0 {
					e := fmt.Errorf("Player at seatNo [%d] stack is not found at state: %s", stack.Seat, verify.State)
					h.gameScript.result.addError(e)
				}
			}
		}
	}

	return nil
}
