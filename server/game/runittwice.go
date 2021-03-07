package game

import (
	"fmt"
	"time"

	"voyager.com/server/poker"
)

func (g *Game) runItTwice(h *HandState) bool {

	if !h.hasEveryOneActed() {
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

			if !h.RunItTwiceOptedPlayers[seatNo] {
				prompt = false
				break
			}
		}

		return prompt
	}
	return false
}

func (g *Game) runItTwicePrompt(h *HandState) bool {

	h.RunItTwicePrompt = true

	player1 := uint64(0)
	player1Seat := uint32(0)
	player2 := uint64(0)
	player2Seat := uint32(0)

	for seat, playerID := range h.ActiveSeats {
		if playerID == 0 {
			continue
		}
		if player1 == 0 {
			player1 = playerID
			player1Seat = uint32(seat)
		} else {
			player2 = playerID
			player2Seat = uint32(seat)
			break
		}
	}

	expiryTime := time.Now().Add(time.Second * time.Duration(g.config.ActionTime))

	// create run it twice
	h.RunItTwice = &RunItTwice{
		Stage:      h.LastState,
		Seat1:      player1Seat,
		Seat2:      player2Seat,
		ExpiryTime: uint64(expiryTime.Unix()),
	}

	// prompt player 1
	nextSeatMessage := &HandMessage{
		GameId:      h.GameId,
		HandNum:     h.HandNum,
		MessageType: HandPlayerAction,
		HandStatus:  h.CurrentState,
		SeatNo:      player1Seat,
	}
	availableActions := []ACTION{ACTION_RUN_IT_TWICE_PROMPT}
	seatAction := &NextSeatAction{
		AvailableActions: availableActions,
		SeatNo:           player1Seat,
	}
	nextSeatMessage.HandMessage = &HandMessage_SeatAction{SeatAction: seatAction}
	g.sendHandMessageToPlayer(nextSeatMessage, player1)

	// prompt player 2
	nextSeatMessage = &HandMessage{
		GameId:      h.GameId,
		HandNum:     h.HandNum,
		MessageType: HandPlayerAction,
		HandStatus:  h.CurrentState,
		SeatNo:      player2Seat,
	}
	seatAction = &NextSeatAction{
		AvailableActions: availableActions,
		SeatNo:           player2Seat,
	}
	nextSeatMessage.HandMessage = &HandMessage_SeatAction{SeatAction: seatAction}
	g.sendHandMessageToPlayer(nextSeatMessage, player2)

	// run a timer for the prompt
	g.runItTwiceTimer(player1Seat, player1, player2Seat, player2)

	return true
}

func (g *Game) handleRunitTwiceTimeout(h *HandState) bool {
	return true
}

// handle run-it-twice confirmation
func (g *Game) runItTwiceConfirmation(h *HandState, message *HandMessage) {
	channelGameLogger.Info().
		Str("game", g.config.GameCode).
		Uint32("seatNo", message.SeatNo).
		Str("message", message.MessageType).
		Msgf("Run it twice confirmation: %d", message.GetPlayerActed().Action)
	action := message.GetPlayerActed().Action
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
		log.Actions = append(log.Actions, message.GetPlayerActed())
		// we need to acknowledge message
	}

	if runItTwice.Seat2 == message.SeatNo {
		runItTwice.Seat2Responded = true
		if action == ACTION_RUN_IT_TWICE_YES {
			runItTwice.Seat2Confirmed = true
		}
		log.Actions = append(log.Actions, message.GetPlayerActed())

		// we need to acknowledge message
	}
	g.handleRunItTwice(h)

	g.saveHandState(h)
}

func (g *Game) handleRunItTwice(h *HandState) {
	runItTwice := h.RunItTwice

	boardCards := make([]uint32, 5)
	for i, card := range h.BoardCards {
		boardCards[i] = uint32(card)
	}
	fmt.Printf("Board1: %s\n", poker.CardsToString(boardCards))

	if runItTwice.Seat1Responded && runItTwice.Seat2Responded {
		if runItTwice.Seat1Confirmed && runItTwice.Seat2Confirmed {
			// run two boards
			channelGameLogger.Info().
				Str("game", g.config.GameCode).
				Uint32("handNum", h.HandNum).
				Msgf("Run two boards")
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

			h.BoardCards_2 = board2

			boardCards2 := make([]uint32, 5)
			for i, card := range h.BoardCards_2 {
				boardCards2[i] = uint32(card)
			}

			fmt.Printf("Board1: %s, Board2: %s\n", poker.CardsToString(boardCards), poker.CardsToString(boardCards2))

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
			handMessage := &HandMessage{ClubId: g.config.ClubId,
				GameId:      g.config.GameId,
				HandNum:     h.HandNum,
				MessageType: HandRunItTwice,
				HandStatus:  h.RunItTwice.Stage}
			handMessage.HandMessage = &HandMessage_RunItTwice{RunItTwice: runItTwiceMessage}
			g.broadcastHandMessage(handMessage)
			if !RunningTests {
				time.Sleep(time.Duration(g.delays.GoToFlop) * time.Millisecond)
			}

			h.FlowState = FlowState_SHOWDOWN
			g.saveHandState(h)
			g.showdown(h)
		} else {
			// one of the players didn't confirm
			channelGameLogger.Info().
				Str("game", g.config.GameCode).
				Uint32("handNum", h.HandNum).
				Msgf("Run one board")
			h.RunItTwiceConfirmed = false

			// run a single board
			h.FlowState = FlowState_ALL_PLAYERS_ALL_IN
			g.allPlayersAllIn(h)
			return
		}
	}
}
