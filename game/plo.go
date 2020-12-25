package game

import (
	"fmt"

	"voyager.com/server/poker"
)

type PloWinnerEvaluate struct {
	handState           *HandState
	gameState           *GameState
	activeSeatBestCombo map[uint32]*EvaluatedCards
	winners             map[uint32]*PotWinners
	highHandCombo       map[uint32]*EvaluatedCards
	includeHighHand     bool
	playerResult        map[uint32]poker.OmahaResult
	hiLo                bool
}

func NewPloWinnerEvaluate(gameState *GameState, handState *HandState, includeHighHand bool, lowWinner bool) *PloWinnerEvaluate {
	return &PloWinnerEvaluate{
		handState:           handState,
		gameState:           gameState,
		activeSeatBestCombo: make(map[uint32]*EvaluatedCards, gameState.MaxSeats),
		winners:             make(map[uint32]*PotWinners),
		highHandCombo:       make(map[uint32]*EvaluatedCards, gameState.MaxSeats),
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
	splitChips := potAmount / float32(noOfWinners)
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

	splitChips := potAmount / float32(noOfWinners)
	handWinners := make([]*HandWinner, noOfWinners)
	i := 0
	for _, seatNo := range pot.Seats {
		if _, ok := h.activeSeatBestCombo[seatNo]; !ok {
			continue
		}

		if h.activeSeatBestCombo[seatNo].loRank == lowestRank {
			evaluatedCards := h.activeSeatBestCombo[seatNo]
			s := poker.CardsToString(evaluatedCards.cards)
			handWinners[i] = &HandWinner{
				SeatNo:          seatNo,
				Amount:          splitChips,
				WinningCards:    evaluatedCards.GetLoCards(),
				WinningCardsStr: s,
				BoardCards:      evaluatedCards.GetLoBoardCards(),
				PlayerCards:     evaluatedCards.GetLoPlayerCards(),
				LoCard:          true,
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
		seatNo := uint32(seatNoIdx + 1)
		seatCards := h.handState.PlayersCards[seatNo]
		playerCardsEval := poker.FromByteCards(seatCards)
		boardCardsEval := poker.FromByteCards(h.handState.BoardCards)
		result := poker.EvaluateOmaha(playerCardsEval, boardCardsEval)

		// save the result to calculate low winners
		h.playerResult[seatNo] = result

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
			poker.CardsToString(h.handState.PlayersCards[seatNo]), poker.CardsToString(h.handState.BoardCards))

		h.activeSeatBestCombo[seatNo] = &EvaluatedCards{
			rank:        result.HiRank,
			cards:       poker.CardsToByteCards(result.HiCards),
			playerCards: poker.CardsToByteCards(playerCards),
			boardCards:  poker.CardsToByteCards(boardCards),
		}
		if result.LowFound {
			fmt.Printf("seat: %d lo rank: %d cards: %s player cards: %s board cards: %s\n",
				seatNo, result.LowRank, poker.CardsToString(result.LowCards),
				poker.CardsToString(h.handState.PlayersCards[seatNo]),
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
			h.activeSeatBestCombo[seatNo].loBoardCards = poker.CardsToByteCards(loBoardCards)
			h.activeSeatBestCombo[seatNo].loPlayerCards = poker.CardsToByteCards(loPlayerCards)
			h.activeSeatBestCombo[seatNo].loRank = result.LowRank
			h.activeSeatBestCombo[seatNo].locards = poker.CardsToByteCards(result.LowCards)
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
