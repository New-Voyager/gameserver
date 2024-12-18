package game

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/pkg/errors"
	"voyager.com/logging"
	"voyager.com/server/poker"
	"voyager.com/server/util"
	"voyager.com/server/util/hashing"
)

var handLogger = logging.GetZeroLogger("game::hand", nil)
var preFlopBets = []int{3, 5, 10}     // big blinds
var postFlopBets = []int{30, 50, 100} // % of pot
var raiseOptions = []int{2, 3, 5}     // raise times
var ploPreFlopBets = []int{2, 5, 10}  // big blinds

func (h *HandState) initializeBettingRound() {
	maxSeats := h.MaxSeats + 1 // dealer seat
	h.RoundState = make(map[uint32]*RoundState)
	h.RoundState[uint32(HandStatus_PREFLOP)] = &RoundState{
		PlayerBalance: make(map[uint32]float64),
		Betting:       &SeatBetting{SeatBet: make([]float64, maxSeats)},
		BetIndex:      1,
	}
	h.RoundState[uint32(HandStatus_FLOP)] = &RoundState{
		PlayerBalance: make(map[uint32]float64),
		Betting:       &SeatBetting{SeatBet: make([]float64, maxSeats)},
		BetIndex:      1,
	}
	h.RoundState[uint32(HandStatus_TURN)] = &RoundState{
		PlayerBalance: make(map[uint32]float64),
		Betting:       &SeatBetting{SeatBet: make([]float64, maxSeats)},
		BetIndex:      1,
	}
	h.RoundState[uint32(HandStatus_RIVER)] = &RoundState{
		PlayerBalance: make(map[uint32]float64),
		Betting:       &SeatBetting{SeatBet: make([]float64, maxSeats)},
		BetIndex:      1,
	}

	// setup player acted tracking
	h.PlayersActed = make([]*PlayerActRound, maxSeats) // seat 0 is dealer
	for i := 0; i < int(maxSeats); i++ {
		if i == 0 {
			h.PlayersActed[i] = &PlayerActRound{
				Action: ACTION_EMPTY_SEAT,
				Amount: 0.0,
			}
		} else {
			h.PlayersActed[i] = &PlayerActRound{
				Action: ACTION_NOT_ACTED,
				Amount: 0.0,
			}
		}
	}
	h.resetPlayerActions()
}

// WARNING: Keep this method idempotent (no mutate HandState). It could get retried in case of reshuffle.
func (h *HandState) board(deck *poker.Deck) ([]poker.Card, int) {
	numCardsUsed := 0
	boardCards := make([]poker.Card, 0)
	if h.BurnCards {
		deck.Draw(1)
		numCardsUsed++
	}

	// FLOP
	boardCards = append(boardCards, deck.Draw(3)...)
	numCardsUsed += 3

	if h.BurnCards {
		deck.Draw(1)
		numCardsUsed++
	}

	// TURN
	boardCards = append(boardCards, deck.Draw(1)...)
	numCardsUsed++

	if h.BurnCards {
		deck.Draw(1)
		numCardsUsed++
	}

	// RIVER
	boardCards = append(boardCards, deck.Draw(1)...)
	numCardsUsed++

	return boardCards, numCardsUsed
}

