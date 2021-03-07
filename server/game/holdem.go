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

func (h *HoldemWinnerEvaluate) Evaluate() {
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

		h.evaluatePlayerBestCards(boardCards)

		for i := len(pots) - 1; i >= 0; i-- {
			pot := pots[i]
			potWinners := &PotWinners{}
			potWinners.HiWinners = h.determineHandWinners(pot, boardCards)
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
				h.evaluatePlayerHighHand(boardCards)
			}
		}

		if !h.handState.RunItTwiceConfirmed {
			break
		}

		board1 = false
		board2 = true
	}
}

func (h *HoldemWinnerEvaluate) GetWinners() map[uint32]*PotWinners {
	return h.winners
}

func (h *HoldemWinnerEvaluate) GetBoard2Winners() map[uint32]*PotWinners {
	return h.board2Winners
}

func (h *HoldemWinnerEvaluate) determineHandWinners(pot *SeatsInPots, board []byte) []*HandWinner {
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

func (h *HoldemWinnerEvaluate) evaluatePlayerBestCards(board []byte) {
	// determine rank for each active player
	for seatNo, active := range h.handState.ActiveSeats {
		if active == 0 || h.handState.PlayersInSeats[seatNo] == 0 {
			continue
		}
		seatCards := h.handState.PlayersCards[uint32(seatNo)]
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
		h.activeSeatBestCombo[uint32(seatNo)] = &EvaluatedCards{
			rank:        rank,
			cards:       poker.CardsToByteCards(playerBestCards),
			playerCards: poker.CardsToByteCards(playerCards),
			boardCards:  poker.CardsToByteCards(boardCards),
		}
	}
}

func (h *HoldemWinnerEvaluate) evaluatePlayerHighHand(cards []byte) {
	boardCards := poker.FromByteCards(cards)
	// determine rank for each active player
	for seatNo, active := range h.handState.ActiveSeats {
		if active == 0 {
			continue
		}
		playerCards := poker.FromByteCards(h.handState.PlayersCards[uint32(seatNo)])
		highHand := poker.EvaluateHighHand(playerCards, boardCards)
		h.highHandCombo[uint32(seatNo)] = &EvaluatedCards{
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
