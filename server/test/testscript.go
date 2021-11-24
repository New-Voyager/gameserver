package test

import (
	"fmt"
	"time"

	"voyager.com/server/game"
)

type TestGameScript struct {
	gameScript                  *game.GameScript
	testGame                    *TestGame
	filename                    string
	result                      *ScriptTestResult
	observer                    *TestPlayer
	observerLastHandMessage     *game.HandMessage
	observerLastHandMessageItem *game.HandMessageItem
}

func (g *TestGameScript) run(t *TestDriver) error {
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

func (g *TestGameScript) waitForObserver() observerChItem {
	chItem := <-g.testGame.observerCh
	if chItem.handMessage != nil {
		g.observerLastHandMessage = chItem.handMessage
	}
	if chItem.handMsgItem != nil {
		g.observerLastHandMessageItem = chItem.handMsgItem
	}
	return chItem
}

// configures the table with the configuration
func (g *TestGameScript) configure(t *TestDriver) error {
	gameType := game.GameType(game.GameType_value[g.gameScript.GameConfig.GameTypeStr])
	chipUnit := ToGameChipUnit(g.gameScript.GameConfig.ChipUnit)
	var err error
	g.testGame, g.observer, err = NewTestGame(g, 1, gameType, g.gameScript.GameConfig.Title, g.gameScript.GameConfig.AutoStart, chipUnit, g.gameScript.Players)
	if err != nil {
		return err
	}
	g.testGame.PopulateSeats(g.gameScript.AssignSeat.Seats, chipUnit)

	return nil
}

func (g *TestGameScript) verifyTableResult(t *TestDriver, expectedPlayers []game.PlayerAtTable, where string) error {
	if expectedPlayers == nil {
		return nil
	}

	if expectedPlayers != nil {
		explectedPlayers := expectedPlayers
		// validate the player stack here to ensure sit-in command worked
		expectedPlayersInTable := len(explectedPlayers)
		actualPlayersInTable := len(g.observer.lastTableState.PlayersState)
		if expectedPlayersInTable != actualPlayersInTable {
			e := fmt.Errorf("[%s section] Expected number of players (%d) did not match the actual players (%d)",
				where, expectedPlayersInTable, actualPlayersInTable)
			g.result.addError(e)
			return e
		}
	}
	actualPlayers := g.observer.lastTableState.PlayersState

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

func (g *TestGameScript) dealHands(t *TestDriver) error {
	for i, hand := range g.gameScript.Hands {
		testHand := NewTestHand(&hand, g)
		if i > 0 {
			// Wait for the server to finish processing the previous hand
			time.Sleep(1000 * time.Millisecond)
		}
		err := testHand.run(t)
		if err != nil {
			return err
		}
	}

	return nil
}

func (g *TestGameScript) playerFromSeat(seatNo uint32) *TestPlayer {
	for _, player := range g.testGame.players {
		if player.player.SeatNo == seatNo {
			return player
		}
	}
	return nil
}