func (h *HandState) initialize(testGameConfig *TestGameConfig,
	newHandInfo *NewHandInfo,
	testHandSetup *TestHandSetup,
	buttonPos uint32, sbPos uint32, bbPos uint32,
	playersInSeats []SeatPlayer,
	chipUnit ChipUnit) error {

	h.Tournament = newHandInfo.Tournament
	if h.Tournament {
		h.HandNum = newHandInfo.HandNum
	}

	h.ChipUnit = chipUnit
	if playersInSeats == nil {
		playersInSeats = newHandInfo.PlayersInSeats
	}

	// settle players in the seats
	if testGameConfig != nil {
		h.PlayersInSeats = make([]*PlayerInSeatState, testGameConfig.MaxPlayers+1) // seat 0 is dealer
		h.GameType = testGameConfig.GameType
		h.ActiveSeats = make([]uint64, testGameConfig.MaxPlayers+1)
		h.ActionTime = uint32(testGameConfig.ActionTime)
	} else {
		h.PlayersInSeats = make([]*PlayerInSeatState, newHandInfo.MaxPlayers+1) // seat 0 is dealer
		h.GameType = newHandInfo.GameType
		h.ActiveSeats = make([]uint64, newHandInfo.MaxPlayers+1)
		h.ActionTime = uint32(newHandInfo.ActionTime)

		// Save encryption keys in case we crash mid hand.
		h.EncryptionKeys = make(map[uint64]string)
		for _, p := range newHandInfo.PlayersInSeats {
			h.EncryptionKeys[p.PlayerID] = p.EncryptionKey
		}
	}
	h.NoActiveSeats = 0
	h.PlayerStats = make(map[uint64]*PlayerStats)
	h.TimeoutStats = make(map[uint64]*TimeoutStats)
	h.PotContribution = make(map[uint32]float64)

	// update active seats with players who are playing
	for seatNo, playerInSeat := range playersInSeats {
		if playerInSeat.PlayerID != 0 {
			h.PlayersInSeats[playerInSeat.SeatNo] = &PlayerInSeatState{
				SeatNo:            playerInSeat.SeatNo,
				Status:            playerInSeat.Status,
				Stack:             playerInSeat.Stack,
				PlayerId:          playerInSeat.PlayerID,
				Name:              playerInSeat.Name,
				BuyInExpTime:      playerInSeat.BuyInTimeExpAt,
				BreakExpTime:      playerInSeat.BreakTimeExpAt,
				Inhand:            playerInSeat.Inhand,
				PostedBlind:       playerInSeat.PostedBlind,
				RunItTwice:        playerInSeat.RunItTwice,
				MissedBlind:       playerInSeat.MissedBlind,
				MuckLosingHand:    playerInSeat.MuckLosingHand,
				ButtonStraddle:    playerInSeat.ButtonStraddle,
				ButtonStraddleBet: playerInSeat.ButtontStraddleBet,
			}
			h.PlayerStats[playerInSeat.PlayerID] = &PlayerStats{InPreflop: true}
			h.TimeoutStats[playerInSeat.PlayerID] = &TimeoutStats{
				ConsecutiveActionTimeouts: 0,
				ActedAtLeastOnce:          false,
			}
		} else {
			if playerInSeat.SeatNo == 0 {
				playerInSeat.SeatNo = uint32(seatNo)
			}
			openSeat := playerInSeat.OpenSeat
			if newHandInfo == nil {
				openSeat = true
			}
			h.PlayersInSeats[playerInSeat.SeatNo] = &PlayerInSeatState{
				SeatNo:   playerInSeat.SeatNo,
				OpenSeat: openSeat,
				Inhand:   false,
			}
		}

		if h.PlayersInSeats[playerInSeat.SeatNo].Inhand {
			h.NoActiveSeats++
			h.ActiveSeats[playerInSeat.SeatNo] = playerInSeat.PlayerID
		}
		h.PotContribution[uint32(seatNo)] = 0
	}

	// if there is no active player in the button pos (panic)
	if !h.PlayersInSeats[buttonPos].Inhand {
		handLogger.Error().
			Uint64(logging.GameIDKey, h.GetGameId()).
			Uint32(logging.HandNumKey, h.GetHandNum()).
			Msgf("Button Pos: %d does not have any active seat: %v. This is a dead button", buttonPos, h.PlayersInSeats)
	}

	h.HandStats = &HandStats{}
	if testGameConfig != nil {
		h.MaxSeats = uint32(testGameConfig.MaxPlayers)
		h.SmallBlind = util.ChipsToCents(testGameConfig.SmallBlind)
		h.BigBlind = util.ChipsToCents(testGameConfig.BigBlind)
		h.Ante = util.ChipsToCents(testGameConfig.Ante)
		h.Straddle = util.ChipsToCents(testGameConfig.StraddleBet)
		h.RakePercentage = float64(testGameConfig.RakePercentage)
		h.RakeCap = util.ChipsToCents(testGameConfig.RakeCap)
		h.ButtonPos = buttonPos
		h.PlayersActed = make([]*PlayerActRound, h.MaxSeats+1)
		h.BringIn = util.ChipsToCents(testGameConfig.BringIn)
		h.MandatoryStraddle = testGameConfig.MandatoryStraddle
	} else {
		h.MaxSeats = uint32(newHandInfo.MaxPlayers)
		h.SmallBlind = float64(newHandInfo.SmallBlind)
		h.BigBlind = float64(newHandInfo.BigBlind)
		h.Ante = float64(newHandInfo.Ante)
		h.Straddle = float64(newHandInfo.StraddleBet)
		h.RakePercentage = float64(newHandInfo.RakePercentage)
		h.RakeCap = float64(newHandInfo.RakeCap)
		h.ButtonPos = newHandInfo.ButtonPos
		h.PlayersActed = make([]*PlayerActRound, h.MaxSeats+1)
		h.BringIn = float64(newHandInfo.BringIn)
		h.RunItTwiceTimeout = newHandInfo.RunItTwiceTimeout
		h.HighHandTracked = newHandInfo.HighHandTracked
		h.HighHandRank = newHandInfo.HighHandRank
		h.MandatoryStraddle = newHandInfo.MandatoryStraddle
	}
	h.BurnCards = false
	h.CurrentActionNum = 0
	if h.RunItTwiceTimeout == 0 {
		h.RunItTwiceTimeout = 10
	}

	if newHandInfo != nil {
		h.BombPot = newHandInfo.BombPot
		h.BombPotBet = newHandInfo.BombPotBet
		h.DoubleBoard = newHandInfo.DoubleBoard
	}

	if testHandSetup != nil {
		h.DoubleBoard = testHandSetup.DoubleBoard
		h.BombPot = testHandSetup.BombPot
		if h.BombPot {
			h.BombPotBet = util.ChipsToCents(testHandSetup.BombPotBet)
		}
	}

	if testHandSetup != nil {
		h.IncludeStatsInResult = testHandSetup.IncludeStats
	}

	// setup data structure to handle betting rounds
	h.initializeBettingRound()

	// if the players don't have money less than the blinds
	// don't let them play
	if sbPos != 0 && bbPos != 0 {
		h.SmallBlindPos = sbPos
		h.BigBlindPos = bbPos
	} else {
		// TODO: make sure small blind is still there
		// if small blind left the game, we can have dead small
		// to make it simple, we will make new players to always to post or wait for the big blind
		sb, bb, err := h.getBlindPos()
		if err != nil {
			return errors.Wrap(err, "Error while getting blind positions")
		}
		h.SmallBlindPos, h.BigBlindPos = sb, bb
	}

	h.BalanceBeforeHand = make([]*PlayerBalance, 0)
	postedBlinds := make([]uint32, 0)

	// also populate current balance of the players in the table
	for _, playerInSeat := range h.PlayersInSeats {
		if !playerInSeat.Inhand {
			// player ID is 0, meaning an open seat or a dealer.
			continue
		}
		if playerInSeat.PostedBlind {
			postedBlinds = append(postedBlinds, playerInSeat.SeatNo)
		}
		h.BalanceBeforeHand = append(h.BalanceBeforeHand,
			&PlayerBalance{SeatNo: playerInSeat.SeatNo,
				PlayerId: playerInSeat.PlayerId,
				Balance:  playerInSeat.Stack})
	}

	var deck *poker.Deck
	var b1Cards, b2Cards []poker.Card
	var playerCardsMap map[uint32][]poker.Card
	var numCardsUsed int

	if testHandSetup == nil || testHandSetup.PlayerCards == nil {
		// Real game or auto-play script.
		playerCardsMap, b1Cards, b2Cards, deck, numCardsUsed = h.shuffleAndPickCards()
		reshuffles := 0
		for reshuffles < 10 && TooManyHighHands(newHandInfo, h.GameType, playerCardsMap, b1Cards, b2Cards) {
			reshuffles++
			handLogger.Debug().
				Uint64(logging.GameIDKey, h.GetGameId()).
				Uint32(logging.HandNumKey, h.GetHandNum()).
				Msgf("Reshuffling for too many high hands. TotalHands=%d StraightFlushCount=%d FourKindCount=%d StraightFlushAllowed=%v FourKindAllowed=%v", newHandInfo.TotalHands, newHandInfo.StraightFlushCount, newHandInfo.FourKindCount, newHandInfo.StraightFlushAllowed, newHandInfo.FourKindAllowed)
			playerCardsMap, b1Cards, b2Cards, deck, numCardsUsed = h.shuffleAndPickCards()
		}
		h.Deck = deck.GetBytes()
	} else {
		// running script test, botrunner script, etc.
		// We're preconfiguring the deck according to the test script.
		playerCards := make([]poker.CardsInAscii, 0)
		for _, seatCards := range testHandSetup.PlayerCards {
			playerCards = append(playerCards, seatCards.Cards)
		}
		if testHandSetup.Board != nil {
			deck = poker.DeckFromBoard(playerCards, testHandSetup.Board, testHandSetup.Board2, false)
		} else {
			// arrange deck
			deck = poker.DeckFromScript(
				playerCards,
				testHandSetup.Flop,
				poker.NewCard(testHandSetup.Turn),
				poker.NewCard(testHandSetup.River),
				false /* burn card */)
		}

		// Ordering of the following 2 lines seems significant for run-it-twice unit tests.
		h.Deck = deck.GetBytes()
		playerCardsMap, b1Cards, b2Cards, numCardsUsed = h.pickScriptedCardsFromDeck(deck, testHandSetup.PlayerCardsBySeat)
	}

	h.DeckIndex = uint32(numCardsUsed)

	h.PlayersCards = make(map[uint32][]byte)
	for seatNo, cards := range playerCardsMap {
		cardsInBytes := make([]byte, 0)
		for _, c := range cards {
			cardsInBytes = append(cardsInBytes, c.GetByte())
		}
		h.PlayersCards[seatNo] = cardsInBytes
	}

	b1ByteCards := poker.CardsToByteCards(b1Cards)
	h.BoardCards = b1ByteCards
	h.NoOfBoards = 1
	h.Boards = make([]*Board, 0)
	board1 := &Board{
		BoardNo: 1,
		Cards:   poker.ByteCardsToUint32Cards(b1ByteCards),
	}
	h.Boards = append(h.Boards, board1)
	handLogger.Debug().
		Uint64(logging.GameIDKey, h.GetGameId()).
		Uint32(logging.HandNumKey, h.GetHandNum()).
		Msgf("Board1: %s", poker.CardsToString(h.BoardCards))

	if h.DoubleBoard {
		h.NoOfBoards = 2
		b2ByteCards := poker.CardsToByteCards(b2Cards)
		board2 := &Board{
			BoardNo: 2,
			Cards:   poker.ByteCardsToUint32Cards(b2ByteCards),
		}
		h.Boards = append(h.Boards, board2)
		handLogger.Debug().
			Uint64(logging.GameIDKey, h.GetGameId()).
			Uint32(logging.HandNumKey, h.GetHandNum()).
			Msgf("Board2: %s", poker.CardsToString(b2ByteCards))
	}

	// setup main pot
	h.Pots = make([]*SeatsInPots, 0)
	mainPot := initializePot(int(h.MaxSeats) + 1)
	h.Pots = append(h.Pots, mainPot)
	h.RakePaid = make(map[uint64]float64)

	// setup hand for preflop
	h.setupPreflop(postedBlinds)
	return nil
}

func TooManyHighHands(newHandInfo *NewHandInfo, gameType GameType, playerCards map[uint32][]poker.Card, b1Cards []poker.Card, b2Cards []poker.Card) bool {
	if !newHandInfo.StraightFlushAllowed {
		return AnyoneHasHighHand(playerCards, b1Cards, gameType, poker.MaxStraightFlush) ||
			AnyoneHasHighHand(playerCards, b2Cards, gameType, poker.MaxStraightFlush)
	}

	if !newHandInfo.FourKindAllowed {
		return AnyoneHasHighHand(playerCards, b1Cards, gameType, poker.MaxFourOfAKind) ||
			AnyoneHasHighHand(playerCards, b2Cards, gameType, poker.MaxFourOfAKind)
	}

	return false
}

func (h *HandState) shuffleAndPickCards() (map[uint32][]poker.Card, []poker.Card, []poker.Card, *poker.Deck, int) {
	deck := poker.NewDeck().Shuffle()
	tmpDeck := poker.CopyDeck(deck)
	playerCardsMap, b1Cards, b2Cards, numCardsUsed := h.drawFromDeck(tmpDeck, nil)

	handNum := h.HandNum
	if handNum <= 10 {
		for AnyoneHasHighHand(playerCardsMap, b1Cards, h.GameType, poker.MaxFourOfAKind) ||
			AnyoneHasHighHand(playerCardsMap, b2Cards, h.GameType, poker.MaxFourOfAKind) {
			deck.Shuffle()
			tmpDeck = poker.CopyDeck(deck)
			playerCardsMap, b1Cards, b2Cards, numCardsUsed = h.drawFromDeck(tmpDeck, nil)
		}
	} else if handNum > 10 && handNum <= 20 {
		if AnyoneHasHighHand(playerCardsMap, b1Cards, h.GameType, poker.MaxFourOfAKind) ||
			AnyoneHasHighHand(playerCardsMap, b2Cards, h.GameType, poker.MaxFourOfAKind) {
			if rand.Int()%2 == 0 {
				deck.Shuffle()
				tmpDeck = poker.CopyDeck(deck)
				playerCardsMap, b1Cards, b2Cards, numCardsUsed = h.drawFromDeck(tmpDeck, nil)
			}
		}
	}

	if !AnyoneHasHighHand(playerCardsMap, b1Cards, h.GameType, poker.MaxFullHouse) &&
		!AnyoneHasHighHand(playerCardsMap, b2Cards, h.GameType, poker.MaxFullHouse) {
		if rand.Int()%2 == 0 {
			maxReshuffleAllowed := 1
			reshuffles := 0
			for AnyoneHasHighHand(playerCardsMap, b2Cards, h.GameType, poker.MaxFullHouse) ||
				(reshuffles < maxReshuffleAllowed && NeedReshuffle(playerCardsMap, b1Cards, b2Cards, h.GameType)) {
				reshuffles++
				deck.Shuffle()
				tmpDeck = poker.CopyDeck(deck)
				playerCardsMap, b1Cards, b2Cards, numCardsUsed = h.drawFromDeck(tmpDeck, nil)
			}
		}
	}

	if !h.DoubleBoard {
		numCardsUsed -= len(b2Cards)
		b2Cards = nil
	}

	return playerCardsMap, b1Cards, b2Cards, deck, numCardsUsed
}

