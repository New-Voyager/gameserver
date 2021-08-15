package game

import (
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
		playerResult:        make(map[uint32]poker.OmahaResult),
		hiLo:                lowWinner,
	}
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
