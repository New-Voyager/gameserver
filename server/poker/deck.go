package poker

import (
	crypto_rand "crypto/rand"
	"fmt"
	"math/big"
	"math/rand"
	"sort"
)

var fullDeck *Deck

func init() {
	fullDeck = &Deck{cards: initializeFullCards()}
}

type Deck struct {
	cards               []Card
	scriptedCardsBySeat map[uint32]CardsInAscii
	randGen             *rand.Rand
}

func NewSeed() rand.Source {
	// var b [8]byte
	// _, err := crypto_rand.Read(b[:])
	// if err != nil {
	// 	panic("cannot seed math/rand package with cryptographically secure random number generator")
	// }

	const MaxUint = ^uint(0)
	const MaxInt = int(MaxUint >> 1)
	nBig, err := crypto_rand.Int(crypto_rand.Reader, big.NewInt(int64(MaxInt)))
	if err != nil {
		panic("cannot seed math/rand package with cryptographically secure random number generator")
	}

	// source := rand.NewSource(int64(binary.LittleEndian.Uint64(b[:])))
	source := rand.NewSource(nBig.Int64())
	return source
}

func NewDeck(source rand.Source) *Deck {
	if source == nil {
		source = NewSeed()
	}
	randGen := rand.New(source)
	deck := &Deck{randGen: randGen}
	deck.Shuffle()
	return deck
}

func NewDeckNoShuffle() *Deck {
	deck := &Deck{}
	deck.cards = make([]Card, len(fullDeck.cards))
	copy(deck.cards, fullDeck.cards)
	return deck
}

func NewDeckFromBytes(cards []byte, deckIndex int) *Deck {
	deck := &Deck{}
	remainingDeckLen := len(fullDeck.cards) - deckIndex
	deck.cards = make([]Card, remainingDeckLen)
	j := 0
	for i := deckIndex; i < len(fullDeck.cards); i++ {
		card := cards[i]
		deck.cards[j] = NewCardFromByte(card)
		j++
	}
	return deck
}

func CopyDeck(original *Deck) *Deck {
	deck := &Deck{}
	deck.cards = make([]Card, len(original.cards))
	copy(deck.cards, original.cards)
	return deck
}

func (deck *Deck) Shuffle() *Deck {
	deck.cards = make([]Card, len(fullDeck.cards))
	copy(deck.cards, fullDeck.cards)
	rand.Shuffle(len(deck.cards), func(i, j int) { deck.cards[i], deck.cards[j] = deck.cards[j], deck.cards[i] })
	rand.Shuffle(len(deck.cards), func(i, j int) { deck.cards[i], deck.cards[j] = deck.cards[j], deck.cards[i] })
	deck.box()
	rand.Shuffle(len(deck.cards), func(i, j int) { deck.cards[i], deck.cards[j] = deck.cards[j], deck.cards[i] })

	return deck
}

func (deck *Deck) ShuffleWithoutReset() *Deck {
	rand.Shuffle(len(deck.cards), func(i, j int) { deck.cards[i], deck.cards[j] = deck.cards[j], deck.cards[i] })
	rand.Shuffle(len(deck.cards), func(i, j int) { deck.cards[i], deck.cards[j] = deck.cards[j], deck.cards[i] })
	deck.box()
	rand.Shuffle(len(deck.cards), func(i, j int) { deck.cards[i], deck.cards[j] = deck.cards[j], deck.cards[i] })

	return deck
}

func (deck *Deck) box() {
	cards := make([]Card, len(deck.cards))
	nextIdx := 0
	cut1 := len(deck.cards) / 3
	cut2 := cut1 * 2
	for i := cut2; i < len(deck.cards); i++ {
		cards[nextIdx] = deck.cards[i]
		nextIdx++
	}
	for i := cut1; i < cut2; i++ {
		cards[nextIdx] = deck.cards[i]
		nextIdx++
	}
	for i := 0; i < cut1; i++ {
		cards[nextIdx] = deck.cards[i]
		nextIdx++
	}
	deck.cards = cards
}

func (deck *Deck) Draw(n int) []Card {
	cards := make([]Card, n)
	copy(cards, deck.cards[:n])
	deck.cards = deck.cards[n:]
	return cards
}

func (deck *Deck) FindAndReplace(cardInDeck Card, newCard Card) {
	idx := deck.getCardLoc(cardInDeck)
	if idx < 0 {
		panic(fmt.Sprintf("Deck.FindAndReplace unable to find card %s in deck\nDeck: %s\n", CardToString(cardInDeck), CardsToString(deck.GetBytes())))
	}
	deck.cards[idx] = newCard
}

func (deck *Deck) Empty() bool {
	return len(deck.cards) == 0
}

func initializeFullCards() []Card {
	var cards []Card

	for _, rank := range strRanks {
		for suit := range charSuitToIntSuit {
			cards = append(cards, NewCard(string(rank)+string(suit)))
		}
	}

	return cards
}

// Returns cards in rank format
// high 4 bits rank of the card, low 4 bits suit of the card
// 0000: 2
// 0001: 3
// 0010: 4
// 0011: 5
// 0100: 6
// 0101: 7
// 0110: 8
// 0111: 9
// 1000: 10
// 1001: J
// 1010: Q
// 1011: K
// 1100: A
// 0001: Spade
// 0010: Heart
// 0100: Diamond
// 1000: Club
func (deck *Deck) GetBytes() []uint8 {
	cards := make([]byte, len(deck.cards))
	for i, card := range deck.cards {
		cards[i] = card.GetByte()
	}
	return cards
}

func (deck *Deck) AddCards(cards []Card) {
	deck.cards = append(deck.cards, cards...)
}

