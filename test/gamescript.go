package test

import (
	"fmt"
	"time"

	"voyager.com/server/poker"

	"voyager.com/server/game"
)

func (g *GameScript) run(t *TestDriver) error {
	err := g.configure(t)
	if err != nil {
		return err
	}

	err = g.dealHands(t)
	if err != nil {
		return err
	}

	return nil
}

// configures the table with the configuration
func (g *GameScript) configure(t *TestDriver) error {
	gameType := game.GameType(game.GameType_value[g.GameConfig.GameType])
	g.testGame = NewTestGame(t, 1, gameType, g.GameConfig.Title, g.GameConfig.AutoStart, g.Players)
	g.testGame.Start(g.AssignSeat.Seats)
	waitTime := 100 * time.Millisecond
	if g.AssignSeat.Wait != 0 {
		waitTime = time.Duration(g.AssignSeat.Wait) * time.Second
	}
	// get current game status
	gameManager.GetTableState(g.testGame.clubID, g.testGame.gameNum, t.Observer.player.PlayerID)
	time.Sleep(waitTime)

	e := g.verifyTableResult(t, g.AssignSeat.Verify.Table.Players, "take-seat")
	if e != nil {
		return e
	}
	return nil
}

func (g *GameScript) verifyTableResult(t *TestDriver, expectedPlayers []PlayerAtTable, where string) error {
	if expectedPlayers == nil {
		return nil
	}

	if expectedPlayers != nil {
		explectedPlayers := expectedPlayers
		// validate the player stack here to ensure sit-in command worked
		expectedPlayersInTable := len(explectedPlayers)
		actualPlayersInTable := len(t.Observer.lastTableState.PlayersState)
		if expectedPlayersInTable != actualPlayersInTable {
			e := fmt.Errorf("[%s section] Expected number of players (%d) did not match the actual players (%d)",
				where, expectedPlayersInTable, actualPlayersInTable)
			g.result.addError(e)
			return e
		}
	}
	actualPlayers := t.Observer.lastTableState.PlayersState

	// verify player in each seat and their stack
	for i, expected := range expectedPlayers {
		actual := actualPlayers[i]
		if actual.PlayerId != expected.PlayerID {
			e := fmt.Errorf("[%s section] Expected player (%v) actual player (%v)",
				where, expected, actual)
			g.result.addError(e)
			return e
		}

		if actual.GetCurrentBalance() != expected.Stack {
			e := fmt.Errorf("[%s section] Player %d stack does not match. Expected: %f, actual: %f",
				where, actual.PlayerId, expected.Stack, actual.CurrentBalance)
			g.result.addError(e)
			return e
		}
	}

	return nil
}

func (g *GameScript) dealHands(t *TestDriver) error {
	for _, hand := range g.Hands {
		hand.gameScript = g
		err := hand.run(t)
		if err != nil {
			return err
		}
	}

	return nil
}

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
		h.gameScript.result.addError(fmt.Errorf("Hand state does not match. Expected: %d actual: %d", verify.State, handState))
		passed = false
	}

	if !passed {
		return fmt.Errorf("Failed to verify at hand setup step")
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
