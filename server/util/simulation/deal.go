package simulation

import (
	"fmt"

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

	hitsPerRank := make(map[int]int)
	for i := 0; i <= 322; i++ {
		hitsPerRank[i] = 0
	}

	deck := poker.NewDeck()

	numEval := 0
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

		playerCards, communityCards, err := shuffleAndDeal(deck, numCardsPerPlayer, numPlayers, gameType)
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
			if rank <= 322 {
				hitsPerRank[int(rank)]++
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

	fmt.Printf("%d deals completed\n\nResult:\n", numDeals)
	numRotalFlushes := 0
	numStraightFlushes := 0
	numfourOfAKind := 0
	numFullHouse := 0
	for rank := 0; rank <= 322; rank++ {
		count := hitsPerRank[rank]
		// fmt.Printf("%3d (%s): %d\n", rank, poker.RankString(int32(rank)), count)
		if rank == 1 {
			numRotalFlushes += count
		} else if rank <= 10 {
			numStraightFlushes += count
		} else if rank <= 166 {
			numfourOfAKind += count
		} else if rank <= 322 {
			numFullHouse += count
		}
	}

	fmt.Printf("Royal Flushes         : %d/%d (%f)\n", numRotalFlushes, numEval, float32(numRotalFlushes)/float32(numEval))
	fmt.Printf("Straight Flushes      : %d/%d (%f)\n", numStraightFlushes, numEval, float32(numStraightFlushes)/float32(numEval))
	fmt.Printf("Four Of A Kind        : %d/%d (%f)\n", numfourOfAKind, numEval, float32(numfourOfAKind)/float32(numEval))
	fmt.Printf("Full House            : %d/%d (%f)\n", numFullHouse, numEval, float32(numFullHouse)/float32(numEval))
	fmt.Printf("Paired Boards         : %d/%d (%f)\n", numPairedBoards, numDeals, float32(numPairedBoards)/float32(numDeals))
	fmt.Printf("Paired Boards (F)     : %d/%d (%f)\n", numFlopPairedBoards, numDeals, float32(numFlopPairedBoards)/float32(numDeals))
	fmt.Printf("Paired Boards (T)     : %d/%d (%f)\n", numTurnPairedBoards, numDeals, float32(numTurnPairedBoards)/float32(numDeals))
	fmt.Printf("Paired Boards (R)     : %d/%d (%f)\n", numRiverPairedBoards, numDeals, float32(numRiverPairedBoards)/float32(numDeals))
	fmt.Printf("Paired Boards (F+T)   : %d/%d (%f)\n", cumTurnPairedBoards, numDeals, float32(cumTurnPairedBoards)/float32(numDeals))
	fmt.Printf("Paired Boards (F+T+R) : %d/%d (%f)\n", cumRiverPairedBoards, numDeals, float32(cumRiverPairedBoards)/float32(numDeals))
	fmt.Printf("One-pair Boards       : %d/%d (%f)\n", numOnePairBoards, numDeals, float32(numOnePairBoards)/float32(numDeals))
	fmt.Printf("Same Hole Cards       : %d/%d (%f)\n", numDealsWithSameHoleCards, numDeals, float32(numDealsWithSameHoleCards)/float32(numDeals))

	return nil
}

func isBoardOnePair(cards []poker.Card) bool {
	rank, _ := poker.Evaluate(cards)
	return rank > 3325 && rank <= 6185
}

func shuffleAndDeal(deck *poker.Deck, numCardsPerPlayer int, numPlayers int, gameType game.GameType) (map[uint32][]poker.Card, []poker.Card, error) {
	deck.Shuffle()
	playerCards, communityCards, err := dealCards(deck, numCardsPerPlayer, numPlayers)
	if err != nil {
		return nil, nil, err
	}
	if game.AnyoneHasHighHand(playerCards, communityCards, gameType) {
		return playerCards, communityCards, nil
	}

	maxReshuffleAllowed := 1
	reshuffles := 0
	for game.AnyoneHasHighHand(playerCards, communityCards, gameType) ||
		(reshuffles < maxReshuffleAllowed && game.NeedReshuffle(playerCards, communityCards, nil, gameType)) {
		deck.Shuffle()
		playerCards, communityCards, err = dealCards(deck, numCardsPerPlayer, numPlayers)
		if err != nil {
			return nil, nil, err
		}
		reshuffles++
	}

	pairedAt := game.PairedAt(communityCards)
	if pairedAt >= 1 && pairedAt <= 3 {
		game.QuickShuffleCards(communityCards)
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
