package game

import (
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
	"voyager.com/server/poker"
)

var handLogger = log.With().Str("logger_name", "game::hand").Logger()

func LoadHandState(handStatePersist PersistHandState, clubID uint32, gameNum uint32, handNum uint32) (*HandState, error) {
	handState, err := handStatePersist.Load(clubID, gameNum, handNum)
	if err != nil {
		return nil, err
	}

	return handState, nil
}

func (h *HandState) PrintTable(players map[uint32]string) string {
	var b strings.Builder
	b.Grow(32)
	fmt.Fprintf(&b, "Game Num: %d Hand Num: %d, Seats: [", h.GameNum, h.HandNum)
	for seatNo, playerID := range h.GetPlayersInSeats() {
		seatNo++
		if playerID == 0 {
			// empty seat
			fmt.Fprintf(&b, " {%d: Empty}, ", seatNo)
		} else {
			player, _ := players[playerID]
			playerState, _ := h.PlayersState[playerID]
			playerCards, _ := h.PlayersCards[playerID]
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

func (h *HandState) initialize(gameState *GameState, deck *poker.Deck, buttonPos int32) {
	// settle players in the seats
	h.PlayersInSeats = gameState.GetPlayersInSeats()
	h.NoActiveSeats = 0
	for _, playerID := range h.PlayersInSeats {
		if playerID != 0 {
			h.NoActiveSeats++
		}
	}

	h.ActiveSeats = gameState.GetPlayersInSeats()

	// determine button and blinds
	if buttonPos == -1 {
		h.ButtonPos = h.moveButton(gameState)
	} else {
		h.ButtonPos = uint32(buttonPos)
	}

	h.SmallBlindPos, h.BigBlindPos = h.getBlindPos(gameState)

	// copy player's stack (we need to copy only the players that are in the hand)
	h.PlayersState = h.getPlayersState(gameState)

	if deck == nil {
		deck = poker.NewDeck().Shuffle()
	}

	h.Deck = deck.GetBytes()
	h.PlayersCards = h.getPlayersCards(gameState, deck)

	// setup hand for preflop
	h.setupPreflob(gameState)
}

func (h *HandState) setupPreflob(gameState *GameState) {
	h.CurrentState = HandStatus_PREFLOP

	// set next action information
	h.CurrentRaise = gameState.BigBlind
	h.ActionCompleteAtSeat = h.BigBlindPos
	h.PreflopActions = &HandActionLog{Actions: make([]*HandAction, 0)}
	h.FlopActions = &HandActionLog{Actions: make([]*HandAction, 0)}
	h.TurnActions = &HandActionLog{Actions: make([]*HandAction, 0)}
	h.RiverActions = &HandActionLog{Actions: make([]*HandAction, 0)}
	h.actionReceived(gameState, &HandAction{
		SeatNo: h.SmallBlindPos,
		Action: ACTION_SB,
		Amount: gameState.SmallBlind,
	})
	h.actionReceived(gameState, &HandAction{
		SeatNo: h.BigBlindPos,
		Action: ACTION_BB,
		Amount: gameState.BigBlind,
	})

	// setup all the active seats
	// look for the player who are taking a break

}

func (h *HandState) getPlayersState(gameState *GameState) map[uint32]*HandPlayerState {
	handPlayerState := make(map[uint32]*HandPlayerState, 0)
	for j := 1; j <= int(gameState.GetMaxSeats()); j++ {
		playerID := h.GetPlayersInSeats()[j-1]
		if playerID == 0 {
			continue
		}
		playerState, _ := gameState.GetPlayersState()[playerID]
		if playerState.GetStatus() != PlayerStatus_PLAYING {
			continue
		}
		handPlayerState[playerID] = &HandPlayerState{
			Status:  HandPlayerState_ACTIVE,
			Balance: playerState.CurrentBalance,
		}
	}
	return handPlayerState
}

func (h *HandState) getBlindPos(gameState *GameState) (uint32, uint32) {

	buttonSeat := uint32(h.GetButtonPos())
	smallBlindPos := h.getNextActivePlayer(gameState, buttonSeat)
	bigBlindPos := h.getNextActivePlayer(gameState, smallBlindPos)

	if smallBlindPos == 0 || bigBlindPos == 0 {
		// TODO: handle not enough players condition
		panic("Not enough players")
	}
	return uint32(smallBlindPos), uint32(bigBlindPos)
}

func (h *HandState) getPlayersCards(gameState *GameState, deck *poker.Deck) map[uint32][]byte {
	//playerState := g.state.GetPlayersState()
	noOfCards := 2
	switch gameState.GetGameType() {
	case GameType_HOLDEM:
		noOfCards = 2
	case GameType_PLO:
		noOfCards = 4
	case GameType_PLO_HILO:
		noOfCards = 4
	}

	playerCards := make(map[uint32][]byte)
	for _, playerID := range gameState.GetPlayersInSeats() {
		if playerID != 0 {
			playerCards[playerID] = make([]byte, 0, 4)
		}
	}

	for i := 0; i < noOfCards; i++ {
		seatNo := gameState.ButtonPos
		for {
			seatNo = h.getNextActivePlayer(gameState, seatNo)
			playerID := gameState.GetPlayersInSeats()[seatNo-1]
			card := deck.Draw(1)
			playerCards[playerID] = append(playerCards[playerID], card[0].GetByte())
			if seatNo == gameState.ButtonPos {
				// next round of cards
				break
			}
		}
		for j := uint32(1); j <= gameState.MaxSeats; j++ {

		}
	}

	return playerCards
}

func (h *HandState) moveButton(gameState *GameState) uint32 {
	seatNo := uint32(gameState.GetButtonPos())
	newButtonPos := h.getNextActivePlayer(gameState, seatNo)
	if newButtonPos == 0 {
		// TODO: return proper error code
		panic("Not enough players")
	}
	return newButtonPos
}

/**
This helper method returns the next active player from the specified seat number.
It is a useful function to determine moving button, blinds, next action
**/
func (h *HandState) getNextActivePlayer(gameState *GameState, seatNo uint32) uint32 {
	nextSeat := uint32(0)
	var j uint32
	for j = 1; j <= gameState.MaxSeats; j++ {
		seatNo++
		if seatNo > gameState.MaxSeats {
			seatNo = 1
		}

		playerID := gameState.GetPlayersInSeats()[seatNo-1]
		// check to see whether the player is playing or sitting out
		if playerID == 0 {
			continue
		}

		if state, ok := h.GetPlayersState()[playerID]; ok {
			if state.Status != HandPlayerState_ACTIVE {
				continue
			}
		}

		nextSeat = seatNo
		break
	}

	return nextSeat
}

func (h *HandState) actionReceived(gameState *GameState, action *HandAction) error {
	if h.NextSeatAction != nil {
		if action.SeatNo != h.NextSeatAction.SeatNo {
			handLogger.Error().
				Uint32("game", gameState.GetGameNum()).
				Uint32("hand", gameState.GetHandNum()).
				Msg(fmt.Sprintf("Invalid seat %d made action. Ignored. The next valid action seat is: %d", action.SeatNo, h.NextSeatAction.SeatNo))
		}
	}

	// get player ID from the seat
	playerID := gameState.GetPlayersInSeats()[action.SeatNo-1]
	if playerID == 0 {
		// something wrong
		handLogger.Error().
			Uint32("game", gameState.GetGameNum()).
			Uint32("hand", gameState.GetHandNum()).
			Uint32("seat", action.SeatNo).
			Msg(fmt.Sprintf("Invalid seat %d. PlayerID is 0", action.SeatNo))
	}

	playerState := h.GetPlayersState()[playerID]

	var log *HandActionLog
	switch h.CurrentState {
	case HandStatus_PREFLOP:
		log = h.PreflopActions
	case HandStatus_FLOP:
		log = h.FlopActions
	case HandStatus_TURN:
		log = h.TurnActions
	case HandStatus_RIVER:
		log = h.RiverActions
	}

	// valid actions
	if action.Action == ACTION_FOLD {
		h.ActiveSeats[action.SeatNo-1] = 0
		h.NoActiveSeats--
		playerState.Status = HandPlayerState_FOLDED
		log.Actions = append(log.Actions, action)
		h.NextSeatAction = h.prepareNextAction(gameState, action)
	} else if action.Action == ACTION_CHECK {
		log.Actions = append(log.Actions, action)
		h.NextSeatAction = h.prepareNextAction(gameState, action)
	} else {
		// action call
		if action.Action == ACTION_CALL {
			if action.Amount < h.GetCurrentRaise() {
				// all in?
				if playerState.GetBalance() < h.GetCurrentRaise() {
					action.Action = ACTION_ALL_IN
				}
			}
		}

		// TODO: we need to handle raise and determine next min raise
		if action.Action == ACTION_RAISE {
			if action.Amount < h.GetCurrentRaise() {
				// invalid
				handLogger.Error().
					Uint32("game", gameState.GetGameNum()).
					Uint32("hand", gameState.GetHandNum()).
					Uint32("seat", action.SeatNo).
					Msg(fmt.Sprintf("Invalid raise %f. Current bet: %f", action.Amount, h.GetCurrentRaise()))
			}
		}
		// raisedAmount := action.Amount - h.CurrentRaise
		if action.Amount > h.CurrentRaise {
			h.ActionCompleteAtSeat = action.SeatNo
		}
		h.CurrentRaise = action.Amount

		// add the action to the log
		log.Actions = append(log.Actions, action)
		log.Pot = log.Pot + action.Amount

		h.NextSeatAction = h.prepareNextAction(gameState, action)
	}
	return nil
}

func (h *HandState) getPlayerFromSeat(seatNo uint32) *HandPlayerState {
	playerID := h.GetPlayersInSeats()[seatNo-1]
	if playerID == 0 {
		return nil
	}
	playerState, _ := h.PlayersState[playerID]
	return playerState
}

func (h *HandState) prepareNextAction(gameState *GameState, currentAction *HandAction) *NextSeatAction {
	// compute next action
	actionSeat := h.getNextActivePlayer(gameState, currentAction.SeatNo)
	playerState := h.getPlayerFromSeat(actionSeat)
	if playerState == nil {
		// something wrong
		panic("Something went wrong. player id cannot be null")
	}
	nextAction := &NextSeatAction{SeatNo: actionSeat}
	availableActions := make([]ACTION, 0)
	availableActions = append(availableActions, ACTION_FOLD)
	if h.CurrentState == HandStatus_PREFLOP {
		if currentAction.Action == ACTION_BB {
			// the player can straddle, if he has enough money
			straddleAmount := 2.0 * gameState.BigBlind
			if playerState.Balance >= straddleAmount {
				availableActions = append(availableActions, ACTION_STRADDLE)
				nextAction.StraddleAmount = gameState.BigBlind * 2.0
			}
		}
	}

	allInAvailable := false
	if h.CurrentRaise == 0.0 {
		// then the caller can check
		availableActions = append(availableActions, ACTION_CHECK)
	} else {
		// then the caller call, raise, or go all in
		if playerState.Balance <= h.CurrentRaise || h.GameType == GameType_HOLDEM {
			allInAvailable = true
		}

		if playerState.Balance > h.CurrentRaise {
			availableActions = append(availableActions, ACTION_CALL)
			nextAction.CallAmount = h.CurrentRaise

			if playerState.Balance < h.CurrentRaise*2 {
				// the player can go all in
				allInAvailable = true
			}
			availableActions = append(availableActions, ACTION_RAISE)
			nextAction.MinRaiseAmount = h.CurrentRaise * 2
		}

		if allInAvailable {
			availableActions = append(availableActions, ACTION_ALL_IN)
			nextAction.AllInAmount = playerState.Balance
		}

		nextAction.AvailableActions = availableActions
	}

	return nextAction
}

func (n *NextSeatAction) PrettyPrint(h *HandState, gameState *GameState, players map[uint32]string) string {
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
		case ACTION_ALL_IN:
			fmt.Fprintf(&b, "{ALL_IN, allInAmount: %f},", n.AllInAmount)
		case ACTION_STRADDLE:
			fmt.Fprintf(&b, "{STRADDLE, straddleAmount: %f},", n.StraddleAmount)
		}
	}
	return b.String()
}

func (h *HandState) PrintCurrentActionLog(gameState *GameState, players map[uint32]string) string {
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

func (a *HandAction) Print(h *HandState, gameState *GameState, players map[uint32]string) string {
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
