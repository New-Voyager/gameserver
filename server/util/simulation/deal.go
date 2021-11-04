package simulation

import (
	"fmt"
	"math/rand"

	"voyager.com/server/game"
	"voyager.com/server/poker"
)

func Run(numDeals int) error {
	gameType := game.GameType_HOLDEM
	numPlayers := 9
	numCardsPerPlayer := -1
	switch gameType {
	case game.GameType_HOLDEM:
		numCardsPerPlayer = 2
	case game.GameType_PLO:
		numCardsPerPlayer = 4
	case game.GameType_PLO_HILO:
		numCardsPerPlayer = 4
	case game.GameType_FIVE_CARD_PLO:
		numCardsPerPlayer = 5
	case game.GameType_FIVE_CARD_PLO_HILO:
		numCardsPerPlayer = 5
	}

	rankClasses := make(map[int]int)
	allRanks := []int{poker.StraightFlush, poker.FourOfAKind, poker.FullHouse,
		poker.Flush, poker.Straight, poker.ThreeOfAKind,
		poker.TwoPair, poker.Pair, poker.HighCard}
	for _, rc := range allRanks {
		rankClasses[rc] = 0
	}

	deck := poker.NewDeck()

	numEval := 0
	numRoyalFlushes := 0
	numPairedBoards := 0
	numFlopPairedBoards := 0
	numTurnPairedBoards := 0
	numRiverPairedBoards := 0
	numOnePairBoards := 0
	numDealsWithSameHoleCards := 0
	for i := 0; i < numDeals; i++ {
		if i > 0 && i%10000 == 0 {
			fmt.Printf("Deal %d\n", i)
		}

		// Start a new game every 100 hands.
		handNum := (i % 100) + 1

		playerCards, communityCards, err := shuffleAndDeal(deck, numCardsPerPlayer, numPlayers, gameType, handNum)
		if err != nil {
			return err
		}

		if len(communityCards) != 5 {
			return fmt.Errorf("len(communityCards) = %d", len(communityCards))
		}
		for _, pc := range playerCards {
			var rank int32 = -1
			if gameType == game.GameType_HOLDEM {
				cards := make([]poker.Card, 0)
				cards = append(cards, pc...)
				cards = append(cards, communityCards...)
				if len(cards) != numCardsPerPlayer+5 {
					return fmt.Errorf("Unexpected number of cards to be evaluated: %d", len(cards))
				}
				rank, _ = poker.Evaluate(cards)
			} else {
				result := poker.EvaluateOmaha(pc, communityCards)
				rank = result.HiRank
			}
			numEval++

			// fmt.Printf("%s: %d (%s)\n", poker.CardsToString(cards), rank, poker.RankString(rank))
			if rank == 1 {
				numRoyalFlushes++
			} else {
				rankClasses[int(poker.RankClass(rank))]++
			}
		}

		sameHoldCardsFound := game.HasSameHoleCards(playerCards)
		if sameHoldCardsFound {
			numDealsWithSameHoleCards++
		}

		pairedAtIdx := game.PairedAt(communityCards)
		if pairedAtIdx > 0 {
			numPairedBoards++
			if pairedAtIdx <= 3 {
				numFlopPairedBoards++
			} else if pairedAtIdx == 4 {
				numTurnPairedBoards++
			} else {
				numRiverPairedBoards++
			}
		}
		if isBoardOnePair(communityCards) {
			numOnePairBoards++
		}
	}

	cumTurnPairedBoards := numFlopPairedBoards + numTurnPairedBoards
	cumRiverPairedBoards := cumTurnPairedBoards + numRiverPairedBoards

	fmt.Printf("\n%d deals completed\n\n", numDeals)
	numStraightFlushes := rankClasses[poker.StraightFlush]
	numfourOfAKind := rankClasses[poker.FourOfAKind]
	numFullHouse := rankClasses[poker.FullHouse]
	numFlush := rankClasses[poker.Flush]
	numStraight := rankClasses[poker.Straight]
	numThreeOfAKind := rankClasses[poker.ThreeOfAKind]
	numTwoPair := rankClasses[poker.TwoPair]
	numOnePair := rankClasses[poker.Pair]
	numHighCard := rankClasses[poker.HighCard]

	fmt.Printf("Result (ours vs expected)\n")
	fmt.Printf("Royal Flush     : %8d/%d (%f vs 0.000032)\n", numRoyalFlushes, numEval, float32(numRoyalFlushes)/float32(numEval))
	fmt.Printf("Straight Flush  : %8d/%d (%f vs 0.000279)\n", numStraightFlushes, numEval, float32(numStraightFlushes)/float32(numEval))
	fmt.Printf("Four Of A Kind  : %8d/%d (%f vs 0.001680)\n", numfourOfAKind, numEval, float32(numfourOfAKind)/float32(numEval))
	fmt.Printf("Full House      : %8d/%d (%f vs 0.025963)\n", numFullHouse, numEval, float32(numFullHouse)/float32(numEval))
	fmt.Printf("Flush           : %8d/%d (%f vs 0.030258)\n", numFlush, numEval, float32(numFlush)/float32(numEval))
	fmt.Printf("Straight        : %8d/%d (%f vs 0.046197)\n", numStraight, numEval, float32(numStraight)/float32(numEval))
	fmt.Printf("Three of a Kind : %8d/%d (%f vs 0.048301)\n", numThreeOfAKind, numEval, float32(numThreeOfAKind)/float32(numEval))
	fmt.Printf("Two Pair        : %8d/%d (%f vs 0.234949)\n", numTwoPair, numEval, float32(numTwoPair)/float32(numEval))
	fmt.Printf("Pair            : %8d/%d (%f vs 0.438221)\n", numOnePair, numEval, float32(numOnePair)/float32(numEval))
	fmt.Printf("High Card       : %8d/%d (%f vs 0.174120)\n\n", numHighCard, numEval, float32(numHighCard)/float32(numEval))

	fmt.Printf("Paired Boards (F)     : %8d/%d (%f vs 0.171765)\n", numFlopPairedBoards, numDeals, float32(numFlopPairedBoards)/float32(numDeals))
	// fmt.Printf("Paired Boards (T)     : %8d/%d (%f)\n", numTurnPairedBoards, numDeals, float32(numTurnPairedBoards)/float32(numDeals))
	// fmt.Printf("Paired Boards (R)     : %8d/%d (%f)\n", numRiverPairedBoards, numDeals, float32(numRiverPairedBoards)/float32(numDeals))
	fmt.Printf("Paired Boards (F+T)   : %8d/%d (%f vs 0.323890)\n", cumTurnPairedBoards, numDeals, float32(cumTurnPairedBoards)/float32(numDeals))
	fmt.Printf("Paired Boards (F+T+R) : %8d/%d (%f vs 0.492917)\n", cumRiverPairedBoards, numDeals, float32(cumRiverPairedBoards)/float32(numDeals))
	fmt.Printf("One-pair Boards       : %8d/%d (%f)\n", numOnePairBoards, numDeals, float32(numOnePairBoards)/float32(numDeals))
	fmt.Printf("Same Hole Cards       : %8d/%d (%f)\n", numDealsWithSameHoleCards, numDeals, float32(numDealsWithSameHoleCards)/float32(numDeals))

	sum := 0
	sum += numRoyalFlushes
	for _, count := range rankClasses {
		sum += count
	}
	if sum != numEval {
		panic(fmt.Sprintf("ranks don't add up %d != %d", sum, numEval))
	}

	return nil
}

