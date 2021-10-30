package game

import (
	"voyager.com/server/poker"
)

type HoldemWinnerEvaluate struct {
	handState           *HandState
	activeSeatBestCombo map[uint32]*EvaluatedCards // seat no is the key, evaluated cards as value
	winners             map[uint32]*PotWinners     // winners of this hand
	board2Winners       map[uint32]*PotWinners     // winners of the second board
	highHandCombo       map[uint32]*EvaluatedCards // seatno is the key, evaluated cards for high hand
	includeHighHand     bool
}

func NewHoldemWinnerEvaluate(handState *HandState, includeHighHand bool, maxSeats uint32) *HoldemWinnerEvaluate {
	return &HoldemWinnerEvaluate{
		handState:           handState,
		activeSeatBestCombo: make(map[uint32]*EvaluatedCards, maxSeats),
		winners:             make(map[uint32]*PotWinners),
		board2Winners:       make(map[uint32]*PotWinners),
		highHandCombo:       make(map[uint32]*EvaluatedCards, maxSeats),
		includeHighHand:     includeHighHand,
	}
}

func (h *HoldemWinnerEvaluate) Evaluate2(seatCards []byte, board []byte) EvaluatedCards {
	allCards := make([]byte, len(board))
	copy(allCards, board)
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

	// determine high hands
	// we will use omaha evaluator to do this
	// 2 cards from player, 3 card combo from board
	playerCardsEval := poker.FromByteCards(seatCards)
	boardCardsEval := poker.FromByteCards(board)
	result := poker.EvaluateOmaha(playerCardsEval, boardCardsEval)

	// the hh rank must be better than or equal board rank
	// for example, player holds 5, 8
	// board is: 5, 5, 5, 3, A
	// best combo: 5, 5, 5, 5, A
	// player hh rank: 5, 5, 5, 5, 8
	hhPlayerCards := make([]poker.Card, 0)
	hhBoardCards := make([]poker.Card, 0)
	hhRank := int32(0)
	if result.HiRank <= rank {
		hhRank = result.HiRank
		for _, card := range result.HiCards {
			isPlayerCard := false
			for _, playerCard := range seatCardsInCard {
				if playerCard == card {
					isPlayerCard = true
					break
				}
			}
			if isPlayerCard {
				hhPlayerCards = append(hhPlayerCards, card)
			} else {
				hhBoardCards = append(hhBoardCards, card)
			}
		}
	}

	return EvaluatedCards{
		cards:         poker.CardsToByteCards(playerBestCards), // best combo
		playerCards:   poker.CardsToByteCards(playerCards),     // players cards used to make the combo
		boardCards:    poker.CardsToByteCards(boardCards),      // board cards used to make the combo
		rank:          rank,
		hhRank:        hhRank,
		hhPlayerCards: poker.CardsToByteCards(hhPlayerCards),
		hhBoardCards:  poker.CardsToByteCards(hhBoardCards),
		hhCards:       poker.CardsToByteCards(result.HiCards),
	}
}
