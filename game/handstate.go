package game

import (
	"fmt"

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

func (h *HandState) initializeBettingRound(gameState *GameState) {
	maxSeats := gameState.MaxSeats
	h.RoundBetting = make(map[uint32]*SeatBetting)
	h.RoundBetting[uint32(HandStatus_PREFLOP)] = &SeatBetting{SeatBet: make([]float32, maxSeats)}
	h.RoundBetting[uint32(HandStatus_FLOP)] = &SeatBetting{SeatBet: make([]float32, maxSeats)}
	h.RoundBetting[uint32(HandStatus_TURN)] = &SeatBetting{SeatBet: make([]float32, maxSeats)}
	h.RoundBetting[uint32(HandStatus_RIVER)] = &SeatBetting{SeatBet: make([]float32, maxSeats)}

	// setup player acted tracking
	h.PlayersActed = make([]PlayerActRound, gameState.MaxSeats)
	h.resetPlayerActions()
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
	// if the players don't have money less than the blinds
	// don't let them play
	h.ActiveSeats = gameState.GetPlayersInSeats()

	// determine button and blinds
	if buttonPos == -1 {
		h.ButtonPos = h.moveButton(gameState)
	} else {
		h.ButtonPos = uint32(buttonPos)
	}

	// TODO: make sure small blind is still there
	// if small blind left the game, we can have dead small
	// to make it simple, we will make new players to always to post or wait for the big blind
	h.SmallBlindPos, h.BigBlindPos = h.getBlindPos(gameState)

	// copy player's stack (we need to copy only the players that are in the hand)
	h.PlayersState = h.getPlayersState(gameState)

	if deck == nil {
		deck = poker.NewDeck(nil).Shuffle()
	}

	h.Deck = deck.GetBytes()
	h.PlayersCards = h.getPlayersCards(gameState, deck)

	// setup main pot
	h.Pots = make([]*SeatsInPots, 0)
	mainPot := initializePot(h, gameState)
	h.Pots = append(h.Pots, mainPot)

	// setup data structure to handle betting rounds
	h.initializeBettingRound(gameState)

	// setup hand for preflop
	h.setupPreflob(gameState)
}

func (h *HandState) setupPreflob(gameState *GameState) {
	h.CurrentState = HandStatus_PREFLOP

	// set next action information
	h.PreflopActions = &HandActionLog{Actions: make([]*HandAction, 0)}
	h.FlopActions = &HandActionLog{Actions: make([]*HandAction, 0)}
	h.TurnActions = &HandActionLog{Actions: make([]*HandAction, 0)}
	h.RiverActions = &HandActionLog{Actions: make([]*HandAction, 0)}
	h.CurrentRaise = gameState.BigBlind

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

	h.ActionCompleteAtSeat = h.BigBlindPos
	//mainPot := h.Pots[0]
	bettingRound := h.RoundBetting[uint32(HandStatus_PREFLOP)]
	if h.SmallBlindPos != 0 {
		// small blind equity
		smallBlind := gameState.SmallBlind
		//mainPot.SeatPotEquity[h.SmallBlindPos-1] = smallBlind
		//mainPot.Pot += smallBlind
		h.PlayersState[h.PlayersInSeats[h.SmallBlindPos-1]].Balance -= smallBlind
		bettingRound.SeatBet[h.SmallBlindPos-1] = smallBlind
	}

	// big blind equity
	bigBlind := gameState.BigBlind
	//mainPot.SeatPotEquity[h.BigBlindPos-1] = bigBlind
	//mainPot.Pot += bigBlind
	h.PlayersState[h.PlayersInSeats[h.BigBlindPos-1]].Balance -= bigBlind
	bettingRound.SeatBet[h.BigBlindPos-1] = bigBlind

	// big blind has acted, will have last action as well
	//h.PlayersActed[h.BigBlindPos-1] = PlayerActRound_ACTED
	h.actionChanged(h.BigBlindPos)
}

func (h *HandState) resetPlayerActions() {
	for seatNo, playerID := range h.GetPlayersInSeats() {
		if playerID == 0 {
			h.PlayersActed[seatNo] = PlayerActRound_EMPTY_SEAT
			continue
		}
		h.PlayersActed[seatNo] = PlayerActRound_NOT_ACTED
	}
}

func (h *HandState) actionChanged(seatChangedAction uint32) {
	for seatNo := range h.GetPlayersInSeats() {
		if h.PlayersActed[seatNo] == PlayerActRound_EMPTY_SEAT ||
			h.PlayersActed[seatNo] == PlayerActRound_FOLDED ||
			h.PlayersActed[seatNo] == PlayerActRound_ALL_IN ||
			seatNo == int(seatChangedAction-1) {
			continue
		}
		// this player has act again
		h.PlayersActed[seatNo] = PlayerActRound_NOT_ACTED
	}
	h.PlayersActed[seatChangedAction-1] = PlayerActRound_ACTED
}

func (h *HandState) hasEveryOneActed() bool {
	allActed := true
	for seatNo := range h.GetPlayersInSeats() {
		if h.PlayersActed[seatNo] == PlayerActRound_EMPTY_SEAT ||
			h.PlayersActed[seatNo] == PlayerActRound_FOLDED ||
			h.PlayersActed[seatNo] == PlayerActRound_ALL_IN ||
			h.PlayersActed[seatNo] == PlayerActRound_ACTED {
			continue
		}
		allActed = false
		break
	}
	return allActed
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
			h.DeckIndex++
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

	playerBalance := playerState.GetBalance()
	bettingRound := h.RoundBetting[uint32(h.CurrentState)]
	playerBetSoFar := bettingRound.SeatBet[action.SeatNo-1]

	// valid actions
	if action.Action == ACTION_FOLD {
		h.ActiveSeats[action.SeatNo-1] = 0
		h.NoActiveSeats--
		playerState.Status = HandPlayerState_FOLDED
		h.PlayersActed[action.SeatNo-1] = PlayerActRound_FOLDED
	} else if action.Action == ACTION_CHECK {
		h.PlayersActed[action.SeatNo-1] = PlayerActRound_ACTED
	} else if action.Action == ACTION_CALL {
		// action call
		// if this player has an equity in this pot, just call subtract the amount
		diff := h.CurrentRaise - playerBetSoFar
		h.PlayersActed[action.SeatNo-1] = PlayerActRound_ACTED

		// does the player enough money ??
		if playerBalance < diff {
			// he is going all in, crazy
			action.Action = ACTION_ALLIN
			h.PlayersActed[action.SeatNo-1] = PlayerActRound_ALL_IN
			diff = playerBalance
		}
		playerBetSoFar += diff
		playerState.Balance -= diff
		bettingRound.SeatBet[action.SeatNo-1] = playerBetSoFar
	} else if action.Action == ACTION_ALLIN {
		h.PlayersActed[action.SeatNo-1] = PlayerActRound_ALL_IN
		bettingRound.SeatBet[action.SeatNo-1] = playerState.Balance
		playerState.Balance = 0
	} else if action.Action == ACTION_RAISE ||
		action.Action == ACTION_BET {
		// TODO: we need to handle raise and determine next min raise
		if action.Amount < h.GetCurrentRaise() {
			// invalid
			handLogger.Error().
				Uint32("game", gameState.GetGameNum()).
				Uint32("hand", gameState.GetHandNum()).
				Uint32("seat", action.SeatNo).
				Msg(fmt.Sprintf("Invalid raise %f. Current bet: %f", action.Amount, h.GetCurrentRaise()))
		}

		h.PlayersActed[action.SeatNo-1] = PlayerActRound_ACTED
		if action.Action == ACTION_ALLIN {
			h.PlayersActed[action.SeatNo-1] = PlayerActRound_ALL_IN
			// player is all in
			action.Amount = playerBetSoFar + playerState.Balance
		}

		if action.Amount > h.CurrentRaise {
			h.CurrentRaise = action.Amount

			// reset player action
			h.actionChanged(action.SeatNo)
			h.ActionCompleteAtSeat = action.SeatNo
		}

		// how much this user already had in the betting round
		diff := action.Amount - playerBetSoFar

		if diff == playerState.Balance {
			// player is all in
			action.Action = ACTION_ALLIN
			h.PlayersActed[action.SeatNo-1] = PlayerActRound_ALL_IN
			playerState.Balance = 0
		} else {
			playerState.Balance -= diff
		}

		bettingRound.SeatBet[action.SeatNo-1] = action.Amount
	}
	// add the action to the log
	log.Actions = append(log.Actions, action)
	log.Pot = log.Pot + action.Amount

	// check whether everyone has acted in this ROUND
	if h.hasEveryOneActed() {
		// move to next round
		h.NextSeatAction = h.gotoNextStage(gameState)
	} else {
		h.NextSeatAction = h.prepareNextAction(gameState, action)
	}
	return nil
}

func index(vs []uint32, t uint32) int {
	for i, v := range vs {
		if v == t {
			return i
		}
	}
	return -1
}

func (h *HandState) getPlayerFromSeat(seatNo uint32) *HandPlayerState {
	playerID := h.GetPlayersInSeats()[seatNo-1]
	if playerID == 0 {
		return nil
	}
	playerState, _ := h.PlayersState[playerID]
	return playerState
}

func (h *HandState) gotoNextStage(gameState *GameState) *NextSeatAction {
	// before we go to next stage, settle pots
	currentBettingRound := h.RoundBetting[uint32(h.CurrentState)]
	currentPot := len(h.Pots) - 1
	for _, bet := range currentBettingRound.SeatBet {
		h.Pots[currentPot].Pot += bet
	}

	// if only one player is active, then announce the result and go to next hand
	if h.NoActiveSeats == 1 {
		h.HandCompletedAt = h.CurrentState
		h.CurrentState = HandStatus_RESULT
		return nil
	}

	// if this player is last to act, then move to the next round
	if h.CurrentState == HandStatus_PREFLOP {
		h.CurrentState = HandStatus_FLOP
	} else if h.CurrentState == HandStatus_FLOP {
		h.CurrentState = HandStatus_TURN
	} else if h.CurrentState == HandStatus_TURN {
		h.CurrentState = HandStatus_RIVER
	} else if h.CurrentState == HandStatus_RIVER {
		h.CurrentState = HandStatus_SHOW_DOWN
	}

	actionSeat := h.getNextActivePlayer(gameState, h.ButtonPos)
	playerState := h.getPlayerFromSeat(actionSeat)
	if playerState == nil {
		// something wrong
		panic("Something went wrong. player id cannot be null")
	}
	h.resetPlayerActions()

	availableActions := make([]ACTION, 0)
	availableActions = append(availableActions, ACTION_FOLD)
	availableActions = append(availableActions, ACTION_CHECK)
	availableActions = append(availableActions, ACTION_BET)

	nextAction := &NextSeatAction{SeatNo: actionSeat}
	nextAction.MinRaiseAmount = gameState.BigBlind
	nextAction.MaxRaiseAmount = gameState.BigBlind
	nextAction.AvailableActions = availableActions
	availableActions = append(availableActions, ACTION_ALLIN)
	nextAction.AllInAmount = playerState.Balance

	return nextAction
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
			availableActions = append(availableActions, ACTION_ALLIN)
			nextAction.AllInAmount = playerState.Balance
		}

		nextAction.AvailableActions = availableActions
	}

	return nextAction
}

