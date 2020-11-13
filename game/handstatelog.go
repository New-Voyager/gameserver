package game

import (
	"fmt"
	"strings"

	"voyager.com/server/poker"
)

func (h *HandState) PrintTable(players map[uint64]string) string {
	var b strings.Builder
	b.Grow(32)
	fmt.Fprintf(&b, "Game ID: %d Hand Num: %d, Seats: [", h.GameId, h.HandNum)
	for seatNo, playerID := range h.GetPlayersInSeats() {
		seatNo++
		if playerID == 0 {
			// empty seat
			fmt.Fprintf(&b, " {%d: Empty}, ", seatNo)
		} else {
			player, _ := players[playerID]
			playerState, _ := h.PlayersState[playerID]
			playerCards, _ := h.PlayersCards[uint32(seatNo)]
			cardString := poker.CardsToString(playerCards)
			if uint32(seatNo) == h.ButtonPos {
				fmt.Fprintf(&b, " {%d: %s, %f, %s, BUTTON} ", seatNo, player, playerState.Balance, cardString)
			} else if uint32(seatNo) == h.SmallBlindPos {
				fmt.Fprintf(&b, " {%d: %s, %f, %s, SB} ", seatNo, player, playerState.Balance, cardString)
			} else if uint32(seatNo) == h.BigBlindPos {
				fmt.Fprintf(&b, " {%d: %s, %f, %s, BB} ", seatNo, player, playerState.Balance, cardString)
			} else {
				fmt.Fprintf(&b, " {%d: %s, %f, %s} ", seatNo, player, playerState.Balance, cardString)
			}
		}
	}
	fmt.Fprintf(&b, "]")

	return b.String()
}

func (n *NextSeatAction) PrettyPrint(h *HandState, gameState *GameState, players map[uint64]string) string {
	seatNo := n.SeatNo
	playerState := h.getPlayerFromSeat(seatNo)
	playerID := gameState.GetPlayersInSeats()[seatNo-1]
	player, _ := players[playerID]
	var b strings.Builder
	b.Grow(32)
	fmt.Fprintf(&b, " Next Action: {Seat No: %d, Player: %s, balance: %f}, ", seatNo, player, playerState.Balance)
	fmt.Fprintf(&b, " Available actions: [")
	for _, action := range n.AvailableActions {
		switch action {
		case ACTION_FOLD:
			fmt.Fprintf(&b, "{FOLD},")
		case ACTION_CHECK:
			fmt.Fprintf(&b, "{CHECK},")
		case ACTION_CALL:
			fmt.Fprintf(&b, "{CALL, callAmount: %f},", n.CallAmount)
		case ACTION_RAISE:
			fmt.Fprintf(&b, "{RAISE, raise min: %f, max: %f},", n.MinRaiseAmount, n.MaxRaiseAmount)
		case ACTION_ALLIN:
			fmt.Fprintf(&b, "{ALL_IN, allInAmount: %f},", n.AllInAmount)
		case ACTION_STRADDLE:
			fmt.Fprintf(&b, "{STRADDLE, straddleAmount: %f},", n.StraddleAmount)
		}
	}
	return b.String()
}

func (h *HandState) PrintCurrentActionLog(gameState *GameState, players map[uint64]string) string {
	var log *HandActionLog
	var b strings.Builder
	b.Grow(32)

	switch h.CurrentState {
	case HandStatus_PREFLOP:
		log = h.PreflopActions
		fmt.Fprintf(&b, "PreFlop: \n")
	case HandStatus_FLOP:
		log = h.FlopActions
		fmt.Fprintf(&b, "Flop: \n")
	case HandStatus_TURN:
		log = h.TurnActions
		fmt.Fprintf(&b, "Turn: \n")
	case HandStatus_RIVER:
		log = h.RiverActions
		fmt.Fprintf(&b, "River: \n")
	}
	for _, seatAction := range log.Actions {
		fmt.Fprintf(&b, "%s\n", seatAction.Print(h, gameState, players))
	}
	fmt.Fprintf(&b, "Pot: %f\n", log.Pot)
	return b.String()
}

func (a *HandAction) Print(h *HandState, gameState *GameState, players map[uint64]string) string {
	action := ""
	switch a.Action {
	case ACTION_FOLD:
		action = "FOLD"
	case ACTION_BB:
		action = "BB"
	case ACTION_SB:
		action = "SB"
	case ACTION_STRADDLE:
		action = "STRADDLE"
	case ACTION_CALL:
		action = "CALL"
	case ACTION_RAISE:
		action = "RAISE"
	case ACTION_BET:
		action = "BET"
	}

	seatNo := a.SeatNo
	//playerState := h.getPlayerFromSeat(seatNo)
	playerID := gameState.GetPlayersInSeats()[seatNo-1]
	player, _ := players[playerID]

	if a.Action == ACTION_FOLD {
		return fmt.Sprintf("%s   %s", player, action)
	}
	return fmt.Sprintf("%s   %s   %f", player, action, a.Amount)
}
