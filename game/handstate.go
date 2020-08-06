package game

import (
	"fmt"
	"time"

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

	h.BalanceBeforeHand = make([]*PlayerBalance, 0)
	// also populate current balance of the players in the table
	for seatNo, player := range h.ActiveSeats {
		if player == 0 {
			continue
		}
		state := h.GetPlayersState()[player]
		h.BalanceBeforeHand = append(h.BalanceBeforeHand,
			&PlayerBalance{SeatNo: uint32(seatNo + 1), PlayerId: player, Balance: state.Balance})
	}

	if deck == nil {
		deck = poker.NewDeck(nil).Shuffle()
	}

	h.Deck = deck.GetBytes()
	h.PlayersCards = h.getPlayersCards(gameState, deck)

	// setup main pot
	h.Pots = make([]*SeatsInPots, 0)
	mainPot := initializePot(int(gameState.MaxSeats))
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
	// initialize all-in players list
	h.AllInPlayers = make([]uint32, gameState.MaxSeats)

	h.ActionCompleteAtSeat = h.BigBlindPos
	bettingRound := h.RoundBetting[uint32(HandStatus_PREFLOP)]
	if h.SmallBlindPos != 0 {
		// small blind equity
		smallBlind := gameState.SmallBlind
		h.PlayersState[h.PlayersInSeats[h.SmallBlindPos-1]].Balance -= smallBlind
		bettingRound.SeatBet[h.SmallBlindPos-1] = smallBlind
	}

	// big blind equity
	bigBlind := gameState.BigBlind
	h.PlayersState[h.PlayersInSeats[h.BigBlindPos-1]].Balance -= bigBlind
	bettingRound.SeatBet[h.BigBlindPos-1] = bigBlind
	h.actionChanged(h.BigBlindPos, false)

	// big blind is the last one to act
	h.PlayersActed[h.BigBlindPos-1] = PlayerActRound_PLAYER_ACT_BB
}

func (h *HandState) resetPlayerActions() {
	for seatNo, playerID := range h.GetPlayersInSeats() {
		if playerID == 0 || h.ActiveSeats[seatNo] == 0 {
			h.PlayersActed[seatNo] = PlayerActRound_PLAYER_ACT_EMPTY_SEAT
			continue
		}
		h.PlayersActed[seatNo] = PlayerActRound_PLAYER_ACT_NOT_ACTED
	}
}

func (h *HandState) actionChanged(seatChangedAction uint32, allin bool) {
	for seatNo := range h.GetPlayersInSeats() {
		if h.PlayersActed[seatNo] == PlayerActRound_PLAYER_ACT_EMPTY_SEAT ||
			h.PlayersActed[seatNo] == PlayerActRound_PLAYER_ACT_FOLDED ||
			h.PlayersActed[seatNo] == PlayerActRound_PLAYER_ACT_ALL_IN ||
			seatNo == int(seatChangedAction-1) {
			continue
		}
		// this player has act again
		h.PlayersActed[seatNo] = PlayerActRound_PLAYER_ACT_NOT_ACTED
	}
	if allin {
		h.PlayersActed[seatChangedAction-1] = PlayerActRound_PLAYER_ACT_ALL_IN
	} else {
		h.PlayersActed[seatChangedAction-1] = PlayerActRound_PLAYER_ACT_ACTED
	}
}

