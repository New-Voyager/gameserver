package game

import (
	"voyager.com/server/poker"
)

func initializePot(maxSeats int) *SeatsInPots {
	// this will contain only the seats that are in the pot
	seats := make([]uint32, 0)
	return &SeatsInPots{
		Seats: seats,
		Pot:   0.0,
	}
}

func (s *SeatsInPots) add(seatNo uint32, amount float32) {
	found := false
	for i := range s.Seats {
		if seatNo == s.Seats[i] {
			// already in the pot
			found = true
			break
		}
	}
	if !found {
		s.Seats = append(s.Seats, seatNo)
	}
	s.Pot += amount
}

func (h *HandState) lowestBet(seatBets []float32) float32 {
	lowestBet := float32(0.0)
	for seatNo, bet := range seatBets {
		if h.PlayersInSeats[seatNo] == 0 {
			// empty seat
			continue
		}

		// also eliminate the player who is not active any longer
		if h.ActiveSeats[seatNo] == 0 {
			continue
		}
		if lowestBet == 0 {
			lowestBet = bet
		} else if bet < lowestBet {
			lowestBet = bet
		}
	}
	// if 0, every one checked or no more bets remaining
	return lowestBet
}

func (h *HandState) addChipsToPot(seatBets []float32, handEnded bool) {
	currentPotIndex := len(h.Pots) - 1
	currentPot := h.Pots[currentPotIndex]
	lowestBet := h.lowestBet(seatBets)
	for seatNoIdx, bet := range seatBets {
		if h.PlayersInSeats[seatNoIdx] == 0 || seatBets[seatNoIdx] == 0.0 {
			// empty seat
			continue
		}
		seatNo := seatNoIdx + 1
		if bet < lowestBet {
			// the player folded
			currentPot.add(uint32(seatNo), bet)
			seatBets[seatNoIdx] = 0
		} else {
			currentPot.add(uint32(seatNo), lowestBet)
			seatBets[seatNoIdx] = bet - lowestBet
		}
	}

	if handEnded {
		// put all remaining bets in the pot
		for seatNo, bet := range seatBets {
			if bet > 0.0 {
				currentPot.add(uint32(seatNo), bet)
			}
		}
	} else {
		anyRemainingBets := 0
		for _, bet := range seatBets {
			if bet > 0.0 {
				anyRemainingBets++
			}
		}
		if anyRemainingBets > 1 {
			// add a new pot and calculate next pot
			newPot := initializePot(len(seatBets))
			h.Pots = append(h.Pots, newPot)
			h.addChipsToPot(seatBets, handEnded)
		}
	}
}

type evaluatedCards struct {
	rank  int32
	cards []byte
}

type HoldemWinnerEvaluate struct {
	handState           *HandState
	gameState           *GameState
	activeSeatBestCombo map[uint32]*evaluatedCards
	winners             map[uint32]*PotWinners
}

func NewHoldemWinnerEvaluate(gameState *GameState, handState *HandState) *HoldemWinnerEvaluate {
	return &HoldemWinnerEvaluate{
		handState:           handState,
		gameState:           gameState,
		activeSeatBestCombo: make(map[uint32]*evaluatedCards, gameState.MaxSeats),
		winners:             make(map[uint32]*PotWinners),
	}
}

func (h *HoldemWinnerEvaluate) evaluate() {
	h.evaluatePlayerBestCards()
	for i := len(h.handState.Pots) - 1; i >= 0; i-- {
		pot := h.handState.Pots[i]
		potWinners := &PotWinners{}
		potWinners.HandWinner = h.determineHandWinners(pot)
		h.winners[uint32(i)] = potWinners
	}
}

func (h *HoldemWinnerEvaluate) determineHandWinners(pot *SeatsInPots) []*HandWinner {
	// determine the lowest ranking card first
	lowestRank := int32(0x7FFFFFFF)
	for _, seatNo := range pot.Seats {
		if _, ok := h.activeSeatBestCombo[seatNo]; !ok {
			continue
		}
		evaluation := h.activeSeatBestCombo[seatNo]
		if evaluation.rank != 0 && evaluation.rank < lowestRank {
			lowestRank = evaluation.rank
		}
	}

	noOfWinners := 0
	for _, seatNo := range pot.Seats {
		if _, ok := h.activeSeatBestCombo[seatNo]; !ok {
			continue
		}

		if h.activeSeatBestCombo[seatNo].rank == lowestRank {
			noOfWinners++
		}
	}
	splitChips := pot.Pot / float32(noOfWinners)
	handWinners := make([]*HandWinner, noOfWinners)
	i := 0
	for _, seatNo := range pot.Seats {
		if _, ok := h.activeSeatBestCombo[seatNo]; !ok {
			continue
		}

		if h.activeSeatBestCombo[seatNo].rank == lowestRank {
			handWinners[i] = &HandWinner{SeatNo: seatNo, Amount: splitChips}
			i++
		}
	}

	return handWinners
}

func (h *HoldemWinnerEvaluate) evaluatePlayerBestCards() {
	// determine rank for each active player
	for seatNoIdx, active := range h.handState.ActiveSeats {
		if active == 0 {
			continue
		}
		seatCards := h.handState.PlayersCards[uint32(seatNoIdx+1)]
		allCards := make([]byte, len(h.handState.BoardCards))
		copy(allCards, h.handState.BoardCards)
		allCards = append(allCards, seatCards...)
		cards := poker.FromByteCards(allCards)
		rank, playerBestCards := poker.Evaluate(cards)
		h.activeSeatBestCombo[uint32(seatNoIdx+1)] = &evaluatedCards{
			rank:  rank,
			cards: poker.CardsToByteCards(playerBestCards),
		}
	}
}
