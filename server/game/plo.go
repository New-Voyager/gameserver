package game

import (
	"fmt"

	"voyager.com/server/poker"
)

type PloWinnerEvaluate struct {
	handState           *HandState
	board1SeatBestCombo map[uint32]*EvaluatedCards
	board2SeatBestCombo map[uint32]*EvaluatedCards
	winners             map[uint32]*PotWinners
	highHandCombo       map[uint32]*EvaluatedCards
	includeHighHand     bool
	playerResult        map[uint32]poker.OmahaResult
	hiLo                bool
	board2Winners       map[uint32]*PotWinners // winners of the second board
}

func NewPloWinnerEvaluate(handState *HandState, includeHighHand bool, lowWinner bool, maxSeats uint32) *PloWinnerEvaluate {
	return &PloWinnerEvaluate{
		handState:           handState,
		board1SeatBestCombo: make(map[uint32]*EvaluatedCards, maxSeats),
		board2SeatBestCombo: make(map[uint32]*EvaluatedCards, maxSeats),
		winners:             make(map[uint32]*PotWinners),
		board2Winners:       make(map[uint32]*PotWinners),
		highHandCombo:       make(map[uint32]*EvaluatedCards, maxSeats),
		includeHighHand:     includeHighHand,
		playerResult:        make(map[uint32]poker.OmahaResult, 0),
		hiLo:                lowWinner,
	}
}

func (h *PloWinnerEvaluate) GetWinners() map[uint32]*PotWinners {
	return h.winners
}

func (h *PloWinnerEvaluate) GetBoard2Winners() map[uint32]*PotWinners {
	return h.board2Winners
}

func (h *PloWinnerEvaluate) Evaluate() {
	boardCards := h.handState.BoardCards
	pots := h.handState.Pots
	board1 := true
	board2 := false

	var board1Pot []*SeatsInPots
	var board2Pot []*SeatsInPots

	if h.handState.RunItTwiceConfirmed {
		// // divide the pot into two (there should only be one pot for run-it-twice)
		pot1 := float32(int(h.handState.Pots[0].Pot / 2.0))
		pot2 := h.handState.Pots[0].Pot - pot1
		board1Pot = []*SeatsInPots{&SeatsInPots{Seats: h.handState.Pots[0].Seats, Pot: pot1}}
		board2Pot = []*SeatsInPots{&SeatsInPots{Seats: h.handState.Pots[0].Seats, Pot: pot2}}
	}

	for i := 0; i < 2; i++ {
		if h.handState.RunItTwiceConfirmed {
			if board1 {
				pots = board1Pot
			} else {
				boardCards = h.handState.BoardCards_2
				pots = board2Pot
			}
		}
		h.evaluatePlayerBestCards(boardCards, board1, board2)
		for i := len(pots) - 1; i >= 0; i-- {

			pot := pots[i]

			hiPotAmount := pot.Pot
			loPotAmount := float32(0)
			potWinners := &PotWinners{}
			potWinners.PotNo = uint32(i)
			potWinners.Amount = pot.Pot
			if h.hiLo {
				// determine whether there is a low winner in this pot
				if h.isLowWinner(pot, board1, board2) {
					loPotAmount = float32(int(pot.Pot / 2.0))
					hiPotAmount = pot.Pot - loPotAmount
					potWinners.LowWinners = h.determineLoHandWinners(pot, loPotAmount, board1, board2)
				}
			}
			potWinners.HiWinners = h.determineHandWinners(pot, hiPotAmount, board1, board2)
			// h.winners[uint32(i)] = potWinners
			// pot := pots[i]
			// potWinners := &PotWinners{}
			// potWinners.HiWinners = h.determineHandWinners(pot, boardCards)
			if board1 {
				h.winners[uint32(i)] = potWinners
			}

			if board2 {
				h.board2Winners[uint32(i)] = potWinners
			}
		}

		// only board1 qualifies for highhand
		if board1 {
			if h.includeHighHand {
				h.evaluatePlayerHighHand()
			}
		}

		if !h.handState.RunItTwiceConfirmed {
			break
		}

		board1 = false
		board2 = true
	}
}

