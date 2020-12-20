package test

import (
	"fmt"
	"reflect"
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

			// send handmessage
			message := game.HandMessage{
				ClubId:      h.gameScript.testGame.clubID,
				GameId:      h.gameScript.testGame.gameID,
				HandNum:     h.hand.Num,
				SeatNo:      action.SeatNo,
				MessageType: game.HandPlayerActed,
			}
			actionType := game.ACTION(game.ACTION_value[action.Action])
			handAction := game.HandAction{SeatNo: action.SeatNo, Action: actionType, Amount: action.Amount}
			message.HandMessage = &game.HandMessage_PlayerActed{PlayerActed: &handAction}
			player.player.HandProtoMessageFromAdapter(&message)

			h.gameScript.waitForObserver()
		}
	}

	lastHandMessage := h.getObserverLastHandMessage()
	// if last hand message was no more downs, there will be no more actions from the players
	if lastHandMessage.MessageType == game.HandNoMoreActions {
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

	// next action
	h.gameScript.waitForObserver()

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

func (h *TestHand) setup(t *TestDriver) error {
	playerCards := make([]poker.CardsInAscii, 0)
	for _, cards := range h.hand.Setup.SeatCards {
		playerCards = append(playerCards, cards.Cards)
	}
	// arrange deck
	deck := poker.DeckFromScript(playerCards, h.hand.Setup.Flop, poker.NewCard(h.hand.Setup.Turn), poker.NewCard(h.hand.Setup.River))

	// setup hand
	h.gameScript.testGame.Observer().setupNextHand(deck.GetBytes(), h.hand.Setup.ButtonPos)
	return nil
}

func (h *TestHand) verifyHandResult(t *TestDriver, handResult *game.HandResult) error {
	passed := true
	for i, expectedWinner := range h.hand.Result.Winners {
		potWinner := handResult.HandLog.PotWinners[uint32(i)]
		winners := potWinner.GetHiWinners()
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
	return h.gameScript.observerLastHandMesage
}

func (h *TestHand) verifyBettingRound(t *TestDriver, verify *game.VerifyBettingRound) error {
	lastHandMessage := h.getObserverLastHandMessage()
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
			if lastHandMessage.MessageType != "RESULT" {
				h.addError(fmt.Errorf("Expected result after preflop actions. Actual message: %s", lastHandMessage.MessageType))
				return fmt.Errorf("Failed at preflop verification step")
			}
		}
	}

	if verify.Pots != nil {

		// get pot information from the observer
		gamePots := h.gameScript.observer.actionChange.GetActionChange().SeatsPots
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
