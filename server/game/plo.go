package game

import (
	"fmt"

	"voyager.com/server/poker"
)

type PloWinnerEvaluate struct {
	handState           *HandState
	activeSeatBestCombo map[uint32]*EvaluatedCards
	winners             map[uint32]*PotWinners
	highHandCombo       map[uint32]*EvaluatedCards
	includeHighHand     bool
	playerResult        map[uint32]poker.OmahaResult
	hiLo                bool
}

func NewPloWinnerEvaluate(handState *HandState, includeHighHand bool, lowWinner bool, maxSeats uint32) *PloWinnerEvaluate {
	return &PloWinnerEvaluate{
		handState:           handState,
		activeSeatBestCombo: make(map[uint32]*EvaluatedCards, maxSeats),
		winners:             make(map[uint32]*PotWinners),
		highHandCombo:       make(map[uint32]*EvaluatedCards, maxSeats),
		includeHighHand:     includeHighHand,
		playerResult:        make(map[uint32]poker.OmahaResult, 0),
		hiLo:                lowWinner,
	}
}

func (h *PloWinnerEvaluate) GetWinners() map[uint32]*PotWinners {
	return h.winners
}

func (h *PloWinnerEvaluate) Evaluate() {
	h.evaluatePlayerBestCards()
	for i := len(h.handState.Pots) - 1; i >= 0; i-- {
		pot := h.handState.Pots[i]
		hiPotAmount := pot.Pot
		loPotAmount := float32(0)
		potWinners := &PotWinners{}
		if h.hiLo {
			// determine whether there is a low winner in this pot
			if h.isLowWinner(pot) {
				loPotAmount = float32(int(pot.Pot / 2.0))
				hiPotAmount = pot.Pot - loPotAmount
				potWinners.LowWinners = h.determineLoHandWinners(pot, loPotAmount)
			}
		}
		potWinners.HiWinners = h.determineHandWinners(pot, hiPotAmount)
		h.winners[uint32(i)] = potWinners
	}

	if h.includeHighHand {
		h.evaluatePlayerHighHand()
	}
}

func (h *PloWinnerEvaluate) isLowWinner(pot *SeatsInPots) bool {
	for _, seatNo := range pot.Seats {
		if _, ok := h.activeSeatBestCombo[seatNo]; !ok {
			continue
		}
		evaluation := h.activeSeatBestCombo[seatNo]
		if evaluation.loRank != 0 {
			return true
		}
	}
	return false
}

