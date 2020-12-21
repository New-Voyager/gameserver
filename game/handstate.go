package game

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"voyager.com/server/poker"
)

var handLogger = log.With().Str("logger_name", "game::hand").Logger()

func LoadHandState(handStatePersist PersistHandState, clubID uint32, gameID uint64, handNum uint32) (*HandState, error) {
	handState, err := handStatePersist.Load(clubID, gameID, handNum)
	if err != nil {
		return nil, err
	}

	return handState, nil
}

func (h *HandState) initializeBettingRound() {
	maxSeats := h.MaxSeats
	h.RoundBetting = make(map[uint32]*SeatBetting)
	h.RoundBetting[uint32(HandStatus_PREFLOP)] = &SeatBetting{SeatBet: make([]float32, maxSeats)}
	h.RoundBetting[uint32(HandStatus_FLOP)] = &SeatBetting{SeatBet: make([]float32, maxSeats)}
	h.RoundBetting[uint32(HandStatus_TURN)] = &SeatBetting{SeatBet: make([]float32, maxSeats)}
	h.RoundBetting[uint32(HandStatus_RIVER)] = &SeatBetting{SeatBet: make([]float32, maxSeats)}

	// setup player acted tracking
	h.PlayersActed = make([]*PlayerActRound, h.MaxSeats)
	h.resetPlayerActions()
}

func (h *HandState) initialize(gameState *GameState, deck *poker.Deck, buttonPos uint32, moveButton bool) {
	// settle players in the seats
	h.PlayersInSeats = make([]uint64, gameState.MaxSeats)
	h.NoActiveSeats = 0

	// update active seats with players who are playing
	for seatNo, playerID := range gameState.GetPlayersInSeats() {
		if playerID != 0 {
			// get player state
			state := gameState.PlayersState[playerID]
			if state == nil {
				continue
			}
			if state.Status == PlayerStatus_IN_BREAK || state.CurrentBalance == 0 {
				continue
			}
			h.PlayersInSeats[seatNo] = playerID
			h.NoActiveSeats++
		}
	}
	h.MaxSeats = gameState.MaxSeats
	h.SmallBlind = gameState.SmallBlind
	h.BigBlind = gameState.BigBlind
	h.Straddle = gameState.Straddle
	h.RakePercentage = gameState.RakePercentage
	h.RakeCap = gameState.RakeCap
	h.ButtonPos = buttonPos

	// if the players don't have money less than the blinds
	// don't let them play
	h.ActiveSeats = gameState.GetPlayersInSeats()

	// determine button and blinds
	if moveButton {
		h.ButtonPos = h.moveButton()
	}

	// TODO: make sure small blind is still there
	// if small blind left the game, we can have dead small
	// to make it simple, we will make new players to always to post or wait for the big blind
	h.SmallBlindPos, h.BigBlindPos = h.getBlindPos()

	// copy player's stack (we need to copy only the players that are in the hand)
	h.PlayersState = h.copyPlayersState(gameState)

	h.BalanceBeforeHand = make([]*PlayerBalance, 0)
	// also populate current balance of the players in the table
	for seatNo, player := range h.ActiveSeats {
		if player == 0 {
			continue
		}
		state := gameState.GetPlayersState()[player]
		h.BalanceBeforeHand = append(h.BalanceBeforeHand,
			&PlayerBalance{SeatNo: uint32(seatNo + 1), PlayerId: player, Balance: state.CurrentBalance})
	}

	if deck == nil {
		deck = poker.NewDeck(nil).Shuffle()
	}

	h.Deck = deck.GetBytes()
	h.PlayersCards = h.getPlayersCards(deck)

	// setup main pot
	h.Pots = make([]*SeatsInPots, 0)
	mainPot := initializePot(int(gameState.MaxSeats))
	h.Pots = append(h.Pots, mainPot)
	h.RakePaid = make(map[uint64]float32, 0)

	deck.Draw(1)
	h.DeckIndex++
	cards := deck.Draw(3)
	h.DeckIndex += 3
	h.FlopCards = make([]uint32, 3)
	fmt.Printf("Flop Cards: ")
	for i, card := range cards {
		h.FlopCards[i] = uint32(card.GetByte())
		fmt.Printf("%s", poker.CardToString(uint32(card.GetByte())))
	}
	fmt.Printf("\n")

	// burn card
	cards = deck.Draw(1)
	burnCard := uint32(cards[0].GetByte())
	fmt.Printf("Burn Card: %s\n", poker.CardToString(burnCard))
	h.DeckIndex++
	// turn card
	cards = deck.Draw(1)
	h.DeckIndex++
	h.TurnCard = uint32(cards[0].GetByte())
	fmt.Printf("Turn card: %s\n", poker.CardToString(h.TurnCard))

	// burn card
	cards = deck.Draw(1)
	h.DeckIndex++
	burnCard = uint32(cards[0].GetByte())
	fmt.Printf("Burn Card: %s\n", poker.CardToString(burnCard))

	// river card
	cards = deck.Draw(1)
	h.DeckIndex++
	h.RiverCard = uint32(cards[0].GetByte())
	fmt.Printf("River card: %s\n", poker.CardToString(h.RiverCard))

	// setup data structure to handle betting rounds
	h.initializeBettingRound()

	// setup hand for preflop
	h.setupPreflob()
}

func (h *HandState) copyPlayersState(gameState *GameState) map[uint64]*HandPlayerState {
	handPlayerState := make(map[uint64]*HandPlayerState, 0)
	for j := 1; j <= int(h.GetMaxSeats()); j++ {
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

func (h *HandState) setupPreflob() {
	h.CurrentState = HandStatus_PREFLOP

	// set next action information
	h.PreflopActions = &HandActionLog{Actions: make([]*HandAction, 0)}
	h.FlopActions = &HandActionLog{Actions: make([]*HandAction, 0)}
	h.TurnActions = &HandActionLog{Actions: make([]*HandAction, 0)}
	h.RiverActions = &HandActionLog{Actions: make([]*HandAction, 0)}
	h.CurrentRaise = h.BigBlind

	h.actionReceived(&HandAction{
		SeatNo: h.SmallBlindPos,
		Action: ACTION_SB,
		Amount: h.SmallBlind,
	})
	h.actionReceived(&HandAction{
		SeatNo: h.BigBlindPos,
		Action: ACTION_BB,
		Amount: h.BigBlind,
	})
	// initialize all-in players list
	h.AllInPlayers = make([]uint32, h.MaxSeats)

	h.ActionCompleteAtSeat = h.BigBlindPos
	bettingRound := h.RoundBetting[uint32(HandStatus_PREFLOP)]
	if h.SmallBlindPos != 0 {
		// small blind equity
		smallBlind := h.SmallBlind
		h.PlayersState[h.PlayersInSeats[h.SmallBlindPos-1]].Balance -= smallBlind
		bettingRound.SeatBet[h.SmallBlindPos-1] = smallBlind
	}

	// big blind equity
	bigBlind := h.BigBlind
	h.PlayersState[h.PlayersInSeats[h.BigBlindPos-1]].Balance -= bigBlind
	bettingRound.SeatBet[h.BigBlindPos-1] = bigBlind
	h.actionChanged(h.BigBlindPos, PlayerActState_PLAYER_ACT_BB, bigBlind)

	// big blind is the last one to act
	h.PlayersActed[h.BigBlindPos-1] = &PlayerActRound{State: PlayerActState_PLAYER_ACT_BB, Amount: bigBlind}

	// track whether the player is active in this round or not
	for j := 1; j <= int(h.GetMaxSeats()); j++ {
		playerID := h.GetPlayersInSeats()[j-1]
		if playerID == 0 {
			continue
		}
		playerState, found := h.GetPlayersState()[playerID]
		if found {
			playerState.Round = HandStatus_PREFLOP
		}
	}
}

func (h *HandState) resetPlayerActions() {
	for seatNo, playerID := range h.GetPlayersInSeats() {
		if playerID == 0 || h.ActiveSeats[seatNo] == 0 {
			h.PlayersActed[seatNo] = &PlayerActRound{
				State: PlayerActState_PLAYER_ACT_EMPTY_SEAT,
			}
			continue
		}
		h.PlayersActed[seatNo] = &PlayerActRound{
			State: PlayerActState_PLAYER_ACT_NOT_ACTED,
		}
	}
}

func (h *HandState) actionChanged(seatChangedAction uint32, state PlayerActState, amount float32) {

	/*
		for seatNo := range h.GetPlayersInSeats() {
			state := h.PlayersActed[seatNo].State
			if state == PlayerActState_PLAYER_ACT_EMPTY_SEAT ||
				state == PlayerActState_PLAYER_ACT_FOLDED ||
				state == PlayerActState_PLAYER_ACT_ALL_IN ||
				seatNo == int(seatChangedAction-1) {
				continue
			}
			// this player has to act again
			//h.PlayersActed[seatNo] = PlayerActRound_PLAYER_ACT_NOT_ACTED
		}*/

	h.PlayersActed[seatChangedAction-1].State = state
	h.PlayersActed[seatChangedAction-1].Amount = amount
}

func (h *HandState) hasEveryOneActed() bool {
	allActed := true

	for seatNo := range h.GetPlayersInSeats() {
		state := h.PlayersActed[seatNo].State
		if state == PlayerActState_PLAYER_ACT_EMPTY_SEAT ||
			state == PlayerActState_PLAYER_ACT_FOLDED ||
			state == PlayerActState_PLAYER_ACT_ALL_IN {
			continue
		}

		// if big blind or straddle hasn't acted, return false
		if state == PlayerActState_PLAYER_ACT_BB || state == PlayerActState_PLAYER_ACT_STRADDLE || state == PlayerActState_PLAYER_ACT_NOT_ACTED {
			return false
		}

		if h.PlayersActed[seatNo].Amount == h.CurrentRaise {
			// if the player amount is same as current raise, action is complete for this player
			continue
		}

		allActed = false
		break
	}
	return allActed
}

func (h *HandState) getBlindPos() (uint32, uint32) {

	buttonSeat := uint32(h.GetButtonPos())
	smallBlindPos := h.getNextActivePlayer(buttonSeat)
	bigBlindPos := h.getNextActivePlayer(smallBlindPos)

	if smallBlindPos == 0 || bigBlindPos == 0 {
		// TODO: handle not enough players condition
		panic("Not enough players")
	}
	return uint32(smallBlindPos), uint32(bigBlindPos)
}

func (h *HandState) getPlayersCards(deck *poker.Deck) map[uint32][]byte {
	noOfCards := 2
	switch h.GetGameType() {
	case GameType_HOLDEM:
		noOfCards = 2
	case GameType_PLO:
		noOfCards = 4
	case GameType_PLO_HILO:
		noOfCards = 4
	}

	playerCards := make(map[uint32][]byte)
	for seatNoIdx, playerID := range h.GetPlayersInSeats() {
		if playerID != 0 {
			playerCards[uint32(seatNoIdx+1)] = make([]byte, 0, 4)
		}
	}

	for i := 0; i < noOfCards; i++ {
		seatNo := h.ButtonPos
		for {
			seatNo = h.getNextActivePlayer(seatNo)
			card := deck.Draw(1)
			h.DeckIndex++
			playerCards[seatNo] = append(playerCards[seatNo], card[0].GetByte())
			if seatNo == h.ButtonPos {
				// next round of cards
				break
			}
		}
		for j := uint32(1); j <= h.MaxSeats; j++ {

		}
	}

	return playerCards
}

func (h *HandState) moveButton() uint32 {
	seatNo := uint32(h.GetButtonPos())
	newButtonPos := h.getNextActivePlayer(seatNo)
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
func (h *HandState) getNextActivePlayer(seatNo uint32) uint32 {
	nextSeat := uint32(0)
	var j uint32
	for j = 1; j <= h.MaxSeats; j++ {
		seatNo++
		if seatNo > h.MaxSeats {
			seatNo = 1
		}

		playerID := h.GetPlayersInSeats()[seatNo-1]
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

func (h *HandState) actionReceived(action *HandAction) error {
	if h.NextSeatAction != nil {
		if action.SeatNo != h.NextSeatAction.SeatNo {
			handLogger.Error().
				Uint64("game", h.GetGameId()).
				Uint32("hand", h.GetHandNum()).
				Msg(fmt.Sprintf("Invalid seat %d made action. Ignored. The next valid action seat is: %d",
					action.SeatNo, h.NextSeatAction.SeatNo))
		}
	}

	// get player ID from the seat
	playerID := h.GetPlayersInSeats()[action.SeatNo-1]
	if playerID == 0 {
		// something wrong
		handLogger.Error().
			Uint64("game", h.GetGameId()).
			Uint32("hand", h.GetHandNum()).
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

		// track what round player folded the hand
		playerState.Status = HandPlayerState_FOLDED
		h.PlayersActed[action.SeatNo-1].State = PlayerActState_PLAYER_ACT_FOLDED
	} else if action.Action == ACTION_CHECK {
		h.PlayersActed[action.SeatNo-1].State = PlayerActState_PLAYER_ACT_CHECK
	} else if action.Action == ACTION_CALL {
		// action call
		// if this player has an equity in this pot, just call subtract the amount
		diff := h.CurrentRaise - playerBetSoFar
		h.PlayersActed[action.SeatNo-1].State = PlayerActState_PLAYER_ACT_CALL
		h.PlayersActed[action.SeatNo-1].Amount = action.Amount

		// does the player enough money ??
		if playerBalance < diff {
			// he is going all in, crazy
			action.Action = ACTION_ALLIN
			h.actionChanged(action.SeatNo, PlayerActState_PLAYER_ACT_ALL_IN, action.Amount)
			//h.PlayersActed[action.SeatNo-1].State = PlayerActState_PLAYER_ACT_ALL_IN
			h.PlayersActed[action.SeatNo-1].Amount = action.Amount
			h.AllInPlayers[action.SeatNo-1] = 1
			diff = playerBalance
		}
		playerBetSoFar += diff
		playerState.Balance -= diff
		bettingRound.SeatBet[action.SeatNo-1] = playerBetSoFar
	} else if action.Action == ACTION_ALLIN {
		h.AllInPlayers[action.SeatNo-1] = 1
		amount := playerState.Balance + playerBetSoFar
		bettingRound.SeatBet[action.SeatNo-1] = amount
		playerState.Balance = 0

		if amount > h.CurrentRaise {
			h.actionChanged(action.SeatNo, PlayerActState_PLAYER_ACT_ALL_IN, amount)
		}
	} else if action.Action == ACTION_RAISE ||
		action.Action == ACTION_BET {
		// TODO: we need to handle raise and determine next min raise
		if action.Amount < h.GetCurrentRaise() {
			// invalid
			handLogger.Error().
				Uint64("game", h.GetGameId()).
				Uint32("hand", h.GetHandNum()).
				Uint32("seat", action.SeatNo).
				Msg(fmt.Sprintf("Invalid raise %f. Current bet: %f", action.Amount, h.GetCurrentRaise()))
		}

		//h.PlayersActed[action.SeatNo-1] = PlayerActRound_PLAYER_ACT_ACTED
		//allin := false
		state := PlayerActState_PLAYER_ACT_RAISE
		if action.Action == ACTION_BET {
			state = PlayerActState_PLAYER_ACT_BET
		}
		if action.Action == ACTION_ALLIN {
			state = PlayerActState_PLAYER_ACT_ALL_IN
			// player is all in
			action.Amount = playerBetSoFar + playerState.Balance
		}

		if action.Amount > h.CurrentRaise {
			h.CurrentRaise = action.Amount

			// reset player action
			h.actionChanged(action.SeatNo, state, action.Amount)
			h.ActionCompleteAtSeat = action.SeatNo
		}

		// how much this user already had in the betting round
		diff := action.Amount - playerBetSoFar

		if diff == playerState.Balance {
			// player is all in
			action.Action = ACTION_ALLIN
			h.actionChanged(action.SeatNo, PlayerActState_PLAYER_ACT_ALL_IN, action.Amount)
			//h.PlayersActed[action.SeatNo-1] = PlayerActRound_PLAYER_ACT_ALL_IN
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
		h.NextSeatAction = h.prepareNextAction(action)
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

func (h *HandState) setupNextRound(state HandStatus) {
	h.CurrentState = state
	actionSeat := h.getNextActivePlayer(h.ButtonPos)
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
	nextAction.MinRaiseAmount = h.BigBlind
	nextAction.MaxRaiseAmount = h.BigBlind
	nextAction.AvailableActions = availableActions
	availableActions = append(availableActions, ACTION_ALLIN)
	nextAction.AllInAmount = playerState.Balance

	h.NextSeatAction = nextAction

	// track whether the player is active in this round or not
	for j := 1; j <= int(h.GetMaxSeats()); j++ {
		playerID := h.GetPlayersInSeats()[j-1]
		if playerID == 0 {
			continue
		}
		playerState, found := h.GetPlayersState()[playerID]
		if found {
			playerState.Round = state
		}
	}
}

func (h *HandState) setupFlop(board []uint32) {
	h.setupNextRound(HandStatus_FLOP)
	h.BoardCards = make([]byte, 3)
	for i, card := range board {
		h.BoardCards[i] = uint8(card)
	}
}

func (h *HandState) setupTurn(turnCard uint32) {
	h.setupNextRound(HandStatus_TURN)
	h.BoardCards = append(h.BoardCards, uint8(turnCard))
}

func (h *HandState) setupRiver(riverCard uint32) {
	h.setupNextRound(HandStatus_RIVER)
	h.BoardCards = append(h.BoardCards, uint8(riverCard))
}

func (h *HandState) prepareNextAction(currentAction *HandAction) *NextSeatAction {
	// compute next action
	actionSeat := h.getNextActivePlayer(currentAction.SeatNo)

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
			straddleAmount := 2.0 * h.BigBlind
			if playerState.Balance >= straddleAmount {
				availableActions = append(availableActions, ACTION_STRADDLE)
				nextAction.StraddleAmount = h.BigBlind * 2.0
			}
		}
	}

	allInAvailable := false
	canRaise := false
	canBet := false
	if h.CurrentRaise == 0.0 {
		// then the caller can check
		availableActions = append(availableActions, ACTION_CHECK)

		// the player can bet
		canBet = true
	} else {
		if playerState.Balance > h.CurrentRaise {
			actedState := h.PlayersActed[actionSeat-1].State
			if actedState == PlayerActState_PLAYER_ACT_BB ||
				actedState == PlayerActState_PLAYER_ACT_STRADDLE {
				availableActions = append(availableActions, ACTION_CHECK)
			} else {
				availableActions = append(availableActions, ACTION_CALL)
			}
			nextAction.CallAmount = h.CurrentRaise
			canRaise = true
		}
	}

	if canBet || canRaise {
		// then the caller call, raise, or go all in
		if playerState.Balance <= h.CurrentRaise || h.GameType == GameType_HOLDEM {
			allInAvailable = true
		}

		if canBet {
			// calculate what the maximum amount the player can bet
			availableActions = append(availableActions, ACTION_BET)
		}

		if canRaise {
			availableActions = append(availableActions, ACTION_RAISE)
			nextAction.MinRaiseAmount = h.CurrentRaise * 2
			// calculate the maximum amount the player can raise
		}

		if playerState.Balance < h.CurrentRaise*2 {
			// the player can go all in
			allInAvailable = true
		}
		if allInAvailable {
			availableActions = append(availableActions, ACTION_ALLIN)
			nextAction.AllInAmount = playerState.Balance
		}
	}

	nextAction.AvailableActions = availableActions

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

	// we will always take rake from the main pot for simplicity
	rakePaid := make(map[uint64]float32, 0)
	rake := float32(0.0)
	rakeFromPlayer := float32(0.0)
	mainPotWinners := potWinners[0]
	if h.RakePercentage > 0.0 {
		mainPot := h.Pots[0].Pot
		rake = float32(int(mainPot * (h.RakePercentage / 100)))
		if rake > h.RakeCap {
			rake = h.RakeCap
		}

		// take rake from main pot winners evenly
		winnersCount := len(mainPotWinners.HiWinners)
		if potWinners[0].LowWinners != nil {
			winnersCount = winnersCount + len(mainPotWinners.LowWinners)
		}

		rakeFromPlayer = float32(int(rake / float32(winnersCount)))
		if rakeFromPlayer == 0.0 {
			rakeFromPlayer = 1.0
		}
	}

	if float32(rakeFromPlayer) > 0.0 {
		totalRakeCollected := float32(0)
		for _, handWinner := range mainPotWinners.HiWinners {
			if totalRakeCollected == rake {
				break
			}
			seatNo := handWinner.SeatNo
			playerID := h.GetPlayersInSeats()[seatNo-1]
			if _, ok := rakePaid[playerID]; !ok {
				rakePaid[playerID] = float32(rakeFromPlayer)
			} else {
				rakePaid[playerID] += float32(rakeFromPlayer)
			}
			totalRakeCollected += rakeFromPlayer
			handWinner.Amount -= rakeFromPlayer
		}
		for _, handWinner := range mainPotWinners.LowWinners {
			if totalRakeCollected == rake {
				break
			}
			seatNo := handWinner.SeatNo
			playerID := h.GetPlayersInSeats()[seatNo-1]
			if _, ok := rakePaid[playerID]; !ok {
				rakePaid[playerID] = float32(rakeFromPlayer)
			} else {
				rakePaid[playerID] += float32(rakeFromPlayer)
			}
			totalRakeCollected += rakeFromPlayer
			handWinner.Amount -= rakeFromPlayer
		}
		h.RakePaid = rakePaid
		h.RakeCollected = totalRakeCollected
	}
	// update player balance
	for _, pot := range potWinners {
		for _, handWinner := range pot.HiWinners {
			seatNo := handWinner.SeatNo
			playerID := h.GetPlayersInSeats()[seatNo-1]
			h.PlayersState[playerID].Balance += handWinner.Amount
			h.PlayersState[playerID].PlayerReceived += handWinner.Amount
		}
		for _, handWinner := range pot.LowWinners {
			seatNo := handWinner.SeatNo
			playerID := h.GetPlayersInSeats()[seatNo-1]
			h.PlayersState[playerID].Balance += handWinner.Amount
			h.PlayersState[playerID].PlayerReceived += handWinner.Amount
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

func (h *HandState) getLog() *HandLog {
	handResult := &HandLog{}
	handResult.PotWinners = h.PotWinners
	handResult.WonAt = h.HandCompletedAt
	handResult.PreflopActions = h.PreflopActions
	handResult.FlopActions = h.FlopActions
	handResult.TurnActions = h.TurnActions
	handResult.RiverActions = h.RiverActions
	handResult.HandStartedAt = h.HandStartedAt
	handResult.HandEndedAt = h.HandEndedAt
	handResult.HandEndedAt = uint64(time.Now().Unix())
	return handResult
}