func NeedReshuffle(playerCardsMap map[uint32][]poker.Card, board1 []poker.Card, board2 []poker.Card, gameType GameType) bool {
	shouldReshuffle := false
	if HasSameHoleCards(playerCardsMap) ||
		IsBoardPaired(board1) ||
		IsBoardPaired(board2) {
		shouldReshuffle = true
	}
	return shouldReshuffle
}

func (h *HandState) pickScriptedCardsFromDeck(deck *poker.Deck, preAssignedCards map[uint32]*GameSetupSeatCards) (map[uint32][]poker.Card, []poker.Card, []poker.Card, int) {
	playerCardsMap, b1Cards, b2Cards, numCardsUsed := h.drawFromDeck(deck, preAssignedCards)
	if !h.DoubleBoard {
		numCardsUsed -= len(b2Cards)
		b2Cards = nil
	}
	return playerCardsMap, b1Cards, b2Cards, numCardsUsed
}

func (h *HandState) drawFromDeck(deck *poker.Deck, preAssignedCards map[uint32]*GameSetupSeatCards) (map[uint32][]poker.Card, []poker.Card, []poker.Card, int) {
	var playerCardsMap map[uint32][]poker.Card
	var b1Cards, b2Cards []poker.Card
	var numCardsUsed, cardsDrawn, cardsNeeded int
	playerCardsMap, numCardsUsed = h.getPlayersCards(deck, preAssignedCards)
	b1Cards, cardsDrawn = h.board(deck)
	numCardsUsed += cardsDrawn

	cardsNeeded = 5
	if h.BurnCards {
		cardsNeeded += 3
	}

	// Any time we have enough cards left in the deck, just setup the second
	// board anyway in case we need it for run-it-twice, etc.
	if deck.Size() >= cardsNeeded {
		b2Cards, cardsDrawn = h.board(deck)
		if cardsDrawn != cardsNeeded {
			// Shouldn't get here. Just guarding against future changes.
			panic("Error in drawing second board. cardsDrawn != cardsNeeded")
		}
		numCardsUsed += cardsDrawn
	}

	return playerCardsMap, b1Cards, b2Cards, numCardsUsed
}

func (h *HandState) setupRound(state HandStatus) {

	var log *HandActionLog
	switch h.CurrentState {
	case HandStatus_FLOP:
		log = h.FlopActions
	case HandStatus_TURN:
		log = h.TurnActions
	case HandStatus_RIVER:
		log = h.RiverActions
	case HandStatus_PREFLOP:
		log = h.PreflopActions
	}

	// track main pot value as starting value
	if log != nil {
		log.PotStart = h.Pots[0].Pot
		log.Pots = make([]float64, 0)
		for _, pot := range h.Pots {
			if pot.Pot != 0.0 {
				log.Pots = append(log.Pots, pot.Pot)
			}
		}

		log.SeatsPots = make([]*SeatsInPots, 0)
		if h.Pots != nil && len(h.Pots) > 0 {
			for _, pot := range h.Pots {
				if len(pot.Seats) > 0 {
					seats := make([]uint32, len(pot.Seats))
					for i, seat := range pot.Seats {
						seats[i] = seat
					}
					seatPot := &SeatsInPots{
						Seats: seats,
						Pot:   pot.Pot,
					}
					log.SeatsPots = append(log.SeatsPots, seatPot)
				}
			}
		}
	}

	h.RoundState[uint32(state)].PlayerBalance = make(map[uint32]float64)
	roundState := h.RoundState[uint32(state)]
	for seatNo, player := range h.PlayersInSeats {
		if seatNo == 0 || !player.Inhand {
			continue
		}
		roundState.PlayerBalance[uint32(seatNo)] = player.Stack
	}
}

func (h *HandState) setupPreflop(postedBlinds []uint32) {
	h.CurrentState = HandStatus_PREFLOP

	// set next action information
	h.PreflopActions = &HandActionLog{Actions: make([]*HandAction, 0)}
	h.FlopActions = &HandActionLog{Actions: make([]*HandAction, 0)}
	h.TurnActions = &HandActionLog{Actions: make([]*HandAction, 0)}
	h.RiverActions = &HandActionLog{Actions: make([]*HandAction, 0)}
	h.CurrentRaise = 0
	// initialize all-in players list
	h.AllInPlayers = make([]uint32, h.MaxSeats+1) // seat 0 is dealer
	for idx := range h.AllInPlayers {
		h.AllInPlayers[idx] = 0
	}

	// add antes here
	h.PreflopActions.PotStart = 0

	// add ante here
	// the players should have ante to be here
	if h.Ante > 0 {
		pot := h.PreflopActions.PotStart
		collectedAnte := float64(0)
		seatsInPot := make([]uint32, 0)
		for seatNoIdx, playerID := range h.ActiveSeats {
			if playerID == 0 {
				continue
			}
			seatNo := uint32(seatNoIdx)
			player := h.PlayersInSeats[seatNo]

			if player.Stack <= h.Ante {
				// we need to handle when the player has less than the ante
				// player is all-in
				h.actionReceived(&HandAction{
					SeatNo: seatNo,
					Action: ACTION_ALLIN,
					Amount: player.Stack,
				}, 0)
				pot += player.Stack
				collectedAnte += player.Stack
				player.Stack = 0
			} else {
				player.Stack -= h.Ante
				// update player balance
				pot += h.Ante
				collectedAnte += h.Ante
			}
			seatsInPot = append(seatsInPot, seatNo)
		}
		h.CollectedAnte = collectedAnte
		h.PreflopActions.PotStart = pot
		h.Pots[0].Pot = pot
		h.Pots[0].Seats = seatsInPot
	}

	h.setupRound(HandStatus_PREFLOP)

	if h.BombPot {
		for seatNoIdx, playerID := range h.ActiveSeats {
			if playerID == 0 {
				continue
			}
			seatNo := uint32(seatNoIdx)
			h.actionReceived(&HandAction{
				SeatNo: seatNo,
				Action: ACTION_BOMB_POT_BET,
				Amount: h.BombPotBet,
			}, 0)
		}
		// move to flop round
		h.settleRound()
		h.setupRound(HandStatus_FLOP)
	} else {
		// button player
		buttonPlayer := h.PlayersInSeats[h.ButtonPos]
		h.ButtonStraddle = false
		if buttonPlayer.ButtonStraddle && buttonPlayer.Stack > 2*h.BigBlind {
			h.ButtonStraddle = true
		}

		for _, seatNo := range postedBlinds {
			// skip natural big blind position
			if seatNo == h.BigBlindPos {
				continue
			}
			// button is straddling, no need to post blind
			if h.ButtonStraddle && seatNo == h.ButtonPos {
				continue
			}
			h.actionReceived(&HandAction{
				SeatNo: seatNo,
				Action: ACTION_POST_BLIND,
				Amount: h.BigBlind,
			}, 0)
		}

		playerInSB := h.PlayersInSeats[h.SmallBlindPos]
		if playerInSB.PlayerId != 0 && playerInSB.Inhand {
			if playerInSB.Stack <= h.SmallBlind {
				h.actionReceived(&HandAction{
					SeatNo: h.SmallBlindPos,
					Action: ACTION_ALLIN,
					Amount: playerInSB.Stack,
				}, 0)
			} else {
				h.actionReceived(&HandAction{
					SeatNo: h.SmallBlindPos,
					Action: ACTION_SB,
					Amount: h.SmallBlind,
				}, 0)
			}
		}

		playerInBB := h.PlayersInSeats[h.BigBlindPos]
		if !playerInBB.Inhand {
			panic("playerInBB.Inhand is false")
		}
		if playerInBB.PlayerId != 0 {
			if playerInBB.Stack <= h.BigBlind {
				h.actionReceived(&HandAction{
					SeatNo: h.BigBlindPos,
					Action: ACTION_ALLIN,
					Amount: playerInBB.Stack,
				}, 0)
			} else {
				h.actionReceived(&HandAction{
					SeatNo: h.BigBlindPos,
					Action: ACTION_BB,
					Amount: h.BigBlind,
				}, 0)
			}
		}

		if h.ButtonStraddle {
			playerInButton := h.PlayersInSeats[h.ButtonPos]
			if playerInButton.Inhand {
				buttonStraddleBet := 2 * h.BigBlind
				if playerInButton.ButtonStraddleBet > 2 {
					buttonStraddleBet = float64(playerInButton.ButtonStraddleBet) * h.BigBlind
				}
				if playerInButton.PlayerId != 0 && playerInButton.Stack >= buttonStraddleBet {
					if playerInButton.Stack == buttonStraddleBet {
						h.actionReceived(&HandAction{
							SeatNo: h.ButtonPos,
							Action: ACTION_ALLIN,
							Amount: playerInBB.Stack,
						}, 0)
					} else {
						h.actionReceived(&HandAction{
							SeatNo: h.ButtonPos,
							Action: ACTION_STRADDLE,
							Amount: buttonStraddleBet,
						}, 0)
						h.Straddle = buttonStraddleBet
					}
				}
			}
		}
	}

	h.ActionCompleteAtSeat = h.BigBlindPos

	// track whether the player is active in this round or not
	for seatNo := 1; seatNo <= int(h.GetMaxSeats()); seatNo++ {
		player := h.PlayersInSeats[seatNo]
		if !player.Inhand {
			continue
		}
		player.Round = HandStatus_PREFLOP
	}
}

