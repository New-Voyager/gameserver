package game

import (
	"fmt"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/rs/zerolog/log"
	"voyager.com/server/poker"
)

var handLogger = log.With().Str("logger_name", "game::hand").Logger()

func LoadHandState(handID uint64) *HandState {
	handStateBytes, ok := runningHands[handID]
	if !ok {
		panic(fmt.Sprintf("Hand id: %d is not found", handID))
	}
	handState := &HandState{}
	err := proto.Unmarshal(handStateBytes, handState)
	if err != nil {
		panic(fmt.Sprintf("Hand id: %d could not be unmarshalled", handID))
	}
	return handState
}

func (h *HandState) PrettyPrint() string {
	var b strings.Builder
	b.Grow(32)
	fmt.Fprintf(&b, "Game Num: %d Hand Num: %d, Seats: [", h.GameNum, h.HandNum)
	for seatNo, playerID := range h.GetPlayersInSeats() {
		seatNo++
		if playerID == 0 {
			// empty seat
			fmt.Fprintf(&b, " {%d: Empty}, ", seatNo)
		} else {
			player, _ := players[playerID]
			playerState, _ := h.PlayersState[playerID]
			playerCards, _ := h.PlayersCards[playerID]
			cardString := poker.CardsToString(playerCards)
			if uint32(seatNo) == h.ButtonPos {
				fmt.Fprintf(&b, " {%d: %s, %f, %s, BUTTON} ", seatNo, player, playerState.Balance, cardString)
			} else if uint32(seatNo) == h.SmallBlindPos {
				fmt.Fprintf(&b, " {%d: %s, %f, %s, SB} ", seatNo, player, playerState.Balance, cardString)
			} else if uint32(seatNo) == h.BigBlindPos {
				fmt.Fprintf(&b, " {%d: %s, %f, %s, BB} ", seatNo, player, playerState.Balance, cardString)
			} else {
				fmt.Fprintf(&b, " {%d: %s, %f, %s} ", seatNo, player, playerState.Balance, cardString)
			}
		}
	}
	fmt.Fprintf(&b, "]")

	return b.String()
}

func (h *HandState) initialize(g *Game) {
	// settle players in the seats
	h.PlayersInSeats = g.state.GetPlayersInSeats()

	// determine button and blinds
	h.ButtonPos = h.moveButton(g)
	h.SmallBlindPos, h.BigBlindPos = h.getBlindPos(g)

	// copy player's stack (we need to copy only the players that are in the hand)
	h.PlayersState = h.getPlayersState(g)

	deck := poker.NewDeck().Shuffle()
	h.Deck = deck.GetBytes()
	h.PlayersCards = h.getPlayersCards(g, deck)

	// setup hand for preflop
	h.setupPreflob(g)
}

func (h *HandState) setupPreflob(g *Game) {
	h.CurrentState = HandState_PREFLOP

	// set next action information
	h.CurrentRaise = g.state.BigBlind
	h.ActionCompleteAtSeat = h.BigBlindPos
	h.PreflopActions = &SeatActionLog{Actions: make([]*SeatAction, 0)}
	h.FlopActions = &SeatActionLog{Actions: make([]*SeatAction, 0)}
	h.TurnActions = &SeatActionLog{Actions: make([]*SeatAction, 0)}
	h.RiverActions = &SeatActionLog{Actions: make([]*SeatAction, 0)}
	h.NextActionSeat = h.SmallBlindPos
	h.nextAction(g, &SeatAction{
		SeatNo: h.SmallBlindPos,
		Action: ACTION_SB,
		Amount: g.state.SmallBlind,
	})
	h.nextAction(g, &SeatAction{
		SeatNo: h.BigBlindPos,
		Action: ACTION_BB,
		Amount: g.state.SmallBlind,
	})
}

func (h *HandState) getPlayersState(game *Game) map[uint32]*HandPlayerState {
	handPlayerState := make(map[uint32]*HandPlayerState, 0)
	for j := 1; j <= int(game.state.GetMaxSeats()); j++ {
		playerID := h.GetPlayersInSeats()[j-1]
		if playerID == 0 {
			continue
		}
		playerState, _ := game.state.GetPlayersState()[playerID]
		if playerState.GetStatus() != PlayerState_PLAYING {
			continue
		}
		handPlayerState[playerID] = &HandPlayerState{
			Status:  HandPlayerState_ACTIVE,
			Balance: playerState.CurrentBalance,
		}
	}
	return handPlayerState
}

func (h *HandState) getBlindPos(game *Game) (uint32, uint32) {

	buttonSeat := uint32(h.GetButtonPos())
	smallBlindPos := h.getNextActivePlayer(game, buttonSeat)
	bigBlindPos := h.getNextActivePlayer(game, smallBlindPos)

	if smallBlindPos == 0 || bigBlindPos == 0 {
		// TODO: handle not enough players condition
		panic("Not enough players")
	}
	return uint32(smallBlindPos), uint32(bigBlindPos)
}

