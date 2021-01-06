package game

import (
	"fmt"
	"math"
	"time"

	"github.com/rs/zerolog/log"
	"voyager.com/server/poker"
)

var handLogger = log.With().Str("logger_name", "game::hand").Logger()
var preFlopBets = []int{3, 5, 10}     // big blinds
var postFlopBets = []int{30, 50, 100} // % of pot
var raiseOptions = []int{2, 3, 5}     // raise times
var ploPreFlopBets = []int{2}         // big blinds

func LoadHandState(handStatePersist PersistHandState, clubID uint32, gameID uint64, handNum uint32) (*HandState, error) {
	handState, err := handStatePersist.Load(clubID, gameID, handNum)
	if err != nil {
		return nil, err
	}

	return handState, nil
}

func (h *HandState) initializeBettingRound() {
	maxSeats := h.MaxSeats + 1 // dealer seat
	h.RoundState = make(map[uint32]*RoundState)
	//h.RoundBetting = make(map[uint32]*SeatBetting)
	h.RoundState[uint32(HandStatus_PREFLOP)] = &RoundState{
		PlayerBalance: make(map[uint32]float32, 0),
		Betting:       &SeatBetting{SeatBet: make([]float32, maxSeats)},
	}
	h.RoundState[uint32(HandStatus_FLOP)] = &RoundState{
		PlayerBalance: make(map[uint32]float32, 0),
		Betting:       &SeatBetting{SeatBet: make([]float32, maxSeats)},
	}
	h.RoundState[uint32(HandStatus_TURN)] = &RoundState{
		PlayerBalance: make(map[uint32]float32, 0),
		Betting:       &SeatBetting{SeatBet: make([]float32, maxSeats)},
	}
	h.RoundState[uint32(HandStatus_RIVER)] = &RoundState{
		PlayerBalance: make(map[uint32]float32, 0),
		Betting:       &SeatBetting{SeatBet: make([]float32, maxSeats)},
	}

	// setup player acted tracking
	h.PlayersActed = make([]*PlayerActRound, maxSeats) // seat 0 is dealer
	h.resetPlayerActions()
}

