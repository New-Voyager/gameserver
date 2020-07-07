package game

import "fmt"

func (game *Game) handleHandMessage(message *HandMessage) {
	channelGameLogger.Debug().
		Uint32("club", game.clubID).
		Uint32("game", game.gameNum).
		Msg(fmt.Sprintf("Hand message: %s", message.MessageType))

	if message.MessageType == HandPlayerActed {
		game.onPlayerActed(message)
	}
}

func (game *Game) onPlayerActed(message *HandMessage) error {
	gameState, err := game.loadState()
	if err != nil {
		return err
	}

	// get hand state
	handState, err := game.loadHandState(gameState)
	if err != nil {
		return err
	}

	err = handState.actionReceived(gameState, message.GetPlayerActed())
	if err != nil {
		return err
	}

	err = game.saveHandState(gameState, handState)
	if err != nil {
		return err
	}

	// if only one player is remaining in the hand, we have a winner
	if handState.NoActiveSeats == 1 {
		winner := uint32(0)
		seatNo := -1
		// we have a winner, find out who the winner is
		for i, playerID := range handState.ActiveSeats {
			if playerID != 0 {
				winner = playerID
				seatNo = i + 1
				break
			}
		}
		fmt.Printf("Winner is %d at seat %d\n", winner, seatNo)
	} else {
		// if the current player is where the action ends, move to the next round

		// action moves to the next player
		actionChange := &ActionChange{SeatNo: handState.NextSeatAction.SeatNo}
		message := &HandMessage{
			ClubId:      game.clubID,
			GameNum:     game.gameNum,
			HandNum:     handState.HandNum,
			MessageType: HandNextAction,
		}
		message.HandMessage = &HandMessage_ActionChange{ActionChange: actionChange}
		game.broadcastHandMessage(message)

		// tell the next player to act
		nextSeatMessage := &HandMessage{
			ClubId:      game.clubID,
			GameNum:     game.gameNum,
			HandNum:     handState.HandNum,
			MessageType: HandPlayerAction,
		}
		nextSeatMessage.HandMessage = &HandMessage_SeatAction{SeatAction: handState.NextSeatAction}
		playerID := handState.PlayersInSeats[handState.NextSeatAction.SeatNo-1]
		player := game.allPlayers[playerID]
		game.sendHandMessageToPlayer(nextSeatMessage, player)
	}

	return nil
}
