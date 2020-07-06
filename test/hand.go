package test

import (
	"fmt"
	"reflect"
	"time"

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

	return nil
}

func (h *Hand) dealHand(t *TestDriver) error {
	// deal new hand
	h.gameScript.testGame.DealNextHand()
	time.Sleep(1 * time.Second)

	// verify current hand player position and cards dealt
	actual := t.Observer.currentHand.GetNewHand()
	verify := h.Setup.Verify
	passed := true
	if verify.Button != 0 && actual.ButtonPos != verify.Button {
		h.gameScript.result.addError(fmt.Errorf("Button position did not match. Expected: %d actual: %d", verify.Button, actual.ButtonPos))
		passed = false
	}

	if verify.SB != 0 && actual.SbPos != verify.SB {
		h.gameScript.result.addError(fmt.Errorf("SB position did not match. Expected: %d actual: %d", verify.SB, actual.SbPos))
		passed = false
	}

	if verify.BB != 0 && actual.BbPos != verify.BB {
		h.gameScript.result.addError(fmt.Errorf("BB position did not match. Expected: %d actual: %d", verify.BB, actual.BbPos))
		passed = false
	}

	if verify.NextActionPos != 0 && actual.NextActionSeat != verify.NextActionPos {
		h.gameScript.result.addError(fmt.Errorf("Next action position did not match. Expected: %d actual: %d", verify.NextActionPos, actual.NextActionSeat))
		passed = false
	}

	// verify hand status
	handState := t.Observer.currentHand.HandStatus.String()
	if len(verify.State) != 0 && verify.State != handState {
		h.gameScript.result.addError(fmt.Errorf("Hand state does not match. Expected: %s actual: %s", verify.State, handState))
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
			h.gameScript.result.addError(fmt.Errorf("Player cards and dealt cards don't match. Player ID: %d, seat pos: %d Expected: %v actual: %v",
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