func (h *HandState) resetPlayerActions() {
	for seatNo, _ := range h.PlayersInSeats {
		if seatNo == 0 || h.ActiveSeats[seatNo] == 0 {
			h.PlayersActed[seatNo] = &PlayerActRound{
				Action: ACTION_EMPTY_SEAT,
			}
			continue
		}
		// if player is all in, then don't reset
		if h.PlayersActed[seatNo].Action == ACTION_ALLIN {
			continue
		}
		h.PlayersActed[seatNo] = &PlayerActRound{
			Action: ACTION_NOT_ACTED,
		}
	}
}

func (h *HandState) acted(seatNo uint32, action ACTION, amount float64) {
	bettingState := h.RoundState[uint32(h.CurrentState)]
	h.PlayersActed[seatNo].Action = action
	h.PlayersActed[seatNo].ActedBetIndex = bettingState.BetIndex
	if action == ACTION_FOLD {
		h.ActiveSeats[seatNo] = 0
		h.NoActiveSeats--
	} else {
		h.PlayersActed[seatNo].Amount = amount
		if amount > h.CurrentRaise {
			h.PlayersActed[seatNo].RaiseAmount = amount - h.CurrentRaise
		} else {
			h.PlayersActed[seatNo].RaiseAmount = h.CurrentRaiseDiff
		}
		playerID := h.ActiveSeats[int(seatNo)]
		if playerID == 0 {
			return
		}
		// this player put money in the pot
		if h.CurrentState == HandStatus_PREFLOP {
			if action == ACTION_CALL ||
				action == ACTION_BET ||
				action == ACTION_RAISE ||
				action == ACTION_STRADDLE ||
				action == ACTION_BOMB_POT_BET {
				h.PlayerStats[playerID].Vpip = true
			}

			if amount > h.CurrentRaise &&
				action != ACTION_BB &&
				action != ACTION_SB && action != ACTION_POST_BLIND {
				h.PlayerStats[playerID].PreflopRaise = true
			}
		} else if h.CurrentState == HandStatus_FLOP {
			if amount > h.CurrentRaise {
				h.PlayerStats[playerID].PostflopRaise = true
			}
		}

		// if player bets or raised, determine whether this is 3bet or cbet
		if amount > h.CurrentRaise {
			if h.CurrentState == HandStatus_PREFLOP {
				if h.CurrentRaise == h.BigBlind { // we need to handle straddle
					h.PlayerStats[playerID].ThreeBet = true
				}
			} else if h.CurrentState == HandStatus_FLOP {
				if h.PlayerStats[playerID].ThreeBet {
					// continuation bet
					h.PlayerStats[playerID].Cbet = true
				}
			}
		}

		if action == ACTION_ALLIN {
			h.AllInPlayers[seatNo] = 1
			h.PlayerStats[playerID].Allin = true
		}
	}

	activeSeats := 0
	player1 := uint64(0)
	player2 := uint64(0)
	for _, playerID := range h.ActiveSeats {
		if playerID != 0 {
			activeSeats++
		}
		if player1 == 0 {
			player1 = playerID
		} else if player2 == 0 {
			player2 = playerID
		}
	}

	if activeSeats == 2 {
		h.HeadsupPlayers = make([]uint64, 0)
		h.HeadsupPlayers = append(h.HeadsupPlayers, player2)
		h.HeadsupPlayers = append(h.HeadsupPlayers, player1)

		// headsup
		h.PlayerStats[player1].Headsup = true
		h.PlayerStats[player1].HeadsupPlayer = player2

		h.PlayerStats[player2].Headsup = true
		h.PlayerStats[player2].HeadsupPlayer = player1
	}
}

func (h *HandState) hasEveryOneActed() bool {
	allActed := true

	for seatNo, player := range h.PlayersInSeats {
		if seatNo == 0 || !player.Inhand {
			continue
		}

		action := h.PlayersActed[seatNo].Action
		if action == ACTION_EMPTY_SEAT ||
			action == ACTION_FOLD ||
			action == ACTION_ALLIN {
			continue
		}

		// if big blind or straddle hasn't acted, return false
		if action == ACTION_BB || action == ACTION_STRADDLE || action == ACTION_NOT_ACTED {
			return false
		}

		if h.PlayersActed[seatNo].Amount == h.CurrentRaise {
			// if the player amount is same as current raise, action is complete for this player
			continue
		}

		allActed = false
		break
	}
	return allActed
}

func (h *HandState) getBlindPos() (uint32, uint32, error) {

	buttonSeat := uint32(h.GetButtonPos())
	smallBlindPos := h.getNextActivePlayer(buttonSeat)
	bigBlindPos := h.getNextActivePlayer(smallBlindPos)

	if smallBlindPos == 0 || bigBlindPos == 0 {
		// TODO: handle not enough players condition
		return 0, 0, fmt.Errorf("Small bind (%d) or big blind (%d) position is 0", smallBlindPos, bigBlindPos)
	}
	return uint32(smallBlindPos), uint32(bigBlindPos), nil
}

// WARNING: Keep this method idempotent (no mutate HandState). It could get retried in case of reshuffle.
func (h *HandState) getPlayersCards(deck *poker.Deck, scriptedCardsBySeat map[uint32]*GameSetupSeatCards) (map[uint32][]poker.Card, int) {
	noOfCards := 2

	switch h.GetGameType() {
	case GameType_HOLDEM:
		noOfCards = 2
	case GameType_PLO:
		noOfCards = 4
	case GameType_PLO_HILO:
		noOfCards = 4
	case GameType_FIVE_CARD_PLO:
		noOfCards = 5
	case GameType_FIVE_CARD_PLO_HILO:
		noOfCards = 5
	case GameType_SIX_CARD_PLO:
		noOfCards = 6
	case GameType_SIX_CARD_PLO_HILO:
		noOfCards = 6
	}

	activeSeats := h.activeSeatsCount()
	totalCards := activeSeats * noOfCards

	var playerCards map[uint32][]poker.Card
	var numCardsUsed int
	if scriptedCardsBySeat == nil {
		// Draw cards normally.
		playerCards, numCardsUsed = h.drawPlayerCards(deck, noOfCards, totalCards)
	} else {
		// We are running a botrunner test that wants to specify player cards by seat.
		// Make sure each seat gets the cards they are supposed to get.
		playerCards, numCardsUsed = h.drawPlayerCardsForTesting(deck, noOfCards, totalCards, scriptedCardsBySeat)
	}

	return playerCards, numCardsUsed
}

// WARNING: Keep this method idempotent (no mutate HandState). It could get retried in case of reshuffle.
func (h *HandState) drawPlayerCards(deck *poker.Deck, numCardsPerPlayer int, numTotalCards int) (map[uint32][]poker.Card, int) {
	numRemainingCards := numTotalCards
	playerCards := make(map[uint32][]poker.Card)
	numCardsUsed := 0
	for i := 0; i < numCardsPerPlayer; i++ {
		seatNo := h.ButtonPos
		for {
			seatNo = h.getNextActivePlayer(seatNo)
			cards := deck.Draw(1)
			numRemainingCards--
			numCardsUsed++
			if playerCards[seatNo] == nil {
				playerCards[seatNo] = make([]poker.Card, 0, numCardsPerPlayer)
			}
			playerCards[seatNo] = append(playerCards[seatNo], cards[0])
			if seatNo == h.ButtonPos || numRemainingCards == 0 {
				// next round of cards
				break
			}
		}
		if numRemainingCards == 0 {
			break
		}
	}
	return playerCards, numCardsUsed
}

