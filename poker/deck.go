package poker

import (
	crypto_rand "crypto/rand"
	"encoding/binary"
	"math/rand"
)

var fullDeck *Deck

func init() {
	fullDeck = &Deck{cards: initializeFullCards()}
}

type Deck struct {
	cards   []Card
	randGen *rand.Rand
}

func newSeed() rand.Source {
	var b [8]byte
	_, err := crypto_rand.Read(b[:])
	if err != nil {
		panic("cannot seed math/rand package with cryptographically secure random number generator")
	}
	source := rand.NewSource(int64(binary.LittleEndian.Uint64(b[:])))
	return source
}

func NewDeck(source rand.Source) *Deck {
	if source == nil {
		var b [8]byte
		_, err := crypto_rand.Read(b[:])
		if err != nil {
			panic("cannot seed math/rand package with cryptographically secure random number generator")
		}
		source = rand.NewSource(int64(binary.LittleEndian.Uint64(b[:])))
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

func (deck *Deck) Shuffle() *Deck {
	deck.cards = make([]Card, len(fullDeck.cards))
	copy(deck.cards, fullDeck.cards)

	randGen := rand.New(newSeed())
	for i := range deck.cards {
		loc := int(randGen.Uint32() % 52)
		deck.cards[i], deck.cards[loc] = deck.cards[loc], deck.cards[i]
	}

	return deck
}

func (deck *Deck) Draw(n int) []Card {
	cards := make([]Card, n)
	copy(cards, deck.cards[:n])
	deck.cards = deck.cards[n:]
	return cards
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

func DeckFromScript(playerCards []CardsInAscii, flop CardsInAscii, turn Card, river Card) *Deck {
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

	// burn card
	deckIndex++

	for _, cardStr := range flop {
		card := NewCard(cardStr)
		cardLoc := deck.getCardLoc(card)
		currentCard := deck.cards[deckIndex]
		deck.cards[deckIndex] = card
		deck.cards[cardLoc] = currentCard
		deckIndex++
	}

	// burn card
	deckIndex++

	// turn
	cardLoc := deck.getCardLoc(turn)
	currentCard := deck.cards[deckIndex]
	deck.cards[deckIndex] = turn
	deck.cards[cardLoc] = currentCard

	// burn card
	deckIndex++

	// river
	cardLoc = deck.getCardLoc(river)
	currentCard = deck.cards[deckIndex]
	deck.cards[deckIndex] = river
	deck.cards[cardLoc] = currentCard
	//fmt.Printf("%s\n", deck.PrettyPrint())

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