func isBoardOnePair(cards []poker.Card) bool {
	rank, _ := poker.Evaluate(cards)
	return rank > poker.MaxTwoPair && rank <= poker.MaxPair
}

func shuffleAndDeal(deck *poker.Deck, numCardsPerPlayer int, numPlayers int, gameType game.GameType, handNum int) (map[uint32][]poker.Card, []poker.Card, error) {
	deck.Shuffle()
	playerCards, communityCards, err := dealCards(deck, numCardsPerPlayer, numPlayers)
	if err != nil {
		return nil, nil, err
	}

	if handNum <= 10 {
		for game.AnyoneHasHighHand(playerCards, communityCards, gameType, poker.MaxFourOfAKind) {
			deck.Shuffle()
			playerCards, communityCards, err = dealCards(deck, numCardsPerPlayer, numPlayers)
		}
	} else if handNum > 10 && handNum <= 20 {
		if game.AnyoneHasHighHand(playerCards, communityCards, gameType, poker.MaxFourOfAKind) {
			if rand.Int()%2 == 0 {
				deck.Shuffle()
				playerCards, communityCards, err = dealCards(deck, numCardsPerPlayer, numPlayers)
			}
		}
	}

	if !game.AnyoneHasHighHand(playerCards, communityCards, gameType, poker.MaxFullHouse) {
		if rand.Int()%2 == 0 {
			maxReshuffleAllowed := 1
			reshuffles := 0
			for game.AnyoneHasHighHand(playerCards, communityCards, gameType, poker.MaxFullHouse) ||
				(reshuffles < maxReshuffleAllowed && game.NeedReshuffle(playerCards, communityCards, nil, gameType)) {
				deck.Shuffle()
				playerCards, communityCards, err = dealCards(deck, numCardsPerPlayer, numPlayers)
				if err != nil {
					return nil, nil, err
				}
				reshuffles++
			}
		}
	}

	return playerCards, communityCards, nil
}

func dealCards(deck *poker.Deck, numCardsPerPlayer int, numPlayers int) (map[uint32][]poker.Card, []poker.Card, error) {
	shouldBurnCards := false
	playerCards := make(map[uint32][]poker.Card)
	communityCards := make([]poker.Card, 0)
	for i := 0; i < numPlayers; i++ {
		playerCards[uint32(i)] = make([]poker.Card, numCardsPerPlayer)
	}

	for cardIdx := 0; cardIdx < numCardsPerPlayer; cardIdx++ {
		for player := 0; player < numPlayers; player++ {
			c := deck.Draw(1)[0]
			playerCards[uint32(player)][cardIdx] = c
		}
	}

	// Burn card
	if shouldBurnCards {
		deck.Draw(1)
	}
	communityCards = append(communityCards, deck.Draw(3)...)
	if shouldBurnCards {
		deck.Draw(1)
	}
	communityCards = append(communityCards, deck.Draw(1)...)
	if shouldBurnCards {
		deck.Draw(1)
	}
	communityCards = append(communityCards, deck.Draw(1)...)

	for i, cards := range playerCards {
		if len(cards) != numCardsPerPlayer {
			return playerCards, communityCards, fmt.Errorf("Misdeal %d %d", i, len(cards))
		}
	}
	if len(communityCards) != 5 {
		return playerCards, communityCards, fmt.Errorf("Misdeal community cards %d", len(communityCards))
	}

	return playerCards, communityCards, nil
}

func reDealCommunity(deck *poker.Deck, prevCommunity []poker.Card) ([]poker.Card, error) {
	deck.AddCards(prevCommunity)
	deck.ShuffleWithoutReset()
	communityCards := make([]poker.Card, 0)
	communityCards = append(communityCards, deck.Draw(5)...)
	if len(communityCards) != 5 {
		return communityCards, fmt.Errorf("Misdeal community cards %d", len(communityCards))
	}
	return communityCards, nil
}
