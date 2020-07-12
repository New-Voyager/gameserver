package game

import (
	"fmt"
	"reflect"

	"voyager.com/server/poker"
)

func (h *Hand) run(t *TestDriver) error {
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
	lastHandMessage := h.gameScript.observer.lastHandMessage
	result := false
	if lastHandMessage.MessageType == "RESULT" {
		result = true
	}

	if !result {
		// go to flop
		err = h.flopActions(t)
		if err != nil {
			return err
		}
		lastHandMessage := h.gameScript.observer.lastHandMessage
		result = false
		if lastHandMessage.MessageType == "RESULT" {
			result = true
		}
	}

	if !result {
		// go to turn
		err = h.turnActions(t)
		if err != nil {
			return err
		}
		lastHandMessage := h.gameScript.observer.lastHandMessage
		result = false
		if lastHandMessage.MessageType == "RESULT" {
			result = true
		}
	}

	if !result {
		// go to river
		err = h.riverActions(t)
		if err != nil {
			return err
		}
		lastHandMessage := h.gameScript.observer.lastHandMessage
		result = false
		if lastHandMessage.MessageType == "RESULT" {
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
		lastHandMessage = h.gameScript.observer.lastHandMessage
		handResult := lastHandMessage.GetHandResult()
		_ = handResult

		err = h.verifyHandResult(t, handResult)
		if err != nil {
			return err
		}
	}

	return nil
}

func (h *Hand) performBettingRound(t *TestDriver, bettingRound *BettingRound) error {
	if !h.noMoreActions {
		for _, action := range bettingRound.Actions {
			player := h.gameScript.playerFromSeat(action.SeatNo)

			// send handmessage
			message := HandMessage{
				ClubId:      h.gameScript.testGame.clubID,
				GameNum:     h.gameScript.testGame.gameNum,
				HandNum:     h.Num,
				MessageType: HandPlayerActed,
			}
			actionType := ACTION(ACTION_value[action.Action])
			handAction := HandAction{SeatNo: action.SeatNo, Action: actionType, Amount: action.Amount}
			message.HandMessage = &HandMessage_PlayerActed{PlayerActed: &handAction}
			player.player.HandProtoMessageFromAdapter(&message)

			h.gameScript.waitForObserver()
		}
	}

	lastHandMessage := h.getObserverLastHandMessage()
	// if last hand message was no more downs, there will be no more actions from the players
	if lastHandMessage.MessageType == HandNoMoreActions {
		h.noMoreActions = true
		// wait for betting round message (flop, turn, river, showdown)
		h.gameScript.waitForObserver()
	} else if lastHandMessage.MessageType != "RESULT" {
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

func (h *Hand) preflopActions(t *TestDriver) error {
	e := h.performBettingRound(t, &h.PreflopAction)
	return e
}

func (h *Hand) flopActions(t *TestDriver) error {
	e := h.performBettingRound(t, &h.FlopAction)
	return e
}

func (h *Hand) turnActions(t *TestDriver) error {
	e := h.performBettingRound(t, &h.TurnAction)
	return e
}

func (h *Hand) riverActions(t *TestDriver) error {
	e := h.performBettingRound(t, &h.RiverAction)
	return e
}

func (h *Hand) dealHand(t *TestDriver) error {
	// deal new hand
	h.gameScript.testGame.DealNextHand()

	// wait for confirmation from the observer
	// new hand
	h.gameScript.waitForObserver()

	// next action
	h.gameScript.waitForObserver()

	// verify current hand player position and cards dealt
	actual := h.gameScript.observer.currentHand.GetNewHand()
	verify := h.Setup.Verify
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
	handState := h.gameScript.observer.currentHand.HandStatus.String()
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

func (h *Hand) setup(t *TestDriver) error {
	playerCards := make([]poker.CardsInAscii, 0)
	for _, cards := range h.Setup.SeatCards {
		playerCards = append(playerCards, cards.Cards)
	}
	// arrange deck
	deck := poker.DeckFromScript(playerCards, h.Setup.Flop, poker.NewCard(h.Setup.Turn), poker.NewCard(h.Setup.River))

	// setup hand
	h.gameScript.testGame.SetupNextHand(deck.GetBytes(), h.Setup.ButtonPos)
	return nil
}

func (h *Hand) verifyHandResult(t *TestDriver, handResult *HandResult) error {
	passed := true
	for i, expectedWinner := range h.Result.Winners {
		potWinner := handResult.PotWinners[uint32(i)]
		winners := potWinner.GetHandWinner()
		if len(winners) != 1 {
			passed = false
		}
		handWinner := winners[0]
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

	if h.Result.ActionEndedAt != "" {
		actualActionEndedAt := HandStatus_name[int32(handResult.WonAt)]
		if h.Result.ActionEndedAt != actualActionEndedAt {
			h.addError(fmt.Errorf("Action won at is not matching. Expected %s, actual: %s",
				h.Result.ActionEndedAt, actualActionEndedAt))
			passed = false
		}
	}

	// now verify players stack
	expectedStacks := h.Result.Stacks
	for _, expectedStack := range expectedStacks {
		for _, playerBalance := range handResult.BalanceAfterHand {
			if playerBalance.SeatNo == expectedStack.Seat {
				if playerBalance.Balance != expectedStack.Stack {
					h.addError(fmt.Errorf("Player %d seatNo: %d is not matching. Expected %f, actual: %f", playerBalance.PlayerId, playerBalance.SeatNo,
						expectedStack.Stack, playerBalance.Balance))
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

func (h *Hand) addError(e error) {
	h.gameScript.result.addError(e)
}

func (h *Hand) getObserverLastHandMessage() *HandMessage {
	return h.gameScript.observerLastHandMesage
}

func (h *Hand) verifyBettingRound(t *TestDriver, verify *VerifyBettingRound) error {
	lastHandMessage := h.getObserverLastHandMessage()
	if verify.State != "" {
		if verify.State == "FLOP" {
			// make sure the hand state is set correctly
			if lastHandMessage.HandStatus != HandStatus_FLOP {
				h.addError(fmt.Errorf("Expected hand status as FLOP Actual: %s", HandStatus_name[int32(lastHandMessage.HandStatus)]))
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
		} else if verify.State == "RESULT" {
			if lastHandMessage.MessageType != "RESULT" {
				h.addError(fmt.Errorf("Expected result after preflop actions. Actual message: %s", lastHandMessage.MessageType))
				return fmt.Errorf("Failed at preflop verification step")
			}
		}
	}

	if verify.Pots != nil {

		// get pot information from the observer
		gamePots := h.gameScript.observer.actionChange.GetActionChange().Pots
		if h.gameScript.observer.noMoreActions != nil {
			gamePots = h.gameScript.observer.noMoreActions.GetNoMoreActions().Pots
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

	return nil
}
