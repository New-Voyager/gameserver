package poker

import (
	"fmt"
	"strings"
)

type GameType int32

const (
	Holdem GameType = 1
	Omaha  GameType = 2
	HiLo   GameType = 3
)

type Player struct {
	PlayerId int64
	Name     string
}

type PlayerHand struct {
	PlayerId int64
	Cards    []Card
}

type PokerHand struct {
	handNum     int64
	gameType    GameType
	playerHands []PlayerHand
	board       []Card
}

type PlayerResult struct {
	PlayerId     int64
	Rank         int32
	LoRank       int32
	BestCards    []Card
	LowBestCards []Card
}

type HandResult struct {
	HandNum       int64
	PlayersResult []PlayerResult
	Winners       []int64
	LoWinners     []int64
}

func (h PokerHand) PrettyPrintResult() string {
	var b strings.Builder
	b.Grow(64)
	fmt.Fprintf(&b, "Hand num: %d\n", h.handNum)
	for _, playerHand := range h.playerHands {
		fmt.Fprintf(&b, "Player: %d  %s\n",
			playerHand.PlayerId, PrintCards(playerHand.Cards))
	}
	fmt.Fprintf(&b, "Community: %s\n", PrintCards(h.board))
	return b.String()
}

func (h HandResult) PrettyPrintResult() string {
	var b strings.Builder
	b.Grow(64)
	//fmt.Fprintf(&b, "Hand num: %d\n", h.HandNum)
	for _, result := range h.PlayersResult {
		winnerStr := ""

		for _, winner := range h.Winners {
			if winner == result.PlayerId {
				winnerStr = "*** WINNER ***"
				break
			}
		}

		fmt.Fprintf(&b, "Player: %d %s Best hand: %s Rank: %s\n",
			result.PlayerId, winnerStr, PrintCards(result.BestCards),
			RankString(result.Rank))

		for _, lowWinner := range h.LoWinners {
			if lowWinner == result.PlayerId {
				fmt.Fprintf(&b, "Player: %d *** LO WINNER *** Best hand: %s Rank: %d\n",
					result.PlayerId, PrintCards(result.LowBestCards), result.LoRank)
			}
		}
	}
	return b.String()
}

func (h PokerHand) GetGameType() GameType {
	return h.gameType
}

func (h PokerHand) EvaulateHoldem() HandResult {
	handResult := HandResult{
		HandNum:       h.handNum,
		PlayersResult: make([]PlayerResult, 0, len(h.playerHands)),
		Winners:       make([]int64, 0),
	}

	var winnerRank int32 = -1
	for _, playerHand := range h.playerHands {
		allCards := make([]Card, 0)
		allCards = append(allCards, playerHand.Cards...)
		allCards = append(allCards, h.board...)
		rank, bestCards := seven(allCards...)
		playerResult := PlayerResult{
			PlayerId:  playerHand.PlayerId,
			Rank:      rank,
			BestCards: bestCards,
		}
		handResult.PlayersResult = append(handResult.PlayersResult, playerResult)
		if winnerRank == -1 || rank < winnerRank {
			winnerRank = rank
		}
	}

	for _, playerResult := range handResult.PlayersResult {
		if playerResult.Rank == winnerRank {
			handResult.Winners = append(handResult.Winners, playerResult.PlayerId)
		}
	}

	return handResult
}

func (h PokerHand) EvaulateOmaha() HandResult {
	handResult := HandResult{
		HandNum:       h.handNum,
		PlayersResult: make([]PlayerResult, 0, len(h.playerHands)),
		Winners:       make([]int64, 0),
		LoWinners:     make([]int64, 0),
	}

	var winnerRank int32 = -1
	var lowWinnerRank int32 = 0x7FFFFFFF

	for _, playerHand := range h.playerHands {
		omahaResult := EvaluateOmaha(playerHand.Cards, h.board)
		playerResult := PlayerResult{
			PlayerId:  playerHand.PlayerId,
			Rank:      omahaResult.HiRank,
			BestCards: omahaResult.HiCards,
		}

		if omahaResult.LowFound {
			playerResult.LowBestCards = omahaResult.LowCards
			playerResult.LoRank = omahaResult.LowRank
		}

		handResult.PlayersResult = append(handResult.PlayersResult, playerResult)
		if winnerRank == -1 || omahaResult.HiRank < winnerRank {
			winnerRank = omahaResult.HiRank
		}

		if omahaResult.LowFound && omahaResult.LowRank < lowWinnerRank {
			lowWinnerRank = omahaResult.LowRank
		}
	}

	for _, playerResult := range handResult.PlayersResult {
		if playerResult.Rank == winnerRank {
			handResult.Winners = append(handResult.Winners, playerResult.PlayerId)
		}

		if playerResult.LoRank == lowWinnerRank {
			handResult.LoWinners = append(handResult.LoWinners, playerResult.PlayerId)
		}
	}

	return handResult
}

