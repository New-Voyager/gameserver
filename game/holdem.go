package game

import "voyager.com/server/poker"

type HoldemWinnerEvaluate struct {
	handState           *HandState
	gameState           *GameState
	activeSeatBestCombo map[uint32]*EvaluatedCards
	winners             map[uint32]*PotWinners
	highHandCombo       map[uint32]*EvaluatedCards
	includeHighHand     bool
}

func NewHoldemWinnerEvaluate(gameState *GameState, handState *HandState, includeHighHand bool) *HoldemWinnerEvaluate {
	return &HoldemWinnerEvaluate{
		handState:           handState,
		gameState:           gameState,
		activeSeatBestCombo: make(map[uint32]*EvaluatedCards, gameState.MaxSeats),
		winners:             make(map[uint32]*PotWinners),
		highHandCombo:       make(map[uint32]*EvaluatedCards, gameState.MaxSeats),
		includeHighHand:     includeHighHand,
	}
}

func (h *HoldemWinnerEvaluate) Evaluate() {
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

func (h *HoldemWinnerEvaluate) GetWinners() map[uint32]*PotWinners {
	return h.winners
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
	givenChips := float32(0)
	splitChips := float32(int(pot.Pot / float32(noOfWinners)))
	winningChips := float32(0)
	handWinners := make([]*HandWinner, noOfWinners)
	i := 0
	for _, seatNo := range pot.Seats {
		if _, ok := h.activeSeatBestCombo[seatNo]; !ok {
			continue
		}
		if h.activeSeatBestCombo[seatNo].rank != lowestRank {
			continue
		}
		// winner
		winningChips = splitChips
		givenChips = givenChips + float32(splitChips)
		if pot.Pot-givenChips <= 1.0 {
			winningChips += (pot.Pot - givenChips)
		}
		evaluatedCards := h.activeSeatBestCombo[seatNo]
		rankStr := poker.RankString(evaluatedCards.rank)
		s := poker.CardsToString(evaluatedCards.cards)
		handWinners[i] = &HandWinner{
			SeatNo:          seatNo,
			Amount:          winningChips,
			WinningCards:    evaluatedCards.GetCards(),
			WinningCardsStr: s,
			RankStr:         rankStr,
			Rank:            uint32(evaluatedCards.rank),
			BoardCards:      evaluatedCards.GetBoardCards(),
			PlayerCards:     evaluatedCards.GetPlayerCards(),
		}
		i++
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

		// determine what player cards and board cards used to determine best cards
		seatCardsInCard := poker.FromByteCards(seatCards)
		playerCards := make([]poker.Card, 0)
		boardCards := make([]poker.Card, 0)
		for _, card := range playerBestCards {
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
			rank:        rank,
			cards:       poker.CardsToByteCards(playerBestCards),
			playerCards: poker.CardsToByteCards(playerCards),
			boardCards:  poker.CardsToByteCards(boardCards),
		}
	}
}

func (h *HoldemWinnerEvaluate) evaluatePlayerHighHand() {
	boardCards := poker.FromByteCards(h.handState.BoardCards)
	// determine rank for each active player
	for seatNoIdx, active := range h.handState.ActiveSeats {
		if active == 0 {
			continue
		}
		playerCards := poker.FromByteCards(h.handState.PlayersCards[uint32(seatNoIdx+1)])
		highHand := poker.EvaluateHighHand(playerCards, boardCards)
		h.highHandCombo[uint32(seatNoIdx+1)] = &EvaluatedCards{
			rank:  highHand.HiRank,
			cards: poker.CardsToByteCards(highHand.HiCards),
		}
	}
}

func (h *HoldemWinnerEvaluate) GetBestPlayerCards() map[uint32]*EvaluatedCards {
	return h.activeSeatBestCombo
}

func (h *HoldemWinnerEvaluate) GetHighHandCards() map[uint32]*EvaluatedCards {
	return h.highHandCombo
}