func (h *HandState) initialize(gameState *GameState, deck *poker.Deck, buttonPos uint32, moveButton bool) {
	// settle players in the seats
	h.PlayersInSeats = make([]uint64, gameState.MaxSeats+1) // seat 0 is dealer
	h.NoActiveSeats = 0
	h.GameType = gameState.GameType

	// copy player's stack (we need to copy only the players that are in the hand)
	h.PlayersState = h.copyPlayersState(gameState)

	// update active seats with players who are playing
	for seatNo, playerID := range gameState.GetPlayersInSeats() {
		if seatNo == 0 { // dealer seat
			continue
		}
		h.PlayersInSeats[seatNo] = 0
		if playerID != 0 {
			// get player state
			state := h.PlayersState[playerID]
			if state == nil || state.Balance == 0 {
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
	h.PlayersActed = make([]*PlayerActRound, h.MaxSeats+1)
	h.BringIn = gameState.BringIn

	// if the players don't have money less than the blinds
	// don't let them play
	h.ActiveSeats = h.GetPlayersInSeats()

	// determine button and blinds
	if moveButton {
		h.ButtonPos = h.moveButton()
	}

	// TODO: make sure small blind is still there
	// if small blind left the game, we can have dead small
	// to make it simple, we will make new players to always to post or wait for the big blind
	h.SmallBlindPos, h.BigBlindPos = h.getBlindPos()

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

	if deck == nil || deck.Empty() {
		deck = poker.NewDeck(nil).Shuffle()
	}

	h.Deck = deck.GetBytes()
	h.PlayersCards = h.getPlayersCards(deck)

	// setup main pot
	h.Pots = make([]*SeatsInPots, 0)
	mainPot := initializePot(int(gameState.MaxSeats + 1))
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
	for seatNo := 1; seatNo <= int(gameState.GetMaxSeats()); seatNo++ {
		playerID := gameState.GetPlayersInSeats()[seatNo]
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

func (h *HandState) setupRound(state HandStatus) {
	h.RoundState[uint32(state)].PlayerBalance = make(map[uint32]float32, 0)
	roundState := h.RoundState[uint32(state)]
	for seatNo, playerID := range h.PlayersInSeats {
		if seatNo == 0 || playerID == 0 {
			continue
		}
		state := h.PlayersState[playerID]
		roundState.PlayerBalance[uint32(seatNo)] = state.Balance
	}
}

func (h *HandState) setupPreflob() {
	h.CurrentState = HandStatus_PREFLOP

	// set next action information
	h.PreflopActions = &HandActionLog{Actions: make([]*HandAction, 0)}
	h.FlopActions = &HandActionLog{Actions: make([]*HandAction, 0)}
	h.TurnActions = &HandActionLog{Actions: make([]*HandAction, 0)}
	h.RiverActions = &HandActionLog{Actions: make([]*HandAction, 0)}
	h.CurrentRaise = 0
	// initialize all-in players list
	h.AllInPlayers = make([]uint32, h.MaxSeats+1) // seat 0 is dealer
	h.setupRound(HandStatus_PREFLOP)

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

	h.ActionCompleteAtSeat = h.BigBlindPos

	// track whether the player is active in this round or not
	for seatNo := 1; seatNo <= int(h.GetMaxSeats()); seatNo++ {
		playerID := h.GetPlayersInSeats()[seatNo]
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
		// if player is all in, then don't reset
		if h.PlayersActed[seatNo].GetState() == PlayerActState_PLAYER_ACT_ALL_IN {
			continue
		}
		h.PlayersActed[seatNo] = &PlayerActRound{
			State: PlayerActState_PLAYER_ACT_NOT_ACTED,
		}
	}
}

func (h *HandState) acted(seatChangedAction uint32, state PlayerActState, amount float32) {
	h.PlayersActed[seatChangedAction].State = state
	if state == PlayerActState_PLAYER_ACT_FOLDED {
		h.ActiveSeats[seatChangedAction] = 0
		h.NoActiveSeats--
	} else {
		h.PlayersActed[seatChangedAction].Amount = amount
		if amount > h.CurrentRaise {
			h.PlayersActed[seatChangedAction].RaiseAmount = amount - h.CurrentRaise
		} else {
			h.PlayersActed[seatChangedAction].RaiseAmount = h.CurrentRaiseDiff
		}
	}
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
	case GameType_FIVE_CARD_PLO:
		noOfCards = 5
	case GameType_FIVE_CARD_PLO_HILO:
		noOfCards = 5
	}

	playerCards := make(map[uint32][]byte)
	for seatNo, playerID := range h.GetPlayersInSeats() {
		if playerID != 0 {
			playerCards[uint32(seatNo)] = make([]byte, 0, 4)
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

		playerID := h.GetPlayersInSeats()[seatNo]
		// check to see whether the player is playing or sitting out
		if playerID == 0 {
			continue
		}

		if state, ok := h.GetPlayersState()[playerID]; ok {
			if state.Status != HandPlayerState_ACTIVE {
				continue
			}
		}

		if h.ActiveSeats[seatNo] == 0 {
			continue
		}

		// skip the all-in player
		if h.PlayersActed[seatNo].GetState() == PlayerActState_PLAYER_ACT_ALL_IN {
			continue
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
	playerID := h.GetPlayersInSeats()[action.SeatNo]
	if playerID == 0 {
		// something wrong
		handLogger.Error().
			Uint64("game", h.GetGameId()).
			Uint32("hand", h.GetHandNum()).
			Uint32("seat", action.SeatNo).
			Msg(fmt.Sprintf("Invalid seat %d. PlayerID is 0", action.SeatNo))
	}

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
	bettingState := h.RoundState[uint32(h.CurrentState)]
	bettingRound := bettingState.Betting
	playerBalance := bettingState.PlayerBalance[action.SeatNo]
	playerBetSoFar := bettingRound.SeatBet[action.SeatNo]
	diff := float32(0)

	// the next player after big blind can straddle
	straddleAvailable := false
	if action.Action == ACTION_BB {
		straddleAvailable = true
	}

	// if player has less than the blinds, then this player will go all-in
	if action.Action == ACTION_BB || action.Action == ACTION_SB {
		if playerBalance < action.Amount {
			action.Action = ACTION_ALLIN
			action.Amount = playerBalance
			h.acted(action.SeatNo, PlayerActState_PLAYER_ACT_ALL_IN, action.Amount)
		}
		bettingRound.SeatBet[int(action.SeatNo)] = action.Amount
	}

	// valid actions
	if action.Action == ACTION_FOLD {
		// track what round player folded the hand
		h.acted(action.SeatNo, PlayerActState_PLAYER_ACT_FOLDED, playerBetSoFar)
	} else if action.Action == ACTION_CHECK {
		h.PlayersActed[action.SeatNo].State = PlayerActState_PLAYER_ACT_CHECK
	} else if action.Action == ACTION_CALL {
		// action call
		if action.Amount < h.CurrentRaise {
			// fold this player
			h.acted(action.SeatNo, PlayerActState_PLAYER_ACT_FOLDED, playerBetSoFar)
		} else if action.Amount == playerBalance {
			// the player is all in
			action.Action = ACTION_ALLIN
			h.acted(action.SeatNo, PlayerActState_PLAYER_ACT_ALL_IN, action.Amount)
			h.AllInPlayers[action.SeatNo] = 1
			diff = playerBalance
		} else {
			// if this player has an equity in this pot, just call subtract the amount
			h.acted(action.SeatNo, PlayerActState_PLAYER_ACT_CALL, action.Amount)
		}
		diff = (action.Amount - playerBetSoFar)
		//bettingState.PlayerBalance[action.SeatNo] -= additionalBet
		playerBetSoFar += diff
		bettingRound.SeatBet[action.SeatNo] = playerBetSoFar
	} else if action.Action == ACTION_ALLIN {
		h.AllInPlayers[action.SeatNo] = 1
		amount := bettingState.PlayerBalance[action.SeatNo] + playerBetSoFar
		bettingRound.SeatBet[action.SeatNo] = amount
		diff = playerBalance
		//bettingState.PlayerBalance[action.SeatNo] = 0
		action.Amount = amount
		h.acted(action.SeatNo, PlayerActState_PLAYER_ACT_ALL_IN, amount)
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

		state := PlayerActState_PLAYER_ACT_RAISE
		if action.Action == ACTION_BET {
			state = PlayerActState_PLAYER_ACT_BET
		}
		if action.Action == ACTION_ALLIN {
			state = PlayerActState_PLAYER_ACT_ALL_IN
			// player is all in
			action.Amount = playerBetSoFar + bettingState.PlayerBalance[action.SeatNo]
		}

		if action.Amount > h.CurrentRaise {
			// reset player action
			h.acted(action.SeatNo, state, action.Amount)
			h.ActionCompleteAtSeat = action.SeatNo
		}

		// how much this user already had in the betting round
		diff = action.Amount - playerBetSoFar

		if diff == bettingState.PlayerBalance[action.SeatNo] {
			// player is all in
			action.Action = ACTION_ALLIN
			h.acted(action.SeatNo, PlayerActState_PLAYER_ACT_ALL_IN, action.Amount)
			h.AllInPlayers[action.SeatNo] = 1
			bettingState.PlayerBalance[action.SeatNo] = 0
		}
		//else {
		//	bettingState.PlayerBalance[action.SeatNo] -= diff
		//}

		bettingRound.SeatBet[action.SeatNo] = action.Amount
	} else if action.Action == ACTION_SB ||
		action.Action == ACTION_BB ||
		action.Action == ACTION_STRADDLE {
		bettingRound.SeatBet[action.SeatNo] = action.Amount
		switch action.Action {
		case ACTION_BB:
			h.acted(action.SeatNo, PlayerActState_PLAYER_ACT_BB, action.Amount)
		case ACTION_STRADDLE:
			h.acted(action.SeatNo, PlayerActState_PLAYER_ACT_STRADDLE, action.Amount)
		}
		diff = action.Amount
	}

	if action.Amount > h.CurrentRaise {
		h.BetBeforeRaise = h.CurrentRaise
		h.CurrentRaiseDiff = action.Amount - h.CurrentRaise
		if h.CurrentState == HandStatus_PREFLOP && h.CurrentRaiseDiff < h.BigBlind {
			h.CurrentRaiseDiff = h.BigBlind
			h.BetBeforeRaise = 0
		}
		h.CurrentRaise = action.Amount
		h.ActionCompleteAtSeat = action.SeatNo
	}
	bettingState.PlayerBalance[action.SeatNo] = bettingState.PlayerBalance[action.SeatNo] - diff
	// add the action to the log
	log.Actions = append(log.Actions, action)
	log.Pot = log.Pot + diff

	// check whether everyone has acted in this ROUND
	// or everyone except folded in this hand
	if h.hasEveryOneActed() || h.NoActiveSeats == 1 {
		// settle this round and move to next round
		h.settleRound()
		// next seat action will be determined outside of here
		// after moving to next round
		h.NextSeatAction = nil
	} else {
		actionSeat := h.getNextActivePlayer(action.SeatNo)
		if action.Action != ACTION_SB {
			h.NextSeatAction = h.prepareNextAction(actionSeat, straddleAvailable)
		}
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
	playerID := h.GetPlayersInSeats()[seatNo]
	if playerID == 0 {
		return nil
	}
	playerState, _ := h.PlayersState[playerID]
	return playerState
}

func (h *HandState) isAllActivePlayersAllIn() bool {
	allIn := true
	noAllInPlayers := 0
	noActivePlayers := 0
	for seatNo, playerID := range h.ActiveSeats {
		if playerID == 0 {
			continue
		}
		noActivePlayers++
		if h.AllInPlayers[seatNo] == 0 {
			allIn = false
		} else {
			noAllInPlayers++
		}
	}
	if allIn {
		return allIn
	}

	// if all players all-in, but one player is remaining, then there will no more actions
	if noActivePlayers == noAllInPlayers+1 {
		return true
	}

	return false
}

func (h *HandState) settleRound() {
	// before we go to next stage, settle pots
	bettingState := h.RoundState[uint32(h.CurrentState)]
	currentBettingRound := bettingState.Betting

	// update player state
	for seatNo, bet := range currentBettingRound.SeatBet {
		playerID := h.PlayersInSeats[seatNo]
		if playerID == 0 {
			continue
		}
		h.PlayersState[playerID].Balance -= bet
	}

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
		if seatBets[1] < seatBets[2] {
			maxBet = seatBets[2]
			secondMaxBet = seatBets[1]
			maxBetPos = 2
		} else {
			maxBet = seatBets[1]
			secondMaxBet = seatBets[2]
			maxBetPos = 1
		}
		for seat := 3; seat < len(seatBets); seat++ {
			if h.ActiveSeats[seat] == 0 {
				continue
			}
			bet := seatBets[seat]
			if bet > maxBet {
				secondMaxBet = maxBet
				maxBet = bet
				maxBetPos = seat
			} else if bet < maxBet {
				secondMaxBet = bet
			}
		}
		if maxBet != 0 && secondMaxBet != 0 && maxBetPos > 0 {
			playerID := h.PlayersInSeats[maxBetPos]
			h.PlayersState[playerID].Balance += (maxBet - secondMaxBet)
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

func (h *HandState) removeFoldedPlayersFromPots() {
	for _, pot := range h.Pots {
		updateSeats := make([]uint32, 0)
		for _, seat := range pot.Seats {
			if h.PlayersActed[seat].GetState() == PlayerActState_PLAYER_ACT_FOLDED {
				// skip this player
			} else {
				updateSeats = append(updateSeats, seat)
			}
		}
		pot.Seats = updateSeats
	}
}

func (h *HandState) removeEmptyPots() {
	pots := make([]*SeatsInPots, 0)
	for _, pot := range h.Pots {
		if len(pot.Seats) == 0 || pot.Pot == 0 {
			continue
		}
		pots = append(pots, pot)
	}
	h.Pots = pots
}

func (h *HandState) setupNextRound(state HandStatus) {
	h.CurrentState = state
	h.resetPlayerActions()
	h.CurrentRaise = 0
	h.CurrentRaiseDiff = 0
	h.BetBeforeRaise = 0
	h.setupRound(state)
	actionSeat := h.getNextActivePlayer(h.ButtonPos)
	if actionSeat == 0 {
		// every one is all in
		return
	}
	playerState := h.getPlayerFromSeat(actionSeat)
	if playerState == nil {
		// something wrong
		panic("Something went wrong. player id cannot be null")
	}
	h.NextSeatAction = h.prepareNextAction(actionSeat, false)

	// track whether the player is active in this round or not
	for seatNo := 1; seatNo <= int(h.GetMaxSeats()); seatNo++ {
		playerID := h.GetPlayersInSeats()[seatNo]
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

func (h *HandState) adjustToBringIn(amount float32) float32 {
	if h.BringIn != 0 {
		// make call amount multiples of bring-in
		if int64(amount)%int64(h.BringIn) > 0 {
			amount = float32(float32(int64(amount/h.BringIn+1.0)) * h.BringIn)
			amount = float32(math.Floor(float64(amount)))
		}
	}
	return amount
}

func (h *HandState) calcPloPotBet(callAmount float32, preFlop bool) float32 {
	roundState := h.RoundState[uint32(h.CurrentState)]
	bettingRound := roundState.Betting
	firstAction := false
	if preFlop {
		firstAction = true
		for _, bet := range bettingRound.SeatBet {
			if bet > h.BigBlind {
				firstAction = false
				break
			}
		}
	}

	totalPot := callAmount
	if !firstAction {
		totalPot = callAmount * 2.0
	}
	for _, pot := range h.Pots {
		totalPot += pot.Pot
	}

	for _, bet := range bettingRound.SeatBet {
		// if there is no call or bet, then consider the blinds as bring-in bets
		if firstAction && bet != 0 && bet < h.BringIn {
			bet = h.BringIn
		}
		totalPot += bet
	}

	if h.BringIn != 0 {
		// make call amount multiples of bring-in
		totalPot = h.adjustToBringIn(totalPot)
	}
	return totalPot
}

func (h *HandState) prepareNextAction(actionSeat uint32, straddleAvailable bool) *NextSeatAction {
	// compute next action
	bettingState := h.RoundState[uint32(h.CurrentState)]
	bettingRound := bettingState.Betting

	playerBalance := bettingState.PlayerBalance[actionSeat]
	nextAction := &NextSeatAction{SeatNo: actionSeat}
	availableActions := make([]ACTION, 0)
	availableActions = append(availableActions, ACTION_FOLD)
	if h.CurrentState == HandStatus_PREFLOP {
		if straddleAvailable {
			// the player can straddle, if he has enough money
			straddleAmount := 2.0 * h.BigBlind
			if playerBalance >= straddleAmount {
				availableActions = append(availableActions, ACTION_STRADDLE)
				nextAction.StraddleAmount = h.BigBlind * 2.0
			}
		}
	}

	allInAvailable := false
	canRaise := false
	canBet := false

	// then the caller call, raise, or go all in
	if playerBalance <= h.CurrentRaise || h.GameType == GameType_HOLDEM {
		allInAvailable = true
	}

	if h.CurrentRaise == 0.0 {
		// then the caller can check
		availableActions = append(availableActions, ACTION_CHECK)

		// the player can bet
		canBet = true
	} else {
		if playerBalance > h.CurrentRaise {
			actedState := h.PlayersActed[actionSeat]

			if (actedState.State == PlayerActState_PLAYER_ACT_BB && h.CurrentRaise > h.BigBlind) ||
				(actedState.State == PlayerActState_PLAYER_ACT_STRADDLE && h.CurrentRaise > h.Straddle) {
				availableActions = append(availableActions, ACTION_CALL)
			} else if actedState.Amount == h.CurrentRaise {
				availableActions = append(availableActions, ACTION_CHECK)
			} else {
				availableActions = append(availableActions, ACTION_CALL)
			}
			nextAction.CallAmount = h.CurrentRaiseDiff + h.BetBeforeRaise

			canRaise = true
			// if the call amount is less than bring in amount, use bring in amount
			if h.CurrentState == HandStatus_PREFLOP {
				if nextAction.CallAmount <= h.BringIn {
					nextAction.CallAmount = h.BringIn
					canBet = true
					canRaise = false
				}
				if h.BetBeforeRaise == 0 {
					canBet = true
					canRaise = false
				}
			}
		}
	}

	playerPrevAction := h.PlayersActed[actionSeat]
	if playerPrevAction.GetState() == PlayerActState_PLAYER_ACT_CALL ||
		playerPrevAction.GetState() == PlayerActState_PLAYER_ACT_BET ||
		playerPrevAction.GetState() == PlayerActState_PLAYER_ACT_RAISE {
		if playerPrevAction.RaiseAmount > h.CurrentRaiseDiff {
			// this player cannot raise
			canRaise = false
			allInAvailable = false
		}
	}

	allIn := bettingRound.SeatBet[actionSeat] + playerBalance
	if canBet || canRaise {
		playerID := h.GetPlayersInSeats()[actionSeat]
		betOptions := make([]*BetRaiseOption, 0)
		if h.CurrentRaiseDiff > 0 {
			nextAction.MinRaiseAmount = (h.CurrentRaiseDiff * 2) + h.BetBeforeRaise
		}
		// at preflop, the min raise should be twice than the bringin amount
		nextAction.MinRaiseAmount = (h.CurrentRaiseDiff * 2) + h.BetBeforeRaise
		if nextAction.MinRaiseAmount == 0 {
			nextAction.MinRaiseAmount = h.BigBlind
		}
		if h.BringIn > 0.0 {
			if h.CurrentState == HandStatus_PREFLOP && nextAction.MinRaiseAmount < 2.0*h.BringIn {
				nextAction.MinRaiseAmount = 2.0 * h.BringIn
			}
		}
		nextAction.MinRaiseAmount = h.adjustToBringIn(nextAction.MinRaiseAmount)

		if h.GameType == GameType_HOLDEM {
			nextAction.MaxRaiseAmount = bettingRound.SeatBet[actionSeat] + playerBalance
		} else {
			preFlop := h.CurrentState == HandStatus_PREFLOP
			// handle PLO max raise
			ploPot := h.calcPloPotBet(nextAction.CallAmount, preFlop)
			nextAction.MaxRaiseAmount = ploPot
			if ploPot > allIn {
				nextAction.MaxRaiseAmount = allIn
			}
		}

		if nextAction.MaxRaiseAmount >= allIn {
			nextAction.MaxRaiseAmount = allIn
		}

		if canBet {
			// calculate what the maximum amount the player can bet
			availableActions = append(availableActions, ACTION_BET)
			betOptions = h.betOptions(actionSeat, h.CurrentState, playerID, nextAction.CallAmount)
		}

		if canRaise {
			// calculate what the maximum amount the player can bet
			if h.CurrentState == HandStatus_PREFLOP && h.CurrentRaise == h.BigBlind {
				betOptions = h.betOptions(actionSeat, h.CurrentState, playerID, nextAction.CallAmount)
				availableActions = append(availableActions, ACTION_BET)
			} else {
				if allIn > nextAction.MinRaiseAmount {
					// calculate the maximum amount the player can raise
					availableActions = append(availableActions, ACTION_RAISE)
					betOptions = h.raiseOptions(actionSeat, nextAction.MinRaiseAmount, nextAction.MaxRaiseAmount, playerID)
				} else {
					// this player can go only all-in to raise
					nextAction.MinRaiseAmount = 0
					nextAction.MaxRaiseAmount = 0
				}
			}
		}

		if h.GameType == GameType_HOLDEM || playerBalance < nextAction.MinRaiseAmount {
			// the player can go all in no limit holdem
			allInAvailable = true
		}
		nextAction.BetOptions = betOptions
	}
	if allInAvailable {
		availableActions = append(availableActions, ACTION_ALLIN)
		nextAction.AllInAmount = allIn
	}

	if nextAction.MaxRaiseAmount == allIn {
		nextAction.AllInAmount = allIn
	}
	nextAction.AvailableActions = availableActions

	return nextAction
}

func (h *HandState) everyOneFoldedWinners() {
	seatNo := 0
	for i, playerID := range h.ActiveSeats {
		if playerID != 0 {
			seatNo = i
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
			playerID := h.GetPlayersInSeats()[seatNo]
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
			playerID := h.GetPlayersInSeats()[seatNo]
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
			playerID := h.GetPlayersInSeats()[seatNo]
			h.PlayersState[playerID].Balance += handWinner.Amount
			h.PlayersState[playerID].PlayerReceived += handWinner.Amount
		}
		for _, handWinner := range pot.LowWinners {
			seatNo := handWinner.SeatNo
			playerID := h.GetPlayersInSeats()[seatNo]
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
			&PlayerBalance{SeatNo: uint32(seatNo), PlayerId: player, Balance: state.Balance})
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

func (h *HandState) betOptions(seatNo uint32, round HandStatus, playerID uint64, callAmount float32) []*BetRaiseOption {
	preFlopBetOptions := preFlopBets
	postFlopBets := postFlopBets
	roundState := h.RoundState[uint32(h.CurrentState)]
	bettingRound := roundState.Betting
	balance := roundState.PlayerBalance[seatNo]
	allIn := bettingRound.SeatBet[seatNo] + balance

	options := make([]*BetRaiseOption, 0)
	if h.GameType == GameType_HOLDEM {
		if round == HandStatus_PREFLOP {
			// pre-flop options
			for _, betOption := range preFlopBetOptions {
				betAmount := float32(int64(float32(betOption) * h.BigBlind))
				if betAmount < balance {
					option := &BetRaiseOption{
						Text:   fmt.Sprintf("%dBB", betOption),
						Amount: betAmount,
					}
					options = append(options, option)
				}
			}
			options = append(options, &BetRaiseOption{Text: "All-In", Amount: allIn})
		} else if round >= HandStatus_FLOP {
			totalPot := float32(0.0)
			for _, pot := range h.Pots {
				totalPot += pot.Pot
			}
			// post-flop options
			for _, betOption := range postFlopBets {
				betAmount := float32(int64(float32(betOption)*totalPot) / 100.0)
				if betAmount > h.BigBlind && betAmount < balance {
					option := &BetRaiseOption{
						Text:   fmt.Sprintf("%d%%", betOption),
						Amount: float32(betAmount),
					}
					options = append(options, option)
				}
			}
			options = append(options, &BetRaiseOption{Text: "All-In", Amount: allIn})
		}
	} else {
		// PLO
		if round == HandStatus_PREFLOP {
			preFlop := h.CurrentState == HandStatus_PREFLOP
			ploPot := h.calcPloPotBet(callAmount, preFlop)
			// pre-flop options
			for _, betOption := range ploPreFlopBets {
				bet := h.BigBlind
				if bet < h.BringIn {
					bet = h.BringIn
				}

				betAmount := float32(int64(float32(betOption) * bet))
				if betAmount > ploPot {
					betAmount = ploPot
				}
				betAmount = h.adjustToBringIn(betAmount)

				if betAmount < balance {
					option := &BetRaiseOption{
						Text:   fmt.Sprintf("%dBB", betOption),
						Amount: betAmount,
					}
					options = append(options, option)
				}
			}
			if ploPot > allIn {
				options = append(options, &BetRaiseOption{Text: "All-In", Amount: allIn})
			} else {
				options = append(options, &BetRaiseOption{Text: "Pot", Amount: ploPot})
			}
		} else {
			preFlop := h.CurrentState == HandStatus_PREFLOP
			// post flop
			ploPot := h.calcPloPotBet(callAmount, preFlop)
			// pre-flop options
			for _, betOption := range postFlopBets {
				// skip 100%
				if betOption == 100 {
					continue
				}
				bet := h.BigBlind
				if bet < h.BringIn {
					bet = h.BringIn
				}

				betAmount := float32(int64(float32(betOption)*ploPot) / 100.0)
				if betAmount > ploPot {
					betAmount = ploPot
				}

				if h.BringIn != 0.0 && betAmount < h.BringIn {
					// skip this option
					continue
				}
				betAmount = h.adjustToBringIn(betAmount)

				if betAmount < balance {
					option := &BetRaiseOption{
						Text:   fmt.Sprintf("%d%%", betOption),
						Amount: betAmount,
					}
					options = append(options, option)
				}
			}
			if ploPot > allIn {
				options = append(options, &BetRaiseOption{Text: "All-In", Amount: allIn})
			} else {
				options = append(options, &BetRaiseOption{Text: "Pot", Amount: ploPot})
			}
		}
	}
	return options
}

func (h *HandState) raiseOptions(seatNo uint32, minRaiseAmount float32, maxRaiseAmount float32, playerID uint64) []*BetRaiseOption {
	roundState := h.RoundState[uint32(h.CurrentState)]
	balance := roundState.PlayerBalance[seatNo]
	bettingRound := roundState.Betting
	allIn := bettingRound.SeatBet[seatNo] + balance

	options := make([]*BetRaiseOption, 0)
	if h.GameType == GameType_HOLDEM {
		for _, betOption := range raiseOptions {
			betAmount := float32(int64(float32(betOption) * minRaiseAmount))
			if betAmount < balance {
				option := &BetRaiseOption{
					Text:   fmt.Sprintf("%dx", betOption),
					Amount: betAmount,
				}
				options = append(options, option)
			}
		}
		options = append(options, &BetRaiseOption{Text: "All-In", Amount: allIn})
	} else {
		for _, betOption := range raiseOptions {
			betAmount := float32(int64(float32(betOption) * minRaiseAmount))
			betAmount = h.adjustToBringIn(betAmount)
			if betAmount < maxRaiseAmount {
				option := &BetRaiseOption{
					Text:   fmt.Sprintf("%dx", betOption),
					Amount: betAmount,
				}
				options = append(options, option)
			}
		}
		if allIn <= maxRaiseAmount {
			options = append(options, &BetRaiseOption{Text: "All-In", Amount: allIn})
		} else {
			options = append(options, &BetRaiseOption{Text: "Pot", Amount: maxRaiseAmount})
		}
	}
	return options
}