// Draw player cards and make sure each seat gets the scripted cards.
func (h *HandState) drawPlayerCardsForTesting(deck *poker.Deck, numCardsPerPlayer int, numTotalCards int, scriptedCards map[uint32]*GameSetupSeatCards) (map[uint32][]poker.Card, int) {
	if scriptedCards == nil {
		panic("scriptedCards == nil in drawPlayerCardsForTesting")
	}

	numCardsUsed := 0
	numRemainingCards := numTotalCards
	playerCards := make(map[uint32][]poker.Card)
	seatNo := h.ButtonPos
	for numRemainingCards > 0 {
		seatNo = h.getNextActivePlayer(seatNo)
		for i := 0; i < numCardsPerPlayer; i++ {
			// In this case we need to make sure each seat gets the specified cards.
			wantedCard := poker.NewCard(scriptedCards[seatNo].Cards[i])
			drawnCard := deck.Draw(1)[0]
			if drawnCard != wantedCard {
				// We want the wantedCard, but drawnCard is what came out of the deck.
				// Which means our wantedCard is still somewhere in the deck.
				// Put back the drawnCard into the deck in the position where our
				// wantedCard is, effectively erasing the wantedCard from the deck as
				// if that was the card drawn out of the deck.
				deck.FindAndReplace(wantedCard, drawnCard)
				drawnCard = wantedCard
			}
			numRemainingCards--
			numCardsUsed++
			if playerCards[seatNo] == nil {
				playerCards[seatNo] = make([]poker.Card, 0, numCardsPerPlayer)
			}
			playerCards[seatNo] = append(playerCards[seatNo], drawnCard)
		}
		if seatNo == h.ButtonPos {
			if numRemainingCards != 0 {
				panic("numRemainingCards != 0 after assigning cards to all active players")
			}
			break
		}
	}
	return playerCards, numCardsUsed
}

/**
This helper method returns the next active player from the specified seat number.
It is a useful function to determine moving button, blinds, next action
**/
func (h *HandState) getNextActivePlayer(seatNo uint32) uint32 {
	nextSeat := uint32(0)
	var j uint32
	for j = 1; j <= h.MaxSeats; j++ {
		seatNo++
		if seatNo > h.MaxSeats {
			seatNo = 1
		}

		player := h.PlayersInSeats[seatNo]
		// check to see whether the player is playing or sitting out
		if !player.Inhand {
			continue
		}

		if h.ActiveSeats[seatNo] == 0 {
			continue
		}

		// skip the all-in player
		if h.PlayersActed[seatNo].Action == ACTION_ALLIN {
			continue
		}

		nextSeat = seatNo
		break
	}

	return nextSeat
}

func (h *HandState) actionReceived(action *HandAction, actionResponseTime uint64) error {
	// get player ID from the seat
	playersInSeats := h.PlayersInSeats
	if len(playersInSeats) < int(action.SeatNo) {
		errMsg := fmt.Sprintf("Unable to find player ID for the action seat %d. PlayerIds: %v", action.SeatNo, playersInSeats)
		handLogger.Error().
			Uint64(logging.GameIDKey, h.GetGameId()).
			Uint32(logging.HandNumKey, h.GetHandNum()).
			Uint32(logging.SeatNumKey, action.SeatNo).
			Msg(errMsg)
		return fmt.Errorf(errMsg)
	}

	if action.Action == ACTION_FOLD || action.Action == ACTION_CHECK {
		if action.Amount > 0 {
			handLogger.Warn().
				Uint64(logging.GameIDKey, h.GetGameId()).
				Uint32(logging.HandNumKey, h.GetHandNum()).
				Uint32(logging.SeatNumKey, action.SeatNo).
				Msgf("Invalid amount %f passed for the fold action. Setting the amount to 0", action.Amount)
		}
		action.Amount = 0
	}

	player := playersInSeats[action.SeatNo]
	if player.PlayerId == 0 {
		errMsg := fmt.Sprintf("Invalid seat %d. PlayerID is 0", action.SeatNo)
		// something wrong
		handLogger.Error().
			Uint64(logging.GameIDKey, h.GetGameId()).
			Uint32(logging.HandNumKey, h.GetHandNum()).
			Uint32(logging.SeatNumKey, action.SeatNo).
			Msg(errMsg)
		return fmt.Errorf(errMsg)
	}

	var log *HandActionLog
	switch h.CurrentState {
	case HandStatus_PREFLOP:
		log = h.PreflopActions
	case HandStatus_FLOP:
		log = h.FlopActions
	case HandStatus_TURN:
		log = h.TurnActions
	case HandStatus_RIVER:
		log = h.RiverActions
	}
	h.LastState = h.CurrentState
	bettingState := h.RoundState[uint32(h.CurrentState)]
	bettingRound := bettingState.Betting
	playerBalance := bettingState.PlayerBalance[action.SeatNo]
	playerBetSoFar := bettingRound.SeatBet[action.SeatNo]
	diff := float64(0)

	// the next player after big blind can straddle
	straddleAvailable := false
	if action.Action == ACTION_BB && !h.ButtonStraddle {
		straddleAvailable = true
	}

	if action.Action == ACTION_BOMB_POT_BET {
		// handle posting blind as special
		amount := action.Amount
		if amount > playerBalance {
			amount = playerBalance
		}
		playerBalance = playerBalance - amount
		h.acted(action.SeatNo, ACTION_BOMB_POT_BET, amount)
		action.Stack = bettingState.PlayerBalance[action.SeatNo]
		action.ActionTime = uint32(actionResponseTime)
		bettingRound.SeatBet[int(action.SeatNo)] = amount
	}

	amount := action.Amount
	if action.Action == ACTION_ALLIN {
		amount = playerBalance + playerBetSoFar
	}

	if amount > h.CurrentRaise {
		if action.Action != ACTION_BB &&
			action.Action != ACTION_SB &&
			action.Action != ACTION_STRADDLE {
			bettingState.BetIndex++
		}
	}

	// if player has less than the blinds, then this player will go all-in
	if action.Action == ACTION_BB ||
		action.Action == ACTION_SB ||
		action.Action == ACTION_POST_BLIND {
		if playerBalance < action.Amount {
			action.Action = ACTION_ALLIN
			action.Amount = playerBalance
		}
	}

	// valid actions
	if action.Action == ACTION_FOLD {
		// track what round player folded the hand
		h.acted(action.SeatNo, ACTION_FOLD, playerBetSoFar)
	} else if action.Action == ACTION_CHECK {
		h.PlayersActed[action.SeatNo].Action = ACTION_CHECK
	} else if action.Action == ACTION_CALL {
		// action call
		if action.Amount < h.CurrentRaise {
			// fold this player
			h.acted(action.SeatNo, ACTION_FOLD, playerBetSoFar)
		} else if action.Amount == playerBalance {
			// the player is all in
			action.Action = ACTION_ALLIN
			h.acted(action.SeatNo, ACTION_ALLIN, action.Amount)
			diff = playerBalance
		} else {
			// if this player has an equity in this pot, just call subtract the amount
			h.acted(action.SeatNo, ACTION_CALL, action.Amount)
		}
		diff = (action.Amount - playerBetSoFar)
		playerBetSoFar += diff
		bettingRound.SeatBet[action.SeatNo] = playerBetSoFar
	} else if action.Action == ACTION_ALLIN {
		amount := playerBalance + playerBetSoFar
		bettingRound.SeatBet[action.SeatNo] = amount
		diff = playerBalance
		action.Amount = amount
		h.acted(action.SeatNo, ACTION_ALLIN, amount)
		if action.Amount > h.CurrentRaise {
			// reset player action
			h.RaiseSeatNo = action.SeatNo
			h.RaiseAmount = action.Amount
		}
	} else if action.Action == ACTION_RAISE ||
		action.Action == ACTION_BET {
		// TODO: we need to handle raise and determine next min raise
		if action.Amount < h.GetCurrentRaise() {
			// invalid
			handLogger.Error().
				Uint64(logging.GameIDKey, h.GetGameId()).
				Uint32(logging.HandNumKey, h.GetHandNum()).
				Uint32(logging.SeatNumKey, action.SeatNo).
				Msgf("Invalid raise %f. Current bet: %f", action.Amount, h.GetCurrentRaise())
		}

		handAction := ACTION_RAISE
		if action.Action == ACTION_BET {
			handAction = ACTION_BET
		}
		if action.Action == ACTION_ALLIN {
			handAction = ACTION_ALLIN
			// player is all in
			action.Amount = playerBetSoFar + bettingState.PlayerBalance[action.SeatNo]
		}

		if action.Amount > h.CurrentRaise {
			// reset player action
			h.acted(action.SeatNo, handAction, action.Amount)
			h.ActionCompleteAtSeat = action.SeatNo
			h.RaiseSeatNo = action.SeatNo
			h.RaiseAmount = action.Amount
		}

		// how much this user already had in the betting round
		diff = action.Amount - playerBetSoFar

		if diff == bettingState.PlayerBalance[action.SeatNo] {
			// player is all in
			action.Action = ACTION_ALLIN
			h.acted(action.SeatNo, ACTION_ALLIN, action.Amount)
		}
		bettingRound.SeatBet[action.SeatNo] = action.Amount
	} else if action.Action == ACTION_SB ||
		action.Action == ACTION_BB ||
		action.Action == ACTION_STRADDLE ||
		action.Action == ACTION_POST_BLIND {
		bettingRound.SeatBet[action.SeatNo] = action.Amount
		switch action.Action {
		case ACTION_SB:
			h.acted(action.SeatNo, ACTION_SB, action.Amount)
		case ACTION_BB:
			h.acted(action.SeatNo, ACTION_BB, action.Amount)
		case ACTION_STRADDLE:
			h.acted(action.SeatNo, ACTION_STRADDLE, action.Amount)
		case ACTION_POST_BLIND:
			h.acted(action.SeatNo, ACTION_POST_BLIND, action.Amount)
		}
		diff = action.Amount
	}

	if action.Action != ACTION_BOMB_POT_BET {
		if action.Amount > h.CurrentRaise {
			h.BetBeforeRaise = h.CurrentRaise
			h.CurrentRaiseDiff = action.Amount - h.CurrentRaise
			if h.CurrentState == HandStatus_PREFLOP && h.CurrentRaiseDiff < h.BigBlind {
				h.CurrentRaiseDiff = action.Amount
				h.BetBeforeRaise = 0
			}
			h.CurrentRaise = action.Amount
			h.ActionCompleteAtSeat = action.SeatNo
		}
	}
	playerBalance = playerBalance - diff
	bettingState.PlayerBalance[action.SeatNo] = playerBalance
	action.Stack = bettingState.PlayerBalance[action.SeatNo]
	action.ActionTime = uint32(actionResponseTime)
	// add the action to the log
	log.Actions = append(log.Actions, action)

	// check whether everyone has acted in this ROUND
	// or everyone except folded in this hand
	if h.hasEveryOneActed() || h.NoActiveSeats == 1 {
		// settle this round and move to next round
		h.settleRound()
		// next seat action will be determined outside of here
		// after moving to next round
		h.NextSeatAction = nil
	} else {
		actionSeat := h.getNextActivePlayer(action.SeatNo)
		if action.Action != ACTION_SB {
			h.NextSeatAction = h.prepareNextAction(actionSeat, straddleAvailable)
		}
	}
	return nil
}