func NewHand(handNum int64, playerHands []PlayerHand, board []Card) PokerHand {
	return PokerHand{
		handNum:     handNum,
		playerHands: playerHands,
		board:       board,
	}
}

type Table interface {
	Deal(handNum int64) PokerHand
}

type PokerTable struct {
	deck1        *Deck
	deck2        *Deck
	lastDeckUsed int32
	players      []Player
	gameType     GameType
}

func NewHoldemTable(players []Player) *PokerTable {
	return &PokerTable{
		deck1:        NewDeck(),
		deck2:        NewDeck(),
		lastDeckUsed: 1,
		players:      players,
		gameType:     Holdem,
	}
}

func NewOmahaTable(players []Player) *PokerTable {
	return &PokerTable{
		deck1:        NewDeck(),
		deck2:        NewDeck(),
		lastDeckUsed: 1,
		players:      players,
		gameType:     Omaha,
	}
}

func NewOmahaHiLoTable(players []Player) *PokerTable {
	return &PokerTable{
		deck1:        NewDeck(),
		deck2:        NewDeck(),
		lastDeckUsed: 1,
		players:      players,
		gameType:     HiLo,
	}
}

func (p *PokerTable) Deal(handNum int64) PokerHand {
	if p.gameType == Holdem {
		return p.DealHoldem(handNum)
	} else {
		return p.DealOmaha(handNum)
	}
}

func (p *PokerTable) DealHoldem(handNum int64) PokerHand {
	deckToUse := p.deck1
	if p.lastDeckUsed == 1 {
		deckToUse = p.deck2
		p.lastDeckUsed = 2
	} else {
		deckToUse = p.deck1
		p.lastDeckUsed = 1
	}

	// shuffle deck
	deckToUse.Shuffle()

	// initiate a poker hand
	hand := PokerHand{
		handNum:     handNum,
		gameType:    p.gameType,
		playerHands: make([]PlayerHand, len(p.players)),
		board:       make([]Card, 5),
	}

	for cardNum := 0; cardNum < 2; cardNum++ {
		for i, player := range p.players {
			if hand.playerHands[i].PlayerId == 0 {
				hand.playerHands[i].PlayerId = player.PlayerId
				hand.playerHands[i].Cards = make([]Card, 2)
			}
			hand.playerHands[i].Cards[cardNum] = deckToUse.Draw(1)[0]
		}
	}

	boardCards := deckToUse.Draw(3)
	deckToUse.Draw(1)
	boardCards = append(boardCards, deckToUse.Draw(1)...)
	deckToUse.Draw(1)
	boardCards = append(boardCards, deckToUse.Draw(1)...)
	hand.board = boardCards
	return hand
}

func (p *PokerTable) DealOmaha(handNum int64) PokerHand {
	deckToUse := p.deck1
	if p.lastDeckUsed == 1 {
		deckToUse = p.deck2
		p.lastDeckUsed = 2
	} else {
		deckToUse = p.deck1
		p.lastDeckUsed = 1
	}

	// shuffle deck
	deckToUse.Shuffle()

	// initiate a poker hand
	hand := PokerHand{
		handNum:     handNum,
		gameType:    p.gameType,
		playerHands: make([]PlayerHand, len(p.players)),
		board:       make([]Card, 5),
	}

	holeCards := 4
	for cardNum := 0; cardNum < holeCards; cardNum++ {
		for i, player := range p.players {
			if hand.playerHands[i].PlayerId == 0 {
				hand.playerHands[i].PlayerId = player.PlayerId
				hand.playerHands[i].Cards = make([]Card, holeCards)
			}
			hand.playerHands[i].Cards[cardNum] = deckToUse.Draw(1)[0]
		}
	}

	boardCards := deckToUse.Draw(3)
	deckToUse.Draw(1)
	boardCards = append(boardCards, deckToUse.Draw(1)...)
	deckToUse.Draw(1)
	boardCards = append(boardCards, deckToUse.Draw(1)...)
	hand.board = boardCards
	return hand
}
