package poker

import (
	"fmt"
)

var table *lookupTable

func init() {
	table = newLookupTable()
}

func RankClass(rank int32) int32 {
	targets := [...]int32{
		maxStraightFlush,
		maxFourOfAKind,
		maxFullHouse,
		maxFlush,
		maxStraight,
		maxThreeOfAKind,
		maxTwoPair,
		maxPair,
		maxHighCard,
	}

	if rank < 0 {
		panic(fmt.Sprintf("rank %d is less than zero", rank))
	}

	for _, target := range targets {
		if rank <= target {
			return maxToRankClass[target]
		}
	}

	panic(fmt.Sprintf("rank %d is unknown", rank))
}

func RankString(rank int32) string {
	return rankClassToString[RankClass(rank)]
}

func Evaluate(cards []Card) (int32, []Card) {
	switch len(cards) {
	case 5:
		return five(cards...)
	case 6:
		return six(cards...)
	case 7:
		return seven(cards...)
	default:
		panic("Only support 5, 6 and 7 cards.")
	}
}

func five(cards ...Card) (int32, []Card) {
	if cards[0]&cards[1]&cards[2]&cards[3]&cards[4]&0xF000 != 0 {
		handOR := (cards[0] | cards[1] | cards[2] | cards[3] | cards[4]) >> 16
		prime := primeProductFromRankBits(int32(handOR))
		return table.flushLookup[prime], cards
	}

	prime := primeProductFromHand(cards)
	return table.unsuitedLookup[prime], cards
}

func six(cards ...Card) (int32, []Card) {
	var minimum int32 = maxHighCard
	targets := make([]Card, len(cards))
	var bestCards []Card = make([]Card, 5)
	for i := 0; i < len(cards); i++ {
		copy(targets, cards)
		targets := append(targets[:i], targets[i+1:]...)

		score, evaluatedCards := five(targets...)
		if score < minimum {
			minimum = score
			copy(bestCards, evaluatedCards)
		}
	}
	return minimum, bestCards
}

func seven(cards ...Card) (int32, []Card) {
	var minimum int32 = maxHighCard
	targets := make([]Card, len(cards))
	var bestCards []Card = make([]Card, 5)
	for i := 0; i < len(cards); i++ {
		copy(targets, cards)
		targets := append(targets[:i], targets[i+1:]...)

		score, evaluatedCards := six(targets...)
		if score < minimum {
			minimum = score
			copy(bestCards, evaluatedCards)
		}
	}

	return minimum, bestCards
}

type OmahaResult struct {
	HiRank   int32
	HiCards  []Card
	LowFound bool
	LowRank  int32
	LowCards []Card
}

type HighHand struct {
	HiRank  int32
	HiCards []Card
}

func EvaluateOmaha(playerCards []Card, boardCards []Card) OmahaResult {
	minimum := int32(maxHighCard)
	lowScore := int32(0x7FFFFFF)

	playerPairs := make([][]Card, 0)
	for pair := range combinations(playerCards, 2) {
		playerPairs = append(playerPairs, pair)
	}
	boardPairs := make([][]Card, 0)
	for pair := range combinations(boardCards, 3) {
		boardPairs = append(boardPairs, pair)
	}
	bestCards := make([]Card, 5)
	lowCards := make([]Card, 5)
	lowFound := false
	for _, playerPair := range playerPairs {
		for _, boardPair := range boardPairs {
			cards := make([]Card, 0)
			cards = append(cards, playerPair...)
			cards = append(cards, boardPair...)
			str := CardsToString(cards)
			score, _ := five(cards...)
			rankText := RankString(score)
			fmt.Printf("%s score: %d rank: %s\n", str, score, rankText)
			if score < minimum {
				minimum = score
				copy(bestCards, cards)
			}

			isLow, score := table.getLowRank(cards)
			if isLow && score < lowScore {
				copy(lowCards, cards)
				lowFound = true
				lowScore = score
			}
		}
	}
	return OmahaResult{
		HiRank:   minimum,
		HiCards:  bestCards,
		LowFound: lowFound,
		LowRank:  lowScore,
		LowCards: lowCards,
	}
}

func EvaluateHighHand(playerCards []Card, boardCards []Card) HighHand {
	minimum := int32(maxHighCard)

	playerPairs := make([][]Card, 0)
	for pair := range combinations(playerCards, 2) {
		playerPairs = append(playerPairs, pair)
	}
	boardPairs := make([][]Card, 0)
	for pair := range combinations(boardCards, 3) {
		boardPairs = append(boardPairs, pair)
	}
	bestCards := make([]Card, 5)
	for _, playerPair := range playerPairs {
		for _, boardPair := range boardPairs {
			cards := make([]Card, 0)
			cards = append(cards, playerPair...)
			cards = append(cards, boardPair...)
			score, _ := five(cards...)
			if score < minimum {
				minimum = score
				copy(bestCards, cards)
			}
		}
	}

	return HighHand{
		HiRank:  minimum,
		HiCards: bestCards,
	}
}
