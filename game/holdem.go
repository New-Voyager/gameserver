package game

import "voyager.com/server/poker"

type evaluatedCards struct {
	rank  int32
	cards []byte
}

func (e evaluatedCards) getCards() []uint32 {
	cards := make([]uint32, len(e.cards))
	for i := range e.cards {
		cards[i] = uint32(e.cards[i])
	}
	return cards
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
		potWinners.HiWinners = h.determineHandWinners(pot)
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
			evaluatedCards := h.activeSeatBestCombo[seatNo]
			rankStr := poker.RankString(evaluatedCards.rank)
			s := poker.CardsToString(evaluatedCards.cards)
			handWinners[i] = &HandWinner{SeatNo: seatNo, Amount: splitChips, WinningCards: evaluatedCards.getCards(), WinningCardsStr: s, RankStr: rankStr}
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

func (h *HoldemWinnerEvaluate) getEvaluatedCards() map[uint32]*evaluatedCards {
	return h.activeSeatBestCombo
}
