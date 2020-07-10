package test

import (
	"fmt"
	"reflect"

	"voyager.com/server/game"

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

func (h *Hand) preflopActions(t *TestDriver) error {
	for _, action := range h.PreflopAction.Actions {
		player := h.gameScript.playerFromSeat(action.SeatNo)

		// send handmessage
		message := game.HandMessage{
			ClubId:      h.gameScript.testGame.clubID,
			GameNum:     h.gameScript.testGame.gameNum,
			HandNum:     h.Num,
			MessageType: game.HandPlayerActed,
		}
		actionType := game.ACTION(game.ACTION_value[action.Action])
		handAction := game.HandAction{SeatNo: action.SeatNo, Action: actionType, Amount: action.Amount}
		message.HandMessage = &game.HandMessage_PlayerActed{PlayerActed: &handAction}
		player.player.HandProtoMessageFromAdapter(&message)

		h.gameScript.waitForObserver()

	}
	lastHandMessage := h.getObserverLastHandMessage()
	if lastHandMessage.MessageType != "RESULT" {
		// wait for flop message
		h.gameScript.waitForObserver()
	}

	// verify next action is correct
	verify := h.PreflopAction.Verify
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
				boardCardsFromGame := poker.ByteCardsToStringArray(flopMessage.Cards)
				expectedCards := verify.Board
				if !reflect.DeepEqual(boardCardsFromGame, expectedCards) {
					e := fmt.Errorf("Flopped cards did not match with expected cards. Expected: %s actual: %s",
						poker.CardsToString(expectedCards), poker.CardsToString(flopMessage.Cards))
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
	return nil
}

func (h *Hand) flopActions(t *TestDriver) error {
	for _, action := range h.FlopAction.Actions {
		player := h.gameScript.playerFromSeat(action.SeatNo)

		// send handmessage
		message := game.HandMessage{
			ClubId:      h.gameScript.testGame.clubID,
			GameNum:     h.gameScript.testGame.gameNum,
			HandNum:     h.Num,
			MessageType: game.HandPlayerActed,
		}
		actionType := game.ACTION(game.ACTION_value[action.Action])
		handAction := game.HandAction{SeatNo: action.SeatNo, Action: actionType, Amount: action.Amount}
		message.HandMessage = &game.HandMessage_PlayerActed{PlayerActed: &handAction}
		player.player.HandProtoMessageFromAdapter(&message)

		h.gameScript.waitForObserver()

	}
	lastHandMessage := h.getObserverLastHandMessage()
	if lastHandMessage.MessageType != "RESULT" {
		// wait for turn message
		h.gameScript.waitForObserver()
	}

	// verify next action is correct
	if h.PreflopAction.Verify.State != "" {
		if h.PreflopAction.Verify.State == "RESULT" {
			if lastHandMessage.MessageType != "RESULT" {
				h.addError(fmt.Errorf("Expected result after preflop actions. Actual message: %s", lastHandMessage.MessageType))
				return fmt.Errorf("Failed at preflop verification step")
			}
		}
	}
	return nil
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
	fmt.Printf(deck.PrettyPrint())
	h.gameScript.testGame.SetupNextHand(deck.GetBytes(), h.Setup.ButtonPos)
	return nil
}

func (h *Hand) verifyHandResult(t *TestDriver, handResult *game.HandResult) error {
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
		actualActionEndedAt := game.HandStatus_name[int32(handResult.WonAt)]
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

func (h *Hand) getObserverLastHandMessage() *game.HandMessage {
	return h.gameScript.observerLastHandMesage
}