func (h *HandState) getPlayerFromSeat(seatNo uint32) *PlayerInSeatState {
	player := h.PlayersInSeats[seatNo]
	if !player.Inhand {
		return nil
	}
	return player
}

func (h *HandState) isAllActivePlayersAllIn() bool {
	allIn := true
	noAllInPlayers := 0
	noActivePlayers := 0
	for seatNo, playerID := range h.ActiveSeats {
		if playerID == 0 {
			continue
		}
		if h.PlayersActed[seatNo].Action != ACTION_FOLD {
			noActivePlayers++
		}

		if h.AllInPlayers[seatNo] == 0 {
			allIn = false
		} else {
			noAllInPlayers++
		}
	}
	return allIn
}

func (h *HandState) allActionComplete() bool {
	noAllInPlayers := 0
	noActivePlayers := 0
	for seatNo, playerID := range h.ActiveSeats {
		if playerID == 0 {
			continue
		}
		if h.PlayersActed[seatNo].Action != ACTION_FOLD {
			noActivePlayers++
		}

		if h.AllInPlayers[seatNo] != 0 {
			noAllInPlayers++
		}
	}
	if h.hasEveryOneActed() {
		if noActivePlayers-noAllInPlayers <= 1 {
			return true
		}
	}

	return false
}

func (h *HandState) getMaxBet() float64 {
	// before we go to next stage, settle pots
	bettingState := h.RoundState[uint32(h.CurrentState)]
	currentBettingRound := bettingState.Betting
	maxBet := float64(0)
	for seatNo, bet := range currentBettingRound.SeatBet {
		if currentBettingRound.SeatBet[seatNo] == 0.0 {
			// empty seat
			continue
		}
		if maxBet < bet {
			maxBet = bet
		}
	}
	return maxBet
}

func (h *HandState) settleRound() {
	// before we go to next stage, settle pots
	bettingState := h.RoundState[uint32(h.CurrentState)]
	currentBettingRound := bettingState.Betting

	// if only one player is active, then this hand is concluded
	handEnded := false
	if h.NoActiveSeats == 1 {
		handEnded = true
	} else {
	}

	for _, playerActRound := range h.PlayersActed {
		playerActRound.BetAmount = 0.0
	}

	h.addChipsToPot(currentBettingRound.SeatBet, handEnded)

	// update the stack based on the amount the player bet on this round
	for seatNo, playerActRound := range h.PlayersActed {
		if seatNo == 0 {
			continue
		}

		player := h.PlayersInSeats[seatNo]
		if !player.Inhand {
			continue
		}
		player.Stack -= playerActRound.BetAmount
	}
	if handEnded {
		h.HandCompletedAt = h.CurrentState
		h.CurrentState = HandStatus_RESULT
	}

	// if this player is last to act, then move to the next round
	if h.CurrentState == HandStatus_PREFLOP {
		h.CurrentState = HandStatus_FLOP
	} else if h.CurrentState == HandStatus_FLOP {
		h.CurrentState = HandStatus_TURN
	} else if h.CurrentState == HandStatus_TURN {
		h.CurrentState = HandStatus_RIVER
	} else if h.CurrentState == HandStatus_RIVER {
		h.CurrentState = HandStatus_SHOW_DOWN
	}
}

func (h *HandState) removeFoldedPlayersFromPots() {
	for _, pot := range h.Pots {
		updateSeats := make([]uint32, 0)
		for _, seat := range pot.Seats {
			if h.PlayersActed[seat].Action == ACTION_FOLD {
				// skip this player
			} else {
				updateSeats = append(updateSeats, seat)
			}
		}
		pot.Seats = updateSeats
	}
}

func (h *HandState) removeEmptyPots() {
	pots := make([]*SeatsInPots, 0)
	for _, pot := range h.Pots {
		if len(pot.Seats) == 0 || pot.Pot == 0 {
			continue
		}
		pots = append(pots, pot)
	}
	h.Pots = pots
}

func (h *HandState) setupNextRound(state HandStatus) error {
	h.CurrentState = state
	h.resetPlayerActions()
	h.CurrentRaise = 0
	h.CurrentRaiseDiff = 0
	h.BetBeforeRaise = 0
	h.setupRound(state)
	actionSeat := h.getNextActivePlayer(h.ButtonPos)
	if actionSeat == 0 {
		// every one is all in
		return nil
	}
	playerState := h.getPlayerFromSeat(actionSeat)
	if playerState == nil {
		return fmt.Errorf("Player state for seat %d is nil", actionSeat)
	}
	h.NextSeatAction = h.prepareNextAction(actionSeat, false)

	// track whether the player is active in this round or not
	for seatNo := 1; seatNo <= int(h.GetMaxSeats()); seatNo++ {
		player := h.PlayersInSeats[seatNo]
		if !player.Inhand {
			continue
		}
		player.Round = state
	}

	return nil
}

func (h *HandState) setupFlop() error {
	err := h.setupNextRound(HandStatus_FLOP)
	if err != nil {
		return errors.Wrap(err, "Error while setting up next round (flop)")
	}
	return nil
}

func (h *HandState) setupTurn() error {
	err := h.setupNextRound(HandStatus_TURN)
	if err != nil {
		return errors.Wrap(err, "Error while setting up next round (turn)")
	}
	return nil
}

func (h *HandState) setupRiver() error {
	err := h.setupNextRound(HandStatus_RIVER)
	if err != nil {
		return errors.Wrap(err, "Error while setting up next round (river)")
	}
	return nil
}

func (h *HandState) adjustToBringIn(amount float64) float64 {
	if h.BringIn != 0 {
		// make call amount multiples of bring-in
		if int64(amount)%int64(h.BringIn) > 0 {
			amount = float64(int64(amount/h.BringIn+1.0)) * h.BringIn
			amount = math.Floor(amount)
		}
	}
	return amount
}

func (h *HandState) calcPloPotBet(actionSeatNo uint32, callAmount float64, preFlop bool) float64 {
	roundState := h.RoundState[uint32(h.CurrentState)]
	bettingRound := roundState.Betting
	firstAction := false
	if preFlop {
		firstAction = true
		for _, bet := range bettingRound.SeatBet {
			if bet > h.BigBlind {
				firstAction = false
				break
			}
		}
	}

	totalPot := callAmount + callAmount
	for _, pot := range h.Pots {
		totalPot += pot.Pot
	}

	for seatNo, bet := range bettingRound.SeatBet {
		if uint32(seatNo) == actionSeatNo {
			// don't include my bet in the pot calculation
			continue
		}
		// if there is no call or bet, then consider the blinds as bring-in bets
		if firstAction && bet != 0 && bet < h.BringIn {
			bet = h.BringIn
		}
		totalPot += bet
	}

	if h.BringIn != 0 {
		// make call amount multiples of bring-in
		totalPot = h.adjustToBringIn(totalPot)
	}
	return totalPot
}

