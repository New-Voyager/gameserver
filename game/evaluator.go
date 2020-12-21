package game

type EvaluatedCards struct {
	rank        int32
	cards       []byte
	playerCards []byte
	boardCards  []byte
}

func (e EvaluatedCards) GetCards() []uint32 {
	cards := make([]uint32, len(e.cards))
	for i := range e.cards {
		cards[i] = uint32(e.cards[i])
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

type HandEvaluator interface {
	Evaluate()
	GetBestPlayerCards() map[uint32]*EvaluatedCards
	GetHighHandCards() map[uint32]*EvaluatedCards
	GetWinners() map[uint32]*PotWinners
}
