package game

func initializePot(maxSeats int) *SeatsInPots {
	// this will contain only the seats that are in the pot
	seats := make([]uint32, 0)
	return &SeatsInPots{
		Seats: seats,
		Pot:   0.0,
	}
}

func (s *SeatsInPots) add(seatNo uint32, amount int64) {
	found := false
	for i := range s.Seats {
		if seatNo == s.Seats[i] {
			// already in the pot
			found = true
			break
		}
	}
	if !found {
		s.Seats = append(s.Seats, seatNo)
	}
	s.Pot += amount
}

func (h *HandState) lowestBet(seatBets []int64) int64 {
	lowestBet := int64(-1)
	for seatNo, bet := range seatBets {
		// if !h.PlayersInSeats[seatNo].OpenSeat ||
		//    !h.PlayersInSeats[seatNo].Inhand  {
		// 	// empty seat
		// 	continue
		// }

		// also eliminate the player who is not active any longer
		if h.ActiveSeats[seatNo] == 0 || bet == 0 {
			continue
		}

		if lowestBet == -1 {
			lowestBet = bet
			continue
		}
		if bet < lowestBet {
			lowestBet = bet
		}
	}

	if lowestBet == -1 {
		lowestBet = 0
	}
	// if 0, every one checked or no more bets remaining
	return lowestBet
}

func (h *HandState) addChipsToPot(seatBets []int64, handEnded bool) {
	currentPotIndex := len(h.Pots) - 1
	currentPot := h.Pots[currentPotIndex]
	lowestBet := h.lowestBet(seatBets)
	allInPlayers := false
	for seatNo, bet := range seatBets {
		if seatBets[seatNo] == 0.0 {
			// empty seat
			continue
		}

		// player has a bet here
		// is he all in?
		if h.PlayersActed[seatNo].Action == ACTION_ALLIN {
			allInPlayers = true
		}

		if bet < lowestBet {
			// the player folded
			currentPot.add(uint32(seatNo), bet)
			h.PlayersActed[seatNo].BetAmount += bet
			seatBets[seatNo] = 0
		} else {
			currentPot.add(uint32(seatNo), lowestBet)
			h.PlayersActed[seatNo].BetAmount += lowestBet
			seatBets[seatNo] = bet - lowestBet
		}
	}

	if handEnded {
		// put all remaining bets in the pot
		for seatNo, bet := range seatBets {
			if seatNo == 0 {
				continue
			}

			if bet > 0.0 {
				currentPot.add(uint32(seatNo), bet)
			}
		}
	} else {
		anyRemainingBets := 0
		for _, bet := range seatBets {
			if bet > 0.0 {
				anyRemainingBets++
			}
		}
		if anyRemainingBets > 1 {
			// add a new pot and calculate next pot
			newPot := initializePot(len(seatBets))
			h.Pots = append(h.Pots, newPot)
			h.addChipsToPot(seatBets, handEnded)
		} else if allInPlayers && anyRemainingBets != 1 {
			// add a new pot
			newPot := initializePot(len(seatBets))
			h.Pots = append(h.Pots, newPot)
			h.addChipsToPot(seatBets, handEnded)
		}
	}
}