func (h *PloWinnerEvaluate) isLowWinner(pot *SeatsInPots, board1 bool, board2 bool) bool {
	var bestRank map[uint32]*EvaluatedCards
	if board1 {
		bestRank = h.board1SeatBestCombo
	} else if board2 {
		bestRank = h.board2SeatBestCombo
	}

	for _, seatNo := range pot.Seats {
		if _, ok := bestRank[seatNo]; !ok {
			continue
		}
		evaluation := bestRank[seatNo]
		if evaluation.loRank != 0 {
			return true
		}
	}
	return false
}

func (h *PloWinnerEvaluate) determineHandWinners(pot *SeatsInPots, potAmount float32, board1 bool, board2 bool) []*HandWinner {
	var bestRank map[uint32]*EvaluatedCards

	if board1 {
		bestRank = h.board1SeatBestCombo
	} else if board2 {
		bestRank = h.board2SeatBestCombo
	}

	// determine the lowest ranking card first
	lowestRank := int32(0x7FFFFFFF)
	for _, seatNo := range pot.Seats {
		if _, ok := bestRank[seatNo]; !ok {
			continue
		}
		evaluation := bestRank[seatNo]
		if evaluation.rank != 0 && evaluation.rank < lowestRank {
			lowestRank = evaluation.rank
		}
	}

	noOfWinners := 0
	for _, seatNo := range pot.Seats {
		if _, ok := bestRank[seatNo]; !ok {
			continue
		}

		if bestRank[seatNo].rank == lowestRank {
			noOfWinners++
		}
	}
	handWinners := make([]*HandWinner, noOfWinners)
	i := 0
	splitChips := int(potAmount / float32(noOfWinners))
	remainingWinners := noOfWinners
	remainingAmount := potAmount
	for _, seatNo := range pot.Seats {
		if _, ok := bestRank[seatNo]; !ok {
			continue
		}
		if bestRank[seatNo].rank != lowestRank {
			continue
		}
		remainingWinners--
		chipsAward := float32(splitChips)
		if remainingWinners == 0 {
			chipsAward = remainingAmount
		}
		remainingAmount = remainingAmount - chipsAward

		evaluatedCards := bestRank[seatNo]
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

func (h *PloWinnerEvaluate) determineLoHandWinners(pot *SeatsInPots, potAmount float32, board1 bool, board2 bool) []*HandWinner {
	var bestRank map[uint32]*EvaluatedCards

	if board1 {
		bestRank = h.board1SeatBestCombo
	} else if board2 {
		bestRank = h.board2SeatBestCombo
	}

	// determine the lowest ranking card first
	lowestRank := int32(0x7FFFFFFF)
	for _, seatNo := range pot.Seats {
		if _, ok := bestRank[seatNo]; !ok {
			continue
		}
		evaluation := bestRank[seatNo]
		if evaluation.loRank != 0 && evaluation.loRank < lowestRank {
			lowestRank = evaluation.loRank
		}
	}

	noOfWinners := 0
	for _, seatNo := range pot.Seats {
		if _, ok := bestRank[seatNo]; !ok {
			continue
		}

		if bestRank[seatNo].loRank == lowestRank {
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
		if _, ok := bestRank[seatNo]; !ok {
			continue
		}
		if bestRank[seatNo].loRank != lowestRank {
			continue
		}

		remainingWinners--
		chipsAward := float32(splitChips)
		if remainingWinners == 0 {
			chipsAward = remainingAmount
		}
		remainingAmount = remainingAmount - chipsAward

		evaluatedCards := bestRank[seatNo]
		s := poker.CardsToString(evaluatedCards.GetLoCards())
		fmt.Printf("\n\nSeatNo: %d Low cards: %s %v\n\n", seatNo, s, evaluatedCards.GetLoCards())
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

func (h *PloWinnerEvaluate) evaluatePlayerBestCards(boardCards []byte, board1 bool, board2 bool) {
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
		boardCardsEval := poker.FromByteCards(boardCards)
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
			poker.CardsToString(h.handState.PlayersCards[uint32(seatNo)]), poker.CardsToString(boardCards))

		if board1 {
			h.board1SeatBestCombo[uint32(seatNo)] = &EvaluatedCards{
				rank:        result.HiRank,
				cards:       poker.CardsToByteCards(result.HiCards),
				playerCards: poker.CardsToByteCards(playerCards),
				boardCards:  poker.CardsToByteCards(boardCards),
			}
		} else if board2 {
			h.board2SeatBestCombo[uint32(seatNo)] = &EvaluatedCards{
				rank:        result.HiRank,
				cards:       poker.CardsToByteCards(result.HiCards),
				playerCards: poker.CardsToByteCards(playerCards),
				boardCards:  poker.CardsToByteCards(boardCards),
			}
		}

		if result.LowFound {
			fmt.Printf("seat: %d lo rank: %d cards: %s player cards: %s board cards: %s\n",
				seatNo, result.LowRank, poker.CardsToString(result.LowCards),
				poker.CardsToString(h.handState.PlayersCards[uint32(seatNo)]),
				poker.CardsToString(boardCards))

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
			if board1 {
				h.board1SeatBestCombo[uint32(seatNo)].loBoardCards = poker.CardsToByteCards(loBoardCards)
				h.board1SeatBestCombo[uint32(seatNo)].loPlayerCards = poker.CardsToByteCards(loPlayerCards)
				h.board1SeatBestCombo[uint32(seatNo)].loRank = result.LowRank
				h.board1SeatBestCombo[uint32(seatNo)].locards = poker.CardsToByteCards(result.LowCards)
			} else if board2 {
				h.board2SeatBestCombo[uint32(seatNo)].loBoardCards = poker.CardsToByteCards(loBoardCards)
				h.board2SeatBestCombo[uint32(seatNo)].loPlayerCards = poker.CardsToByteCards(loPlayerCards)
				h.board2SeatBestCombo[uint32(seatNo)].loRank = result.LowRank
				h.board2SeatBestCombo[uint32(seatNo)].locards = poker.CardsToByteCards(result.LowCards)
			}
		}
	}
}

func (h *PloWinnerEvaluate) evaluatePlayerHighHand() {
	// determine rank for each active player
	for seatNo, active := range h.handState.ActiveSeats {
		if active == 0 || h.handState.PlayersInSeats[seatNo] == 0 {
			continue
		}
		highHand := h.board1SeatBestCombo[uint32(seatNo)]
		h.highHandCombo[uint32(seatNo)] = &EvaluatedCards{
			rank:  highHand.rank,
			cards: highHand.cards,
		}
	}
}

func (h *PloWinnerEvaluate) GetBestPlayerCards() map[uint32]*EvaluatedCards {
	return h.board1SeatBestCombo
}

func (h *PloWinnerEvaluate) GetHighHandCards() map[uint32]*EvaluatedCards {
	return h.highHandCombo
}

func (h *PloWinnerEvaluate) Evaluate2(seatCards []byte, board []byte) EvaluatedCards {
	playerCardsEval := poker.FromByteCards(seatCards)
	boardCardsEval := poker.FromByteCards(board)
	result := poker.EvaluateOmaha(playerCardsEval, boardCardsEval)

	// determine what player cards and board cards used to determine best cards
	seatCardsInCard := poker.FromByteCards(seatCards)
	hiPlayerCards := make([]poker.Card, 0)
	hiBoardCards := make([]poker.Card, 0)
	for _, card := range result.HiCards {
		isPlayerCard := false
		for _, playerCard := range seatCardsInCard {
			if playerCard == card {
				isPlayerCard = true
				break
			}
		}
		if isPlayerCard {
			hiPlayerCards = append(hiPlayerCards, card)
		} else {
			hiBoardCards = append(hiBoardCards, card)
		}
	}
	loPlayerCards := make([]poker.Card, 0)
	loBoardCards := make([]poker.Card, 0)

	if result.LowFound {
		// fmt.Printf("seat: %d lo rank: %d cards: %s player cards: %s board cards: %s\n",
		// 	seatNo, result.LowRank, poker.CardsToString(result.LowCards),
		// 	poker.CardsToString(h.handState.PlayersCards[uint32(seatNo)]),
		// 	poker.CardsToString(boardCards))

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
	}
	eval := EvaluatedCards{
		rank:        result.HiRank,
		cards:       poker.CardsToByteCards(result.HiCards),
		playerCards: poker.CardsToByteCards(hiPlayerCards),
		boardCards:  poker.CardsToByteCards(hiBoardCards),
		loFound:     result.LowFound,
	}
	eval.loRank = int32(0x7fffffff)
	if result.LowFound {
		eval.loRank = result.LowRank
		eval.locards = poker.CardsToByteCards(loBoardCards)
		eval.locards = append(eval.locards, poker.CardsToByteCards(loPlayerCards)...)
		eval.loPlayerCards = poker.CardsToByteCards(loPlayerCards)
		eval.loBoardCards = poker.CardsToByteCards(loBoardCards)
	}
	return eval
}