func (h *HandState) determineWinners() {
	seatNo := -1
	for i, playerID := range h.ActiveSeats {
		if playerID != 0 {
			seatNo = i + 1
			break
		}
	}

	// take rake from here

	potWinners := make(map[uint32]*PotWinners)
	for i, pot := range h.Pots {
		playerID := h.GetPlayersInSeats()[seatNo-1]
		h.PlayersState[playerID].Balance += pot.Pot
		// only one pot
		handWinner := &HandWinner{
			SeatNo: uint32(seatNo),
			Amount: pot.Pot,
		}
		handWinners := make([]*HandWinner, 0)
		handWinners = append(handWinners, handWinner)
		potWinners[uint32(i)] = &PotWinners{HandWinner: handWinners}
	}
	h.PotWinners = potWinners
	h.CurrentState = HandStatus_RESULT

	h.BalanceAfterHand = make([]*PlayerBalance, 0)
	// also populate current balance of the players in the table
	for seatNo, player := range h.GetPlayersInSeats() {
		if player == 0 {
			continue
		}
		state := h.GetPlayersState()[player]
		h.BalanceAfterHand = append(h.BalanceAfterHand,
			&PlayerBalance{SeatNo: uint32(seatNo + 1), PlayerId: player, Balance: state.Balance})
	}
}

func (h *HandState) getResult() *HandResult {
	handResult := &HandResult{}
	handResult.PotWinners = h.PotWinners
	handResult.WonAt = h.HandCompletedAt
	handResult.PreflopActions = h.PreflopActions
	handResult.FlopActions = h.FlopActions
	handResult.TurnActions = h.TurnActions
	handResult.RiverActions = h.RiverActions
	handResult.BalanceAfterHand = h.BalanceAfterHand
	return handResult
}