func (h *HandState) getPlayersCards(g *Game, deck *poker.Deck) map[uint32][]byte {
	//playerState := g.state.GetPlayersState()
	noOfCards := 2
	switch g.state.GetGameType() {
	case GameState_HOLDEM:
		noOfCards = 2
	case GameState_PLO:
		noOfCards = 4
	case GameState_PLO_HILO:
		noOfCards = 4
	}

	playerCards := make(map[uint32][]byte)
	for _, playerID := range g.state.GetPlayersInSeats() {
		playerCards[playerID] = make([]byte, 0, 4)
	}

	for i := 0; i < noOfCards; i++ {
		seatNo := g.state.ButtonPos
		for {
			seatNo = h.getNextActivePlayer(g, seatNo)
			playerID := g.state.GetPlayersInSeats()[seatNo-1]
			card := deck.Draw(1)
			playerCards[playerID] = append(playerCards[playerID], card[0].GetByte())
			if seatNo == g.state.ButtonPos {
				// next round of cards
				break
			}
		}
		for j := uint32(1); j <= g.state.MaxSeats; j++ {

		}
	}

	return playerCards
}

func (h *HandState) moveButton(g *Game) uint32 {
	seatNo := uint32(g.state.GetButtonPos())
	newButtonPos := h.getNextActivePlayer(g, seatNo)
	if newButtonPos == 0 {
		// TODO: return proper error code
		panic("Not enough players")
	}
	return newButtonPos
}

/**
This helper method returns the next active player from the specified seat number.
It is a useful function to determine moving button, blinds, next action
**/
func (h *HandState) getNextActivePlayer(g *Game, seatNo uint32) uint32 {
	nextSeat := uint32(0)
	var j uint32
	for j = 1; j <= g.state.MaxSeats; j++ {
		seatNo++
		if seatNo > g.state.MaxSeats {
			seatNo = 1
		}

		playerID := g.state.GetPlayersInSeats()[seatNo-1]
		// check to see whether the player is playing or sitting out
		if playerID == 0 {
			continue
		}

		if state, ok := h.GetPlayersState()[playerID]; ok {
			if state.Status != HandPlayerState_ACTIVE {
				continue
			}
		}

		nextSeat = seatNo
		break
	}

	return nextSeat
}

func (h *HandState) nextAction(g *Game, action *SeatAction) {
	if action.SeatNo != h.NextActionSeat {
		handLogger.Error().
			Uint64("game", g.state.GetGameNum()).
			Uint32("hand", g.state.GetHandNum()).
			Msg(fmt.Sprintf("Invalid seat %d made action. Ignored. The next valid action seat is: %d", action.SeatNo, h.NextActionSeat))
	}

	// get player ID from the seat
	playerID := g.state.GetPlayersInSeats()[action.SeatNo-1]
	if playerID == 0 {
		// something wrong
		handLogger.Error().
			Uint64("game", g.state.GetGameNum()).
			Uint32("hand", g.state.GetHandNum()).
			Uint32("seat", action.SeatNo).
			Msg(fmt.Sprintf("Invalid seat %d. PlayerID is 0", action.SeatNo))
	}

	playerState := h.GetPlayersState()[playerID]

	var log *SeatActionLog
	switch h.CurrentState {
	case HandState_PREFLOP:
		log = h.PreflopActions
	case HandState_FLOP:
		log = h.FlopActions
	case HandState_TURN:
		log = h.TurnActions
	case HandState_RIVER:
		log = h.RiverActions
	}

	// valid actions
	if action.Action == ACTION_FOLD {
		playerState.Status = HandPlayerState_FOLDED
		log.Actions = append(log.Actions, action)
	} else if action.Action == ACTION_CHECK {
		log.Actions = append(log.Actions, action)
	} else {
		// action call
		if action.Action == ACTION_CALL {
			if action.Amount < h.GetCurrentRaise() {
				// all in?
				if playerState.GetBalance() < h.GetCurrentRaise() {
					action.Action = ACTION_ALL_IN
				}
			}
		}

		// TODO: we need to handle raise and determine next min raise
		if action.Action == ACTION_RAISE {
			if action.Amount < h.GetCurrentRaise() {
				// invalid
				handLogger.Error().
					Uint64("game", g.state.GetGameNum()).
					Uint32("hand", g.state.GetHandNum()).
					Uint32("seat", action.SeatNo).
					Msg(fmt.Sprintf("Invalid raise %f. Current bet: %f", action.Amount, h.GetCurrentRaise()))
			}
		}
		// raisedAmount := action.Amount - h.CurrentRaise
		if action.Amount > h.CurrentRaise {
			h.ActionCompleteAtSeat = action.SeatNo
		}
		h.CurrentRaise = action.Amount
		log.Actions = append(log.Actions, action)
		log.Pot = log.Pot + action.Amount
		h.NextActionSeat = h.getNextActivePlayer(g, action.SeatNo)
	}

}
