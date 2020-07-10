package game

func initializePot(maxSeats int) *SeatsInPots {
	// this will create an array to represent each set in the table
	seats := make([]uint32, maxSeats)
	return &SeatsInPots{
		Seats: seats,
		Pot:   0.0,
	}
}

func (s *SeatsInPots) add(seatNo uint32, amount float32) {
	s.Seats[seatNo-1] = 1
	s.Pot += amount
}

func (h *HandState) lowestBet(seatBets []float32) float32 {
	lowestBet := float32(0.0)
	for seatNo, bet := range seatBets {
		if h.PlayersInSeats[seatNo] == 0 {
			// empty seat
			continue
		}
		if lowestBet == 0 {
			lowestBet = bet
		} else if bet < lowestBet {
			lowestBet = bet
		}
	}
	// if 0, every one checked or no more bets remaining
	return lowestBet
}

func (h *HandState) addChipsToPot(seatBets []float32, handEnded bool) {
	currentPotIndex := len(h.Pots) - 1
	currentPot := h.Pots[currentPotIndex]
	lowestBet := h.lowestBet(seatBets)
	for seatNoIdx, bet := range seatBets {
		if h.PlayersInSeats[seatNoIdx] == 0 || seatBets[seatNoIdx] == 0.0 {
			// empty seat
			continue
		}
		seatNo := seatNoIdx + 1
		currentPot.add(uint32(seatNo), lowestBet)
		seatBets[seatNoIdx] = bet - lowestBet
	}

	if handEnded {
		// put all remaining bets in the pot
		for seatNo, bet := range seatBets {
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
			initializePot(len(seatBets))
			h.addChipsToPot(seatBets, handEnded)
		}
	}
}