func (h *HandState) prepareNextAction(actionSeat uint32, straddleAvailable bool) *NextSeatAction {
	// compute next action
	bettingState := h.RoundState[uint32(h.CurrentState)]
	bettingRound := bettingState.Betting

	playerBalance := bettingState.PlayerBalance[actionSeat]
	nextAction := &NextSeatAction{SeatNo: actionSeat}
	availableActions := make([]ACTION, 0)
	availableActions = append(availableActions, ACTION_FOLD)
	if h.CurrentState == HandStatus_PREFLOP {
		if straddleAvailable {
			// the player can straddle, if he has enough money
			straddleAmount := 2.0 * h.BigBlind
			if playerBalance >= straddleAmount {
				availableActions = append(availableActions, ACTION_STRADDLE)
				nextAction.StraddleAmount = h.BigBlind * 2.0
			}
		}
	}

	// the current raise amount in the betting round
	// this is used for calculating 2x, 3, 5x
	nextAction.RaiseAmount = h.CurrentRaiseDiff

	nextAction.SeatInSoFar = bettingRound.SeatBet[actionSeat]
	betOptions := make([]*BetRaiseOption, 0)
	allInAvailable := false
	canRaise := false
	canBet := false
	canCall := false
	canCheck := false
	// then the caller call, raise, or go all in
	if playerBalance <= h.CurrentRaise || h.GameType == GameType_HOLDEM {
		allInAvailable = true
	}

	if h.CurrentRaise == 0.0 {
		// then the caller can check
		canCheck = true
		// the player can bet
		canBet = true
	} else {
		actedState := h.PlayersActed[actionSeat]
		if (actedState.Action == ACTION_BB ||
			actedState.Action == ACTION_STRADDLE) &&
			h.GetCurrentRaise() <= actedState.Amount {
			// we can check
			canCheck = true
		}

		if playerBalance+actedState.Amount >= h.CurrentRaise {
			if (actedState.Action == ACTION_BB && h.CurrentRaise > h.BigBlind) ||
				(actedState.Action == ACTION_STRADDLE && h.CurrentRaise > h.Straddle) {
				availableActions = append(availableActions, ACTION_CALL)
			} else if actedState.Amount == h.CurrentRaise {
				canCheck = true
			} else {
				canCall = true
			}
			nextAction.CallAmount = h.CurrentRaiseDiff + h.BetBeforeRaise

			canRaise = true
			// if the call amount is less than bring in amount, use bring in amount
			if h.CurrentState == HandStatus_PREFLOP {
				if nextAction.CallAmount <= h.BringIn {
					nextAction.CallAmount = h.BringIn
					canBet = true
					canRaise = false
				}
				if h.BetBeforeRaise == 0 {
					canBet = true
					canRaise = false
				}
			}
		}
	}

	playerPrevAction := h.PlayersActed[actionSeat]
	if playerPrevAction.Action == ACTION_CALL ||
		playerPrevAction.Action == ACTION_BET ||
		playerPrevAction.Action == ACTION_RAISE {
		if playerPrevAction.RaiseAmount > h.CurrentRaiseDiff {
			// this player cannot raise
			canCall = true
			canRaise = false
			allInAvailable = false
		}
	}

	allIn := bettingRound.SeatBet[actionSeat] + playerBalance
	if canBet || canRaise {
		player := h.PlayersInSeats[actionSeat]
		if h.CurrentRaiseDiff > 0 {
			nextAction.MinRaiseAmount = (h.CurrentRaiseDiff * 2) + h.BetBeforeRaise
		}
		// at preflop, the min raise should be twice than the bringin amount
		nextAction.MinRaiseAmount = (h.CurrentRaiseDiff * 2) + h.BetBeforeRaise
		if nextAction.MinRaiseAmount == 0 {
			nextAction.MinRaiseAmount = h.BigBlind
		}
		if h.BringIn > 0.0 {
			if h.CurrentState == HandStatus_PREFLOP && nextAction.MinRaiseAmount < 2.0*h.BringIn {
				nextAction.MinRaiseAmount = 2.0 * h.BringIn
			}
		}
		nextAction.MinRaiseAmount = h.adjustToBringIn(nextAction.MinRaiseAmount)

		if h.GameType == GameType_HOLDEM {
			nextAction.MaxRaiseAmount = bettingRound.SeatBet[actionSeat] + playerBalance
		} else {
			preFlop := h.CurrentState == HandStatus_PREFLOP
			// handle PLO max raise
			ploPot := h.calcPloPotBet(actionSeat, nextAction.CallAmount, preFlop)
			nextAction.MaxRaiseAmount = ploPot
			if ploPot > allIn {
				nextAction.MaxRaiseAmount = allIn
				allInAvailable = true
			}
		}

		if nextAction.MaxRaiseAmount >= allIn {
			nextAction.MaxRaiseAmount = allIn
		}

		if canBet {
			// calculate what the maximum amount the player can bet
			//availableActions = append(availableActions, ACTION_BET)
			betOptions = h.betOptions(actionSeat,
				h.CurrentState,
				player.PlayerId,
				nextAction.CallAmount)
		}

		if canRaise {
			// calculate what the maximum amount the player can bet
			if h.CurrentState == HandStatus_PREFLOP && h.CurrentRaise == h.BigBlind {
				betOptions = h.betOptions(actionSeat,
					h.CurrentState,
					player.PlayerId,
					nextAction.CallAmount)
				canBet = true
				canRaise = false
			} else {
				if allIn > nextAction.MinRaiseAmount {
					// calculate the maximum amount the player can raise
					canRaise = true
					betOptions = h.raiseOptions(actionSeat,
						nextAction.CallAmount,
						h.CurrentRaiseDiff,
						h.BetBeforeRaise,
						nextAction.MinRaiseAmount,
						nextAction.MaxRaiseAmount,
						player.PlayerId)
				} else {
					canRaise = false
					// this player can go only all-in to raise
					nextAction.MinRaiseAmount = 0
					nextAction.MaxRaiseAmount = 0
					allInAvailable = true
				}
			}
		}

		if h.GameType == GameType_HOLDEM || playerBalance < nextAction.MinRaiseAmount {
			// the player can go all in no limit holdem
			allInAvailable = true
		}
	}

	if canBet {
		// if player all in amount is less than min raise amount, then the player cannot bet or raise
		if nextAction.MinRaiseAmount >= allIn {
			allInAvailable = true
			nextAction.MinRaiseAmount = 0
			nextAction.MaxRaiseAmount = 0
		} else {
			availableActions = append(availableActions, ACTION_BET)
			nextAction.BetOptions = betOptions
		}
	}

	if canRaise {

		// this player can raise
		// let us see how many active players on this hand other this player
		if h.activeSeatsCount()-1 == h.allinCount() {
			// all the remaining players all-in
			// this player can just call the last player's all-in bet
			canRaise = false
			allInAvailable = false
		}
		if canRaise {
			if nextAction.MinRaiseAmount >= allIn {
				allInAvailable = true
				nextAction.MinRaiseAmount = 0
				nextAction.MaxRaiseAmount = 0
			} else {
				availableActions = append(availableActions, ACTION_RAISE)
				nextAction.BetOptions = betOptions
			}
		}
	}

	if canCheck {
		actionFound := false
		for _, action := range availableActions {
			if action == ACTION_CHECK {
				actionFound = true
			}
		}
		if !actionFound {
			availableActions = append(availableActions, ACTION_CHECK)
		}
	}

	if canCall {
		actionFound := false
		for _, action := range availableActions {
			if action == ACTION_CALL {
				actionFound = true
			}
		}

		if !actionFound {
			if h.CurrentRaiseDiff > playerBalance {
				allInAvailable = true
			} else {
				actedState := h.PlayersActed[actionSeat]
				if playerBalance+actedState.Amount == h.CurrentRaise {
					allInAvailable = true
				} else {
					availableActions = append(availableActions, ACTION_CALL)
					nextAction.CallAmount = h.CurrentRaiseDiff + h.BetBeforeRaise
				}
			}
		}
	}

	if allInAvailable {
		availableActions = append(availableActions, ACTION_ALLIN)
		nextAction.AllInAmount = allIn
	}

	// if all in amount is equal to call amount, then don't use CALL action
	if nextAction.AllInAmount == nextAction.CallAmount {
		actions := make([]ACTION, 0)
		for _, action := range availableActions {
			if action != ACTION_CALL {
				actions = append(actions, action)
			}
		}
		availableActions = actions
		nextAction.CallAmount = 0
	}

	if nextAction.MaxRaiseAmount == allIn {
		nextAction.AllInAmount = allIn
	}
	nextAction.AvailableActions = availableActions

	totalPot := float64(0)
	for _, pot := range h.Pots {
		totalPot += pot.Pot
	}
	nextAction.PotAmount = totalPot

	// Generate the action ID deterministically instead of a randomly in case the server crashes
	// between sending YOUR_ACTION and persisting the hand state. When the server comes back from
	// the crash, it will generate and store the same action ID in the hand state this way
	// and successfully validate the client action generated from the action ID before the crash.
	nextAction.ActionId = hashing.GenerateStringHash(fmt.Sprintf("%d|%s|%d|%d", h.HandNum, h.CurrentState, actionSeat, h.CurrentActionNum+1))

	return nextAction
}

