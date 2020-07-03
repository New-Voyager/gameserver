package poker

import (
	"math/rand"
	"time"
)

var fullDeck *Deck

func init() {
	fullDeck = &Deck{initializeFullCards()}
	rand.Seed(time.Now().UnixNano())
}

type Deck struct {
	cards []Card
}

func NewDeck() *Deck {
	deck := &Deck{}
	deck.Shuffle()
	return deck
}

func NewDeckNoShuffle() *Deck {
	deck := &Deck{}
	deck.cards = make([]Card, len(fullDeck.cards))
	copy(deck.cards, fullDeck.cards)
	return deck
}

func (deck *Deck) Shuffle() *Deck {
	deck.cards = make([]Card, len(fullDeck.cards))
	copy(deck.cards, fullDeck.cards)
	rand.Shuffle(len(deck.cards), func(i, j int) {
		deck.cards[i], deck.cards[j] = deck.cards[j], deck.cards[i]
	})

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
