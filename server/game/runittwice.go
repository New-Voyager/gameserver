package game

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	"voyager.com/logging"
	"voyager.com/server/poker"
)

func (g *Game) runItTwice(h *HandState, lastPlayerAction *PlayerActRound) bool {

	if !(h.CurrentState == HandStatus_PREFLOP ||
		h.CurrentState == HandStatus_FLOP ||
		h.CurrentState == HandStatus_TURN ||
		h.CurrentState == HandStatus_RIVER) {
		return false
	}

	if !h.hasEveryOneActed() {
		return false
	}

	// if last player folded his cards, then we won't trigger run it twice
	if lastPlayerAction.Action == ACTION_FOLD {
		return false
	}

	// we run it twice only for headsup and one of the players went all in
	allInPlayers := h.allinCount()

	if allInPlayers != 0 && allInPlayers <= 2 && h.activeSeatsCount() == 2 {
		// if both players opted for run-it-twice, then we will prompt
		prompt := true
		for seatNo, playerID := range h.ActiveSeats {
			if playerID == 0 {
				continue
			}
			playerSeatState := h.PlayersInSeats[seatNo]
			if !playerSeatState.RunItTwice {
				prompt = false
				break
			}
		}

		return prompt
	}
	return false
}

func (g *Game) runItTwicePrompt(h *HandState) ([]*HandMessageItem, error) {

	h.RunItTwicePrompt = true

	player1ID := uint64(0)
	player1Seat := uint32(0)
	player2ID := uint64(0)
	player2Seat := uint32(0)

	for seat, playerID := range h.ActiveSeats {
		if playerID == 0 {
			continue
		}
		if player1ID == 0 {
			player1ID = playerID
			player1Seat = uint32(seat)
		} else {
			player2ID = playerID
			player2Seat = uint32(seat)
			break
		}
	}

	// +1 second buffer to account for network delay to the client
	timeoutAt := time.Now().Add(time.Duration(h.ActionTime+1) * time.Second)

	// create run it twice
	h.RunItTwice = &RunItTwice{
		Stage:      h.LastState,
		Seat1:      player1Seat,
		Seat2:      player2Seat,
		ExpiryTime: uint64(timeoutAt.Unix()),
	}

	var msgItems []*HandMessageItem
	// run a timer for the prompt
	timeoutAtUnix := timeoutAt.Unix()

	// prompt player 1
	seatAction := &NextSeatAction{
		AvailableActions:    []ACTION{ACTION_RUN_IT_TWICE_PROMPT},
		SeatNo:              player1Seat,
		ActionTimesoutAt:    timeoutAtUnix,
		SecondsTillTimesout: uint32(h.RunItTwiceTimeout),
	}
	player1MsgItem := &HandMessageItem{
		MessageType: HandPlayerAction,
		Content:     &HandMessageItem_SeatAction{SeatAction: seatAction},
	}
	msgItems = append(msgItems, player1MsgItem)

	// prompt player 2
	seatAction = &NextSeatAction{
		AvailableActions:    []ACTION{ACTION_RUN_IT_TWICE_PROMPT},
		SeatNo:              player2Seat,
		ActionTimesoutAt:    timeoutAtUnix,
		SecondsTillTimesout: uint32(h.RunItTwiceTimeout),
	}
	player2MsgItem := &HandMessageItem{
		MessageType: HandPlayerAction,
		Content:     &HandMessageItem_SeatAction{SeatAction: seatAction},
	}
	msgItems = append(msgItems, player2MsgItem)

	//h.NextSeatAction.ActionTimesoutAt = timeoutAt.Unix()
	g.runItTwiceTimer(player1Seat, player1ID, player2Seat, player2ID, timeoutAt)

	return msgItems, nil
}

// handle run-it-twice confirmation
func (g *Game) runItTwiceConfirmation(h *HandState, message *HandMessage) ([]*HandMessageItem, error) {
	actionMsg := g.getClientMsgItem(message)
	g.logger.Info().
		Uint32(logging.SeatNumKey, message.SeatNo).
		Str("message", actionMsg.MessageType).
		Msgf("Run it twice confirmation: %d", actionMsg.GetPlayerActed().Action)
	action := actionMsg.GetPlayerActed().Action
	runItTwice := h.RunItTwice

	var log *HandActionLog
	switch runItTwice.Stage {
	case HandStatus_PREFLOP:
		log = h.PreflopActions
	case HandStatus_FLOP:
		log = h.FlopActions
	case HandStatus_TURN:
		log = h.TurnActions
	case HandStatus_RIVER:
		log = h.RiverActions
	}

	if runItTwice.Seat1 == message.SeatNo {
		runItTwice.Seat1Responded = true
		if action == ACTION_RUN_IT_TWICE_YES {
			runItTwice.Seat1Confirmed = true
		}
		log.Actions = append(log.Actions, actionMsg.GetPlayerActed())
		// we need to acknowledge message
	}

	if runItTwice.Seat2 == message.SeatNo {
		runItTwice.Seat2Responded = true
		if action == ACTION_RUN_IT_TWICE_YES {
			runItTwice.Seat2Confirmed = true
		}
		log.Actions = append(log.Actions, actionMsg.GetPlayerActed())

		// we need to acknowledge message
	}
	if runItTwice.Seat1 == message.SeatNo {
		g.pausePlayTimer(message.SeatNo)
	}
	if runItTwice.Seat2 == message.SeatNo {
		g.pausePlayTimer2(message.SeatNo)
	}

	var allMsgItems []*HandMessageItem

	// Broadcast this player's confirmation msg (or the default timeout msg) back to everyone.
	// Commenting this out because it is causing the board cards to show in the app UI without
	// waiting for the other player's confirmation.
	// allMsgItems = append(allMsgItems, actionMsg)

	msgItems, err := g.handleRunItTwice(h)
	if err != nil {
		return nil, err
	}

	for _, msgItem := range msgItems {
		allMsgItems = append(allMsgItems, msgItem)
	}

	return allMsgItems, nil
}