func (h *HandState) everyOneFoldedWinners() {
	// let us settle the round
	// bettingState := h.RoundState[uint32(h.CurrentState)]
	// currentBettingRound := bettingState.Betting
	// h.addChipsToPot(currentBettingRound.SeatBet, true)

	seatNo := 0
	for i, playerID := range h.ActiveSeats {
		if playerID != 0 {
			seatNo = i
			break
		}
	}
	potWinners := make(map[uint32]*PotWinners)
	for i, pot := range h.Pots {
		handWinner := &HandWinner{
			SeatNo: uint32(seatNo),
			Amount: pot.Pot,
		}
		handWinners := make([]*HandWinner, 0)
		handWinners = append(handWinners, handWinner)
		potWinners[uint32(i)] = &PotWinners{HiWinners: handWinners}
	}
}

func (h *HandState) getLog() *HandLog {
	handResult := &HandLog{}
	handResult.PotWinners = h.PotWinners
	handResult.WonAt = h.HandCompletedAt
	handResult.PreflopActions = h.PreflopActions
	handResult.FlopActions = h.FlopActions
	handResult.TurnActions = h.TurnActions
	handResult.RiverActions = h.RiverActions
	handResult.HandStartedAt = h.HandStartedAt
	handResult.HandEndedAt = h.HandEndedAt
	handResult.HandEndedAt = uint64(time.Now().Unix())
	if h.HeadsupPlayers != nil {
		handResult.HeadsupPlayers = make([]uint64, 0)
		handResult.HeadsupPlayers = append(handResult.HeadsupPlayers, h.HeadsupPlayers...)
	}

	if h.RunItTwiceConfirmed {
		handResult.RunItTwice = true
		handResult.RunItTwiceResult = &RunItTwiceResult{
			RunItTwiceStartedAt: h.RunItTwice.Stage,
			Board_1Winners:      h.PotWinners,
			Board_2Winners:      h.Board2Winners,
		}
	}
	return handResult
}

func (h *HandState) betOptions(seatNo uint32, round HandStatus, playerID uint64, callAmount float64) []*BetRaiseOption {
	preFlopBetOptions := preFlopBets
	postFlopBets := postFlopBets
	roundState := h.RoundState[uint32(h.CurrentState)]
	bettingRound := roundState.Betting
	balance := roundState.PlayerBalance[seatNo]
	allIn := bettingRound.SeatBet[seatNo] + balance

	options := make([]*BetRaiseOption, 0)
	if h.GameType == GameType_HOLDEM {
		if round == HandStatus_PREFLOP {
			// pre-flop options
			for _, betOption := range preFlopBetOptions {
				betAmount := float64(int64(float64(betOption) * h.BigBlind))
				if betAmount < balance {
					option := &BetRaiseOption{
						Text:   fmt.Sprintf("%dBB", betOption),
						Amount: betAmount,
					}
					options = append(options, option)
				}
			}
			options = append(options, &BetRaiseOption{Text: "All-In", Amount: allIn})
		} else if round >= HandStatus_FLOP {
			totalPot := float64(0.0)
			for _, pot := range h.Pots {
				totalPot += pot.Pot
			}
			// post-flop options
			for _, betOption := range postFlopBets {
				betAmount := float64(int64(float64(betOption)*totalPot) / 100.0)
				if h.ChipUnit == ChipUnit_DOLLAR {
					betAmount = util.FloorToNearest(betAmount, 100)
				}
				if betAmount > h.BigBlind && betAmount < balance {
					option := &BetRaiseOption{
						Text:   fmt.Sprintf("%d%%", betOption),
						Amount: float64(betAmount),
					}
					options = append(options, option)
				}
			}
			options = append(options, &BetRaiseOption{Text: "All-In", Amount: allIn})
		}
	} else {
		// PLO
		if round == HandStatus_PREFLOP {
			preFlop := h.CurrentState == HandStatus_PREFLOP
			ploPot := h.calcPloPotBet(seatNo, callAmount, preFlop)
			// pre-flop options
			for _, betOption := range ploPreFlopBets {
				bet := h.BigBlind
				if bet < h.BringIn {
					bet = h.BringIn
				}

				betAmount := float64(int64(float64(betOption) * bet))
				if betAmount > ploPot {
					//betAmount = ploPot
					continue
				}
				betAmount = h.adjustToBringIn(betAmount)

				if betAmount < balance {
					option := &BetRaiseOption{
						Text:   fmt.Sprintf("%dBB", betOption),
						Amount: betAmount,
					}
					options = append(options, option)
				}
			}
			if ploPot > allIn {
				options = append(options, &BetRaiseOption{Text: "All-In", Amount: allIn})
			} else {
				options = append(options, &BetRaiseOption{Text: "Pot", Amount: ploPot})
			}
		} else {
			preFlop := h.CurrentState == HandStatus_PREFLOP
			// post flop
			ploPot := h.calcPloPotBet(seatNo, callAmount, preFlop)
			// pre-flop options
			for _, betOption := range postFlopBets {
				// skip 100%
				if betOption == 100 {
					continue
				}
				bet := h.BigBlind
				if bet < h.BringIn {
					bet = h.BringIn
				}

				betAmount := float64(int64(float64(betOption)*ploPot) / 100.0)
				if betAmount > ploPot {
					betAmount = ploPot
				}

				if h.BringIn != 0.0 && betAmount < h.BringIn {
					// skip this option
					continue
				}
				betAmount = h.adjustToBringIn(betAmount)

				if betAmount < balance {
					option := &BetRaiseOption{
						Text:   fmt.Sprintf("%d%%", betOption),
						Amount: betAmount,
					}
					options = append(options, option)
				}
			}
			if ploPot > allIn {
				options = append(options, &BetRaiseOption{Text: "All-In", Amount: allIn})
			} else {
				options = append(options, &BetRaiseOption{Text: "Pot", Amount: ploPot})
			}
		}
	}
	return options
}

func (h *HandState) raiseOptions(seatNo uint32, callAmount float64, raiseAmount float64, seatInSoFar float64, minRaiseAmount float64, maxRaiseAmount float64, playerID uint64) []*BetRaiseOption {
	roundState := h.RoundState[uint32(h.CurrentState)]
	balance := roundState.PlayerBalance[seatNo]
	bettingRound := roundState.Betting
	allIn := bettingRound.SeatBet[seatNo] + balance

	options := make([]*BetRaiseOption, 0)
	if h.GameType == GameType_HOLDEM {
		for _, betOption := range raiseOptions {
			betAmount := float64(float64(betOption)*raiseAmount) + seatInSoFar
			if betAmount < balance {
				option := &BetRaiseOption{
					Text:   fmt.Sprintf("%dx", betOption),
					Amount: betAmount,
				}
				options = append(options, option)
			}
		}
		options = append(options, &BetRaiseOption{Text: "All-In", Amount: allIn})
	} else {
		for _, betOption := range raiseOptions {
			betAmount := float64(int64(float64(betOption)*raiseAmount)) + callAmount
			betAmount = h.adjustToBringIn(betAmount)
			if betAmount < maxRaiseAmount {
				option := &BetRaiseOption{
					Text:   fmt.Sprintf("%dx", betOption),
					Amount: betAmount,
				}
				options = append(options, option)
			}
		}
		if allIn <= maxRaiseAmount {
			options = append(options, &BetRaiseOption{Text: "All-In", Amount: allIn})
		} else {
			options = append(options, &BetRaiseOption{Text: "Pot", Amount: maxRaiseAmount})
		}
	}
	return options
}

func (h *HandState) activeSeatsCount() int {
	count := 0
	for _, playerID := range h.ActiveSeats {
		if playerID != 0 {
			count++
		}
	}
	return count
}

func (h *HandState) allinCount() int {
	count := 0
	for _, playerID := range h.AllInPlayers {
		if playerID != 0 {
			count++
		}
	}
	return count
}

func (h *HandState) playerLeftGame(playerID uint64) {
	for _, player := range h.PlayersInSeats {
		if player.PlayerId == playerID {
			player.Status = PlayerStatus_LEFT
			break
		}
	}
}

func (h *HandState) hasPlayerLeftGame(playerID uint64) bool {
	for _, player := range h.PlayersInSeats {
		if player.PlayerId == playerID {
			return player.Status == PlayerStatus_LEFT
		}
	}
	return false
}

func (h *HandState) getPlayerFromSeatNo(seatNo uint32) *PlayerInSeatState {
	for _, player := range h.PlayersInSeats {
		if player.SeatNo == seatNo {
			return player
		}
	}
	return nil
}