func (h *HandState) hasEveryOneActed() bool {
	allActed := true
	for seatNo := range h.GetPlayersInSeats() {
		if h.PlayersActed[seatNo] == PlayerActRound_PLAYER_ACT_EMPTY_SEAT ||
			h.PlayersActed[seatNo] == PlayerActRound_PLAYER_ACT_FOLDED ||
			h.PlayersActed[seatNo] == PlayerActRound_PLAYER_ACT_ALL_IN ||
			h.PlayersActed[seatNo] == PlayerActRound_PLAYER_ACT_ACTED {
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
	for seatNoIdx, playerID := range gameState.GetPlayersInSeats() {
		if playerID != 0 {
			playerCards[uint32(seatNoIdx+1)] = make([]byte, 0, 4)
		}
	}

	for i := 0; i < noOfCards; i++ {
		seatNo := gameState.ButtonPos
		for {
			seatNo = h.getNextActivePlayer(gameState, seatNo)
			card := deck.Draw(1)
			h.DeckIndex++
			playerCards[seatNo] = append(playerCards[seatNo], card[0].GetByte())
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
				Msg(fmt.Sprintf("Invalid seat %d made action. Ignored. The next valid action seat is: %d",
					action.SeatNo, h.NextSeatAction.SeatNo))
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
	h.LastState = h.CurrentState
	playerBalance := playerState.GetBalance()
	bettingRound := h.RoundBetting[uint32(h.CurrentState)]
	playerBetSoFar := bettingRound.SeatBet[action.SeatNo-1]

	// valid actions
	if action.Action == ACTION_FOLD {
		h.ActiveSeats[action.SeatNo-1] = 0
		h.NoActiveSeats--
		playerState.Status = HandPlayerState_FOLDED
		h.PlayersActed[action.SeatNo-1] = PlayerActRound_PLAYER_ACT_FOLDED
	} else if action.Action == ACTION_CHECK {
		h.PlayersActed[action.SeatNo-1] = PlayerActRound_PLAYER_ACT_ACTED
	} else if action.Action == ACTION_CALL {
		// action call
		// if this player has an equity in this pot, just call subtract the amount
		diff := h.CurrentRaise - playerBetSoFar
		h.PlayersActed[action.SeatNo-1] = PlayerActRound_PLAYER_ACT_ACTED

		// does the player enough money ??
		if playerBalance < diff {
			// he is going all in, crazy
			action.Action = ACTION_ALLIN
			h.PlayersActed[action.SeatNo-1] = PlayerActRound_PLAYER_ACT_ALL_IN
			h.AllInPlayers[action.SeatNo-1] = 1
			diff = playerBalance
		}
		playerBetSoFar += diff
		playerState.Balance -= diff
		bettingRound.SeatBet[action.SeatNo-1] = playerBetSoFar
	} else if action.Action == ACTION_ALLIN {
		h.PlayersActed[action.SeatNo-1] = PlayerActRound_PLAYER_ACT_ALL_IN
		h.AllInPlayers[action.SeatNo-1] = 1
		amount := playerState.Balance + playerBetSoFar
		bettingRound.SeatBet[action.SeatNo-1] = amount
		playerState.Balance = 0

		if amount > h.CurrentRaise {
			h.actionChanged(action.SeatNo, true)
		}
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

		h.PlayersActed[action.SeatNo-1] = PlayerActRound_PLAYER_ACT_ACTED
		allin := false
		if action.Action == ACTION_ALLIN {
			allin = true
			// player is all in
			action.Amount = playerBetSoFar + playerState.Balance
		}

		if action.Amount > h.CurrentRaise {
			h.CurrentRaise = action.Amount

			// reset player action
			h.actionChanged(action.SeatNo, allin)
			h.ActionCompleteAtSeat = action.SeatNo
		}

		// how much this user already had in the betting round
		diff := action.Amount - playerBetSoFar

		if diff == playerState.Balance {
			// player is all in
			action.Action = ACTION_ALLIN
			h.PlayersActed[action.SeatNo-1] = PlayerActRound_PLAYER_ACT_ALL_IN
			h.AllInPlayers[action.SeatNo-1] = 1
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
	// or everyone except folded in this hand
	if h.hasEveryOneActed() || h.NoActiveSeats == 1 {
		// settle this round and move to next round
		h.settleRound()
		// next seat action will be determined outside of here
		// after moving to next round
		h.NextSeatAction = nil
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

func (h *HandState) isAllActivePlayersAllIn() bool {
	allIn := true
	for seatNoIdx, playerID := range h.ActiveSeats {
		if playerID == 0 {
			continue
		}
		if h.AllInPlayers[seatNoIdx] == 0 {
			allIn = false
			break
		}
	}
	return allIn
}

func (h *HandState) settleRound() {
	// before we go to next stage, settle pots
	currentBettingRound := h.RoundBetting[uint32(h.CurrentState)]

	// if only one player is active, then this hand is concluded
	handEnded := false
	if h.NoActiveSeats == 1 {
		handEnded = true
	} else {
		// we need to find the second largest bet
		// subtract that money from the largest bet player
		// and return the balance back to the player
		// for example, if two players go all in a hand
		// player 1 has 50 chips and player 2 has 100 chips
		// then the action is over, we need to return 50 chips
		// back to player 1
		maxBetPos := -1
		// we should have atleast two seats to play
		maxBet := float32(0)
		secondMaxBet := float32(0)
		seatBets := currentBettingRound.SeatBet
		if seatBets[0] < seatBets[1] {
			maxBet = seatBets[1]
			secondMaxBet = seatBets[0]
			maxBetPos = 1
		} else {
			maxBet = seatBets[0]
			secondMaxBet = seatBets[1]
			maxBetPos = 0
		}
		for seatIdx := 2; seatIdx < len(seatBets); seatIdx++ {
			bet := seatBets[seatIdx]
			if bet > maxBet {
				secondMaxBet = maxBet
				maxBet = bet
				maxBetPos = seatIdx
			}
		}
		if maxBetPos > 0 {
			playerID := h.PlayersInSeats[maxBetPos]
			h.PlayersState[playerID].Balance = maxBet - secondMaxBet
		}
	}

	h.addChipsToPot(currentBettingRound.SeatBet, handEnded)

	if handEnded {
		h.HandCompletedAt = h.CurrentState
		h.CurrentState = HandStatus_RESULT
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
}

func (h *HandState) setupNextRound(state HandStatus, gameState *GameState) {
	h.CurrentState = state
	actionSeat := h.getNextActivePlayer(gameState, h.ButtonPos)
	playerState := h.getPlayerFromSeat(actionSeat)
	if playerState == nil {
		// something wrong
		panic("Something went wrong. player id cannot be null")
	}
	h.resetPlayerActions()
	h.CurrentRaise = 0
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

	h.NextSeatAction = nextAction
}

func (h *HandState) setupFlop(gameState *GameState, board []uint32) {
	h.setupNextRound(HandStatus_FLOP, gameState)
	h.BoardCards = make([]byte, 3)
	for i, card := range board {
		h.BoardCards[i] = uint8(card)
	}
}

func (h *HandState) setupTurn(gameState *GameState, turnCard uint32) {
	h.setupNextRound(HandStatus_TURN, gameState)
	h.BoardCards = append(h.BoardCards, uint8(turnCard))
}

func (h *HandState) setupRiver(gameState *GameState, riverCard uint32) {
	h.setupNextRound(HandStatus_RIVER, gameState)
	h.BoardCards = append(h.BoardCards, uint8(riverCard))
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

func (h *HandState) everyOneFoldedWinners() {
	seatNo := -1
	for i, playerID := range h.ActiveSeats {
		if playerID != 0 {
			seatNo = i + 1
			break
		}
	}
	potWinners := make(map[uint32]*PotWinners)
	for i, pot := range h.Pots {
		handWinner := &HandWinner{
			SeatNo: uint32(seatNo),
			Amount: pot.Pot,
		}
		handWinners := make([]*HandWinner, 0)
		handWinners = append(handWinners, handWinner)
		potWinners[uint32(i)] = &PotWinners{HiWinners: handWinners}
	}
	h.setWinners(potWinners)
}

func (h *HandState) setWinners(potWinners map[uint32]*PotWinners) {
	h.PotWinners = potWinners
	h.CurrentState = HandStatus_RESULT

	// take rake from here

	// update player balance
	for _, pot := range potWinners {
		for _, handWinner := range pot.HiWinners {
			seatNo := handWinner.SeatNo
			playerID := h.GetPlayersInSeats()[seatNo-1]
			h.PlayersState[playerID].Balance += handWinner.Amount
		}
		for _, handWinner := range pot.LowWinners {
			seatNo := handWinner.SeatNo
			playerID := h.GetPlayersInSeats()[seatNo-1]
			h.PlayersState[playerID].Balance += handWinner.Amount
		}
	}

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
	handResult.BalanceBeforeHand = h.BalanceBeforeHand
	handResult.HandStartedAt = h.HandStartedAt
	handResult.HandEndedAt = h.HandEndedAt
	handResult.PlayersInSeats = h.PlayersInSeats
	handResult.HandEndedAt = uint64(time.Now().Unix())
	return handResult
}