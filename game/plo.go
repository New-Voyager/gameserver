package game

import "voyager.com/server/poker"

type PloWinnerEvaluate struct {
	handState           *HandState
	gameState           *GameState
	activeSeatBestCombo map[uint32]*EvaluatedCards
	winners             map[uint32]*PotWinners
	highHandCombo       map[uint32]*EvaluatedCards
	includeHighHand     bool
}

func NewPloWinnerEvaluate(gameState *GameState, handState *HandState, includeHighHand bool) *PloWinnerEvaluate {
	return &PloWinnerEvaluate{
		handState:           handState,
		gameState:           gameState,
		activeSeatBestCombo: make(map[uint32]*EvaluatedCards, gameState.MaxSeats),
		winners:             make(map[uint32]*PotWinners),
		highHandCombo:       make(map[uint32]*EvaluatedCards, gameState.MaxSeats),
		includeHighHand:     includeHighHand,
	}
}

func (h *PloWinnerEvaluate) GetWinners() map[uint32]*PotWinners {
	return h.winners
}

func (h *PloWinnerEvaluate) Evaluate() {
	h.evaluatePlayerBestCards()
	for i := len(h.handState.Pots) - 1; i >= 0; i-- {
		pot := h.handState.Pots[i]
		potWinners := &PotWinners{}
		potWinners.HiWinners = h.determineHandWinners(pot)
		h.winners[uint32(i)] = potWinners
	}

	if h.includeHighHand {
		h.evaluatePlayerHighHand()
	}
}

func (h *PloWinnerEvaluate) determineHandWinners(pot *SeatsInPots) []*HandWinner {
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
			handWinners[i] = &HandWinner{
				SeatNo:          seatNo,
				Amount:          splitChips,
				WinningCards:    evaluatedCards.GetCards(),
				WinningCardsStr: s,
				RankStr:         rankStr,
				BoardCards:      evaluatedCards.GetBoardCards(),
				PlayerCards:     evaluatedCards.GetPlayerCards(),
			}
			i++
		}
	}

	return handWinners
}

func (h *PloWinnerEvaluate) evaluatePlayerBestCards() {
	// determine rank for each active player
	for seatNoIdx, active := range h.handState.ActiveSeats {
		if active == 0 {
			continue
		}
		seatCards := h.handState.PlayersCards[uint32(seatNoIdx+1)]
		playerCardsEval := poker.FromByteCards(seatCards)
		boardCardsEval := poker.FromByteCards(h.handState.BoardCards)
		result := poker.EvaluateOmaha(playerCardsEval, boardCardsEval)

		// determine what player cards and board cards used to determine best cards
		seatCardsInCard := poker.FromByteCards(seatCards)
		playerCards := make([]poker.Card, 0)
		boardCards := make([]poker.Card, 0)
		for _, card := range result.HiCards {
			isPlayerCard := false
			for _, playerCard := range seatCardsInCard {
				if playerCard == card {
					isPlayerCard = true
					break
				}
			}
			if isPlayerCard {
				playerCards = append(playerCards, card)
			} else {
				boardCards = append(boardCards, card)
			}
		}
		h.activeSeatBestCombo[uint32(seatNoIdx+1)] = &EvaluatedCards{
			rank:        result.HiRank,
			cards:       poker.CardsToByteCards(result.HiCards),
			playerCards: poker.CardsToByteCards(playerCards),
			boardCards:  poker.CardsToByteCards(boardCards),
		}
	}
}

func (h *PloWinnerEvaluate) evaluatePlayerHighHand() {
	// determine rank for each active player
	for seatNoIdx, active := range h.handState.ActiveSeats {
		if active == 0 {
			continue
		}
		seatNo := uint32(seatNoIdx + 1)
		highHand := h.activeSeatBestCombo[seatNo]
		h.highHandCombo[uint32(seatNoIdx+1)] = &EvaluatedCards{
			rank:  highHand.rank,
			cards: highHand.cards,
		}
	}
}

func (h *PloWinnerEvaluate) GetBestPlayerCards() map[uint32]*EvaluatedCards {
	return h.activeSeatBestCombo
}

func (h *PloWinnerEvaluate) GetHighHandCards() map[uint32]*EvaluatedCards {
	return h.highHandCombo
}