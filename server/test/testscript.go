package test

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	"voyager.com/server/game"
)

type TestGameScript struct {
	gameScript             *game.GameScript
	testGame               *TestGame
	filename               string
	result                 *ScriptTestResult
	observer               *TestPlayer
	observerLastHandMesage *game.HandMessage
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

func (g *TestGameScript) waitForObserver() *game.HandMessage {
	messageBytes := <-g.testGame.observerCh
	var handMessage game.HandMessage
	proto.Unmarshal(messageBytes, &handMessage)
	g.observerLastHandMesage = &handMessage
	return &handMessage
}

// configures the table with the configuration
func (g *TestGameScript) configure(t *TestDriver) error {
	gameType := game.GameType(game.GameType_value[g.gameScript.GameConfig.GameTypeStr])
	var err error
	g.testGame, g.observer, err = NewTestGame(g, 1, gameType, g.gameScript.GameConfig.Title, g.gameScript.GameConfig.AutoStart, g.gameScript.Players)
	if err != nil {
		return err
	}
	g.testGame.Start(g.gameScript.AssignSeat.Seats)
	// get current game status
	//gameManager.GetTableState(g.testGame.clubID, g.testGame.gameNum, g.observer.player.PlayerID)
	// g.observer.getTableState()
	// g.waitForObserver()

	// e := g.verifyTableResult(t, g.gameScript.AssignSeat.Verify.Table.Players, "take-seat")
	// if e != nil {
	// 	return e
	// }

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
	for _, hand := range g.gameScript.Hands {
		testHand := NewTestHand(&hand, g)
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