func (h *PloWinnerEvaluate) determineHandWinners(pot *SeatsInPots, potAmount float32) []*HandWinner {
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
	handWinners := make([]*HandWinner, noOfWinners)
	i := 0
	splitChips := int(potAmount / float32(noOfWinners))
	remainingWinners := noOfWinners
	remainingAmount := potAmount
	for _, seatNo := range pot.Seats {
		if _, ok := h.activeSeatBestCombo[seatNo]; !ok {
			continue
		}
		if h.activeSeatBestCombo[seatNo].rank != lowestRank {
			continue
		}
		remainingWinners--
		chipsAward := float32(splitChips)
		if remainingWinners == 0 {
			chipsAward = remainingAmount
		}
		remainingAmount = remainingAmount - chipsAward

		evaluatedCards := h.activeSeatBestCombo[seatNo]
		rankStr := poker.RankString(evaluatedCards.rank)
		s := poker.CardsToString(evaluatedCards.cards)
		handWinners[i] = &HandWinner{
			SeatNo:          seatNo,
			Amount:          chipsAward,
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

func (h *PloWinnerEvaluate) determineLoHandWinners(pot *SeatsInPots, potAmount float32) []*HandWinner {
	// determine the lowest ranking card first
	lowestRank := int32(0x7FFFFFFF)
	for _, seatNo := range pot.Seats {
		if _, ok := h.activeSeatBestCombo[seatNo]; !ok {
			continue
		}
		evaluation := h.activeSeatBestCombo[seatNo]
		if evaluation.loRank != 0 && evaluation.loRank < lowestRank {
			lowestRank = evaluation.loRank
		}
	}

	noOfWinners := 0
	for _, seatNo := range pot.Seats {
		if _, ok := h.activeSeatBestCombo[seatNo]; !ok {
			continue
		}

		if h.activeSeatBestCombo[seatNo].loRank == lowestRank {
			noOfWinners++
		}
	}
	if lowestRank == 0x7FFFFFFF {
		lowestRank = 0
	}

	handWinners := make([]*HandWinner, noOfWinners)
	splitChips := int(potAmount / float32(noOfWinners))
	remainingWinners := noOfWinners
	remainingAmount := potAmount

	i := 0
	for _, seatNo := range pot.Seats {
		if _, ok := h.activeSeatBestCombo[seatNo]; !ok {
			continue
		}
		if h.activeSeatBestCombo[seatNo].loRank != lowestRank {
			continue
		}

		remainingWinners--
		chipsAward := float32(splitChips)
		if remainingWinners == 0 {
			chipsAward = remainingAmount
		}
		remainingAmount = remainingAmount - chipsAward

		evaluatedCards := h.activeSeatBestCombo[seatNo]
		s := poker.CardsToString(evaluatedCards.cards)
		handWinners[i] = &HandWinner{
			SeatNo:          seatNo,
			Amount:          chipsAward,
			WinningCards:    evaluatedCards.GetLoCards(),
			WinningCardsStr: s,
			Rank:            uint32(lowestRank),
			BoardCards:      evaluatedCards.GetLoBoardCards(),
			PlayerCards:     evaluatedCards.GetLoPlayerCards(),
			LoCard:          true,
		}
		i++
	}

	return handWinners
}

func (h *PloWinnerEvaluate) evaluatePlayerBestCards() {
	// determine rank for each active player
	for seatNo, active := range h.handState.ActiveSeats {
		if active == 0 {
			continue
		}
		seatCards := h.handState.PlayersCards[uint32(seatNo)]
		if seatCards == nil {
			continue
		}
		playerCardsEval := poker.FromByteCards(seatCards)
		boardCardsEval := poker.FromByteCards(h.handState.BoardCards)
		result := poker.EvaluateOmaha(playerCardsEval, boardCardsEval)

		// save the result to calculate low winners
		h.playerResult[uint32(seatNo)] = result

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
		fmt.Printf("seat: %d lo rank: %d cards: %s player cards: %s board cards: %s\n",
			seatNo, result.HiRank, poker.CardsToString(result.HiCards),
			poker.CardsToString(h.handState.PlayersCards[uint32(seatNo)]), poker.CardsToString(h.handState.BoardCards))

		h.activeSeatBestCombo[uint32(seatNo)] = &EvaluatedCards{
			rank:        result.HiRank,
			cards:       poker.CardsToByteCards(result.HiCards),
			playerCards: poker.CardsToByteCards(playerCards),
			boardCards:  poker.CardsToByteCards(boardCards),
		}
		if result.LowFound {
			fmt.Printf("seat: %d lo rank: %d cards: %s player cards: %s board cards: %s\n",
				seatNo, result.LowRank, poker.CardsToString(result.LowCards),
				poker.CardsToString(h.handState.PlayersCards[uint32(seatNo)]),
				poker.CardsToString(h.handState.BoardCards))

			loPlayerCards := make([]poker.Card, 0)
			loBoardCards := make([]poker.Card, 0)
			for _, card := range result.LowCards {
				isPlayerCard := false
				for _, playerCard := range seatCardsInCard {
					if playerCard == card {
						isPlayerCard = true
						break
					}
				}
				if isPlayerCard {
					loPlayerCards = append(loPlayerCards, card)
				} else {
					loBoardCards = append(loBoardCards, card)
				}
			}
			// lo cards
			h.activeSeatBestCombo[uint32(seatNo)].loBoardCards = poker.CardsToByteCards(loBoardCards)
			h.activeSeatBestCombo[uint32(seatNo)].loPlayerCards = poker.CardsToByteCards(loPlayerCards)
			h.activeSeatBestCombo[uint32(seatNo)].loRank = result.LowRank
			h.activeSeatBestCombo[uint32(seatNo)].locards = poker.CardsToByteCards(result.LowCards)
		}
	}
}

func (h *PloWinnerEvaluate) evaluatePlayerHighHand() {
	// determine rank for each active player
	for seatNo, active := range h.handState.ActiveSeats {
		if active == 0 || h.handState.PlayersInSeats[seatNo] == 0 {
			continue
		}
		highHand := h.activeSeatBestCombo[uint32(seatNo)]
		h.highHandCombo[uint32(seatNo)] = &EvaluatedCards{
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
