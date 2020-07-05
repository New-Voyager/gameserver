package poker

import (
	"fmt"
	"strings"
)

type Card int32

var (
	intRanks [13]int32
	strRanks = "23456789TJQKA"
	primes   = []int32{2, 3, 5, 7, 11, 13, 17, 19, 23, 29, 31, 37, 41}
)

var (
	charRankToIntRank = map[uint8]int32{}
	charSuitToIntSuit = map[uint8]int32{
		's': 1, // spades
		'h': 2, // hearts
		'd': 4, // diamonds
		'c': 8, // clubs
	}
	intSuitToCharSuit = "xshxdxxxc"
)

var (
	prettySuits = map[int]string{
		1: "\u2660", // spades
		2: "\u2764", // hearts
		4: "\u2666", // diamonds
		8: "\u2663", // clubs
	}
	prettyReds = [...]int{2, 4}
)

func init() {
	for i := 0; i < 13; i++ {
		intRanks[i] = int32(i)
	}

	for i := range strRanks {
		charRankToIntRank[strRanks[i]] = intRanks[i]
	}
}

func NewCard(s string) Card {
	rankInt := charRankToIntRank[s[0]]
	suitInt := charSuitToIntSuit[s[1]]
	rankPrime := primes[rankInt]

	bitRank := int32(1) << uint32(rankInt) << 16
	suit := suitInt << 12
	rank := rankInt << 8

	return Card(bitRank | suit | rank | rankPrime)
}

func NewCardFromByte(cardByte uint8) Card {
	rankInt := int32(cardByte >> 4)
	suitInt := int32(cardByte & 0xF)
	rankPrime := primes[rankInt]

	bitRank := int32(1) << uint32(rankInt) << 16
	suit := suitInt << 12
	rank := rankInt << 8

	return Card(bitRank | suit | rank | rankPrime)
}

func (c *Card) MarshalJSON() ([]byte, error) {
	return []byte("\"" + c.String() + "\""), nil
}

func (c *Card) UnmarshalJSON(b []byte) error {
	*c = NewCard(string(b[1:3]))
	return nil
}

func (c Card) String() string {
	return string(strRanks[c.Rank()]) + string(intSuitToCharSuit[c.Suit()])
}

func (c Card) Rank() int32 {
	return (int32(c) >> 8) & 0xF
}

func (c Card) Suit() int32 {
	return (int32(c) >> 12) & 0xF
}

func (c Card) BitRank() int32 {
	return (int32(c) >> 16) & 0x1FFF
}

func (c Card) Prime() int32 {
	return int32(c) & 0x3F
}

func primeProductFromHand(cards []Card) int32 {
	product := int32(1)

	for _, card := range cards {
		product *= (int32(card) & 0xFF)
	}

	return product
}

func primeProductFromRankBits(rankBits int32) int32 {
	product := int32(1)

	for _, i := range intRanks {
		if rankBits&(1<<uint(i)) != 0 {
			product *= primes[i]
		}
	}

	return product
}

func (c Card) GetByte() uint8 {
	rank := c.Rank()
	suit := c.Suit()
	b := uint8((rank << 4) | suit)
	return b
}

func CardToString(card interface{}) string {
	switch card.(type) {
	case Card:
		c := card.(Card)
		val := c.GetByte()
		suit := int(val & 0xF)
		rank := int((val >> 4) & 0xF)
		return fmt.Sprintf("%s%s", string(strRanks[rank]), string(prettySuits[suit]))
	case uint32:
		c := card.(uint32)
		suit := int(c & 0xF)
		rank := int((c >> 4) & 0xF)
		return fmt.Sprintf("%s%s", string(strRanks[rank]), string(prettySuits[suit]))
	default:
		return ""
	}
}

func CardsToString(cards interface{}) string {
	var b strings.Builder
	b.Grow(32)
	fmt.Fprintf(&b, "[")
	switch cards.(type) {
	case []Card:
		for _, c := range cards.([]Card) {
			fmt.Fprintf(&b, " %s ", CardToString(c))
		}
	case []uint32:
		for _, c := range cards.([]uint32) {
			fmt.Fprintf(&b, " %s ", CardToString(c))
		}
	}

	fmt.Fprintf(&b, "]")
	return b.String()
}

func PrintCards(cards []Card) string {
	return CardsToString(cards)
}