func (g *Game) handleRunItTwice(h *HandState) ([]*HandMessageItem, error) {
	runItTwice := h.RunItTwice

	boardCards := make([]uint32, 5)
	for i, card := range h.BoardCards {
		boardCards[i] = uint32(card)
	}
	fmt.Printf("Board1: %s\n", poker.CardsToString(boardCards))

	var allMsgItems []*HandMessageItem

	if !runItTwice.Seat1Responded || !runItTwice.Seat2Responded {
		return allMsgItems, nil
	}

	if runItTwice.Seat1Confirmed && runItTwice.Seat2Confirmed {
		// run two boards
		g.logger.Info().
			Uint32("handNum", h.HandNum).
			Msgf("Both seats YES. Running two boards")
		h.RunItTwiceConfirmed = true

		deck := poker.DeckFromBytes(h.Deck)
		fmt.Printf("Deck: %s\n", poker.CardsToString(deck.GetBytes()))

		otherCards := deck.Draw(int(h.DeckIndex))
		fmt.Printf("Other Cards: %s\n", poker.CardsToString(otherCards))

		board2 := make([]byte, 0)
		flop := false
		turn := false
		river := false

		// get two boards and and run it twice
		if h.RunItTwice.Stage == HandStatus_PREFLOP {
			// all 5 cards
			flop = true
			turn = true
			river = true
		} else if h.RunItTwice.Stage == HandStatus_FLOP {
			turn = true
			river = true
			// turn card and river card
			board2 = append(board2, h.BoardCards[:3]...)
		} else if h.RunItTwice.Stage == HandStatus_TURN {
			river = true
			// river card
			// turn card and river card
			board2 = append(board2, h.BoardCards[:4]...)
		}

		if flop {
			// flop
			cards := deck.Draw(3)
			h.DeckIndex += 3
			for _, card := range cards {
				board2 = append(board2, card.GetByte())
			}
		}

		if turn {
			// turn
			if h.BurnCards {
				deck.Draw(1)
				h.DeckIndex++
			}
			cards := deck.Draw(1)
			h.DeckIndex++
			fmt.Printf("Cards: %s\n", poker.CardsToString(cards))
			for _, card := range cards {
				board2 = append(board2, card.GetByte())
			}
		}

		if river {
			// river
			if h.BurnCards {
				deck.Draw(1)
				h.DeckIndex++
			}
			cards := deck.Draw(1)
			fmt.Printf("Cards: %s\n", poker.CardsToString(cards))
			h.DeckIndex++
			for _, card := range cards {
				board2 = append(board2, card.GetByte())
			}
		}
		h.NoOfBoards++
		h.Boards = append(h.Boards, &Board{
			BoardNo: h.NoOfBoards,
			Cards:   poker.ByteCardsToUint32Cards(board2),
		})
		h.BoardCards_2 = board2
		fmt.Printf("Board2: %s\n", poker.CardsToString(board2))

		boardCards2 := make([]uint32, 5)
		for i, card := range h.BoardCards_2 {
			boardCards2[i] = uint32(card)
		}

		g.logger.Trace().Msgf("Board1: %s, Board2: %s", poker.CardsToString(boardCards), poker.CardsToString(boardCards2))

		pots := make([]*SeatsInPots, 0)
		for _, pot := range h.Pots {
			if pot.Pot != 0 {
				pots = append(pots, pot)
			}
		}

		// send the two boards to the app
		runItTwiceMessage := &RunItTwiceBoards{
			Board_1:   boardCards,
			Board_2:   boardCards2,
			Stage:     h.RunItTwice.Stage,
			Seat1:     h.RunItTwice.Seat1,
			Seat2:     h.RunItTwice.Seat2,
			SeatsPots: pots,
		}
		msgItem := &HandMessageItem{
			MessageType: HandRunItTwice,
			Content:     &HandMessageItem_RunItTwice{RunItTwice: runItTwiceMessage},
		}
		allMsgItems = append(allMsgItems, msgItem)

		msgItems, err := g.showdown(h)
		if err != nil {
			return nil, errors.Wrap(err, "Error from showdown")
		}
		for _, m := range msgItems {
			allMsgItems = append(allMsgItems, m)
		}
	} else {
		// one of the players didn't confirm
		g.logger.Info().
			Uint32("handNum", h.HandNum).
			Msgf("Running one board")
		h.RunItTwiceConfirmed = false

		// run a single board
		msgItems, err := g.allPlayersAllIn(h)
		if err != nil {
			return nil, errors.Wrap(err, "Error from allPlayersAllIn")
		}
		for _, m := range msgItems {
			allMsgItems = append(allMsgItems, m)
		}
	}

	return allMsgItems, nil
}