func DeckFromBytes(cardsInByte []byte) *Deck {
	cardsInDeck := len(cardsInByte)
	cards := make([]Card, cardsInDeck)
	for i, cardInByte := range cardsInByte {
		cards[i] = NewCardFromByte(cardInByte)
	}
	deck := &Deck{
		cards: cards,
	}
	return deck
}

func (deck *Deck) PrettyPrint() string {
	return CardsToString(deck.cards)
}

type CardsInAscii []string

func DeckFromScript(playerCards []CardsInAscii, flop CardsInAscii, turn Card, river Card, burnCard bool) *Deck {
	// first we are going to create a deck
	deck := NewDeck(nil)
	noOfPlayers := len(playerCards)
	for i, playerCards := range playerCards {
		for j, cardStr := range playerCards {
			deckIndex := i + j*noOfPlayers
			card := NewCard(cardStr)
			cardLoc := deck.getCardLoc(card)
			currentCard := deck.cards[deckIndex]
			deck.cards[deckIndex] = card
			deck.cards[cardLoc] = currentCard
		}
	}

	// now setup flop cards
	deckIndex := len(playerCards) * len(playerCards[0])
	if burnCard {
		// burn card
		deckIndex++
	}

	for _, cardStr := range flop {
		card := NewCard(cardStr)
		cardLoc := deck.getCardLoc(card)
		currentCard := deck.cards[deckIndex]
		deck.cards[deckIndex] = card
		deck.cards[cardLoc] = currentCard
		deckIndex++
	}

	if burnCard {
		// burn card
		deckIndex++
	}

	// turn
	cardLoc := deck.getCardLoc(turn)
	currentCard := deck.cards[deckIndex]
	deck.cards[deckIndex] = turn
	deck.cards[cardLoc] = currentCard
	deckIndex++

	if burnCard {
		// burn card
		deckIndex++
	}

	// river
	cardLoc = deck.getCardLoc(river)
	currentCard = deck.cards[deckIndex]
	deck.cards[deckIndex] = river
	deck.cards[cardLoc] = currentCard

	return deck
}

// DeckFromBoard used for setting up player cards board cards (run it twice tests)
func DeckFromBoard(playerCards []CardsInAscii, board CardsInAscii, board2 CardsInAscii, burnCard bool) *Deck {
	// first we are going to create a deck
	deck := NewDeck(nil)
	noOfPlayers := len(playerCards)
	for i, playerCards := range playerCards {
		for j, cardStr := range playerCards {
			deckIndex := i + j*noOfPlayers
			card := NewCard(cardStr)
			cardLoc := deck.getCardLoc(card)
			currentCard := deck.cards[deckIndex]
			deck.cards[deckIndex] = card
			deck.cards[cardLoc] = currentCard
		}
	}

	// now setup flop cards
	deckIndex := len(playerCards) * len(playerCards[0])

	if burnCard {
		// burn card
		deckIndex++
	}

	for _, cardStr := range board[:3] {
		card := NewCard(cardStr)
		cardLoc := deck.getCardLoc(card)
		currentCard := deck.cards[deckIndex]
		deck.cards[deckIndex] = card
		deck.cards[cardLoc] = currentCard
		deckIndex++
	}

	if burnCard { // burn card
		deckIndex++
	}

	// turn
	card := NewCard(board[3])
	cardLoc := deck.getCardLoc(card)
	currentCard := deck.cards[deckIndex]
	deck.cards[deckIndex] = card
	deck.cards[cardLoc] = currentCard
	deckIndex++

	if burnCard { // burn card
		deckIndex++
	}

	// river
	card = NewCard(board[4])
	cardLoc = deck.getCardLoc(card)
	currentCard = deck.cards[deckIndex]
	deck.cards[deckIndex] = card
	deck.cards[cardLoc] = currentCard
	deckIndex++

	if board2 != nil {
		index := 0
		if len(board2) == 5 {
			for _, cardStr := range board2[:3] {
				card := NewCard(cardStr)
				cardLoc := deck.getCardLoc(card)
				currentCard := deck.cards[deckIndex]
				deck.cards[deckIndex] = card
				deck.cards[cardLoc] = currentCard
				deckIndex++
				index++
			}

			if burnCard {
				// burn card
				deckIndex++
			}
		}

		if len(board2) >= 2 {
			// turn
			card := NewCard(board2[index])
			cardLoc := deck.getCardLoc(card)
			currentCard := deck.cards[deckIndex]
			deck.cards[deckIndex] = card
			deck.cards[cardLoc] = currentCard
			deckIndex++
			index++

			if burnCard {
				// burn card
				deckIndex++
			}
		}

		if len(board2) >= 1 {
			// river
			card = NewCard(board2[index])
			index++
			cardLoc = deck.getCardLoc(card)
			currentCard = deck.cards[deckIndex]
			deck.cards[deckIndex] = card
			deck.cards[cardLoc] = currentCard
			deckIndex++
		}
	}

	fmt.Printf("Deck: %s\n", CardsToString(deck.GetBytes()))

	return deck
}

func (deck *Deck) getCardLoc(cardToLocate Card) int {
	for i, card := range deck.cards {
		if card == cardToLocate {
			return i
		}
	}
	return -1
}

func HasSameHoleCards(playerCards map[uint32][]Card) bool {
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

// Returns
// 0   : not paired
// 1-3 : paired at flop
// 4   : paired at turn
// 5   : paired at river
func PairedAt(board []Card) int {
	m := make(map[int32]int)
	pairedAtIdx := 0
	for i := 0; i < 5; i++ {
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

func QuickShuffleCards(cards []Card) {
	rand.Shuffle(len(cards), func(i, j int) { cards[i], cards[j] = cards[j], cards[i] })
}
