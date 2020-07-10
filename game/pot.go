package game

func initializePot(handState *HandState, gameState *GameState) *SeatsInPots {
	// this will create an array to represent each set in the table
	maxSeats := gameState.MaxSeats
	seats := make([]float32, maxSeats)
	return &SeatsInPots{
		SeatPotEquity: seats,
		Pot:           0.0,
	}
}
