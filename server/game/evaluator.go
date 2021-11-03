package game

import (
	"math/rand"
	"sort"

	"voyager.com/server/poker"
)

type EvaluatedCards struct {
	rank        int32
	cards       []byte
	playerCards []byte
	boardCards  []byte

	loFound       bool
	loRank        int32
	locards       []byte
	loPlayerCards []byte
	loBoardCards  []byte

	// high hand cards
	hhRank        int32
	hhCards       []byte
	hhPlayerCards []byte
	hhBoardCards  []byte
}

func (e EvaluatedCards) GetCards() []uint32 {
	cards := make([]uint32, len(e.cards))
	for i := range e.cards {
		cards[i] = uint32(e.cards[i])
	}
	return cards
}

func (e EvaluatedCards) GetLoCards() []uint32 {
	cards := make([]uint32, len(e.locards))
	for i := range e.locards {
		cards[i] = uint32(e.locards[i])
	}
	return cards
}

func (e EvaluatedCards) GetLoPlayerCards() []uint32 {
	cards := make([]uint32, len(e.loPlayerCards))
	for i := range e.loPlayerCards {
		cards[i] = uint32(e.loPlayerCards[i])
	}
	return cards
}

func (e EvaluatedCards) GetLoBoardCards() []uint32 {
	cards := make([]uint32, len(e.loBoardCards))
	for i := range e.loBoardCards {
		cards[i] = uint32(e.loBoardCards[i])
	}
	return cards
}

func (e EvaluatedCards) GetPlayerCards() []uint32 {
	cards := make([]uint32, len(e.playerCards))
	for i := range e.playerCards {
		cards[i] = uint32(e.playerCards[i])
	}
	return cards
}

func (e EvaluatedCards) GetBoardCards() []uint32 {
	cards := make([]uint32, len(e.boardCards))
	for i := range e.boardCards {
		cards[i] = uint32(e.boardCards[i])
	}
	return cards
}

func (e EvaluatedCards) GetHHBoardCards() []uint32 {
	cards := make([]uint32, len(e.hhBoardCards))
	for i := range e.hhBoardCards {
		cards[i] = uint32(e.hhBoardCards[i])
	}
	return cards
}

func (e EvaluatedCards) GetHHPlayerCards() []uint32 {
	cards := make([]uint32, len(e.hhPlayerCards))
	for i := range e.hhPlayerCards {
		cards[i] = uint32(e.hhPlayerCards[i])
	}
	return cards
}

type HandEvaluator interface {
	// Evaluate()
	Evaluate2(playerCards []byte, boardCards []byte) EvaluatedCards

	// GetBestPlayerCards() map[uint32]*EvaluatedCards
	// GetHighHandCards() map[uint32]*EvaluatedCards
	// GetWinners() map[uint32]*PotWinners
	// GetBoard2Winners() map[uint32]*PotWinners
}

func AnyoneHasHighHand(playerCards map[uint32][]poker.Card, board []poker.Card, gameType GameType) bool {
	if board == nil {
		return false
	}

	for _, pc := range playerCards {
		var rank int32 = -1
		if gameType == GameType_HOLDEM {
			cards := make([]poker.Card, 0)
			cards = append(cards, pc...)
			cards = append(cards, board...)
			rank, _ = poker.Evaluate(cards)
		} else {
			result := poker.EvaluateOmaha(pc, board)
			rank = result.HiRank
		}
		if rank <= 322 {
			return true
		}
	}

	return false
}

func HasSameHoleCards(playerCards map[uint32][]poker.Card) bool {
	sameHoldCardsFound := false
	matchesFound := 0
	for i := 0; i < len(playerCards); i++ {
		for j := 0; j < len(playerCards); j++ {
			if i == j {
				// Same Player
				continue
			}

			matchesFound = 0
			p1Cards := playerCards[uint32(i)]
			p2Cards := playerCards[uint32(j)]
			p1CardRanks := make([]int32, 0)
			p2CardRanks := make([]int32, 0)
			for _, c := range p1Cards {
				p1CardRanks = append(p1CardRanks, c.Rank())
			}
			for _, c := range p2Cards {
				p2CardRanks = append(p2CardRanks, c.Rank())
			}
			sort.Slice(p1CardRanks, func(a, b int) bool { return p1CardRanks[a] < p1CardRanks[b] })
			sort.Slice(p2CardRanks, func(a, b int) bool { return p2CardRanks[a] < p2CardRanks[b] })

			seenCards := make(map[int32]bool)
			for _, p1c := range p1CardRanks {
				if _, ok := seenCards[p1c]; ok {
					continue
				}
				seenCards[p1c] = true
				for _, p2c := range p2CardRanks {
					if p1c == p2c {
						matchesFound++
						break
					}
				}
			}

			if matchesFound >= 2 {
				sameHoldCardsFound = true
				break
			}
		}
		if sameHoldCardsFound {
			break
		}
	}

	return sameHoldCardsFound
}

func IsBoardPaired(board []poker.Card) bool {
	if board == nil {
		return false
	}

	pairedAt := PairedAt(board)
	return pairedAt > 0
}

// Returns
// 0   : not paired
// 1-3 : paired at flop
// 4   : paired at turn
// 5   : paired at river
func PairedAt(board []poker.Card) int {
	m := make(map[int32]int)
	pairedAtIdx := 0
	for i := 0; i < len(board); i++ {
		rank := board[i].Rank()
		_, exists := m[rank]
		if exists {
			pairedAtIdx = i + 1
			break
		}
		m[rank] = 1
	}
	return pairedAtIdx
}

func QuickShuffleCards(cards []poker.Card) {
	rand.Shuffle(len(cards), func(i, j int) { cards[i], cards[j] = cards[j], cards[i] })
}
