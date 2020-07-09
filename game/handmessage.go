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
		game.sendWinnerBeforeShowdown(gameState, handState)
		// result of the hand is sent

		// wait for the animation to complete before we send the next hand
		// if it is not auto deal, we return from here
		if !game.autoDeal {
			return nil
		}
	} else {
		// if the current player is where the action ends, move to the next round
		game.moveToNextAct(gameState, handState)
	}

	return nil
}

func (game *Game) sendWinnerBeforeShowdown(gameState *GameState, handState *HandState) error {
	// every one folded except one player, send the pot to the player

	// we need to deal with all in players as well

	// determine winners
	handState.determineWinners()
	err := game.saveHandState(gameState, handState)
	if err != nil {
		return err
	}
	// send the hand to the database to store first
	handResult := handState.getResult()

	// now send the data to users
	handMessage := &HandMessage{
		ClubId:      game.clubID,
		GameNum:     game.gameNum,
		HandNum:     handState.HandNum,
		MessageType: HandResultMessage,
		HandStatus:  handState.CurrentState,
	}

	handMessage.HandMessage = &HandMessage_HandResult{HandResult: handResult}
	game.broadcastHandMessage(handMessage)

	return nil
}

func (game *Game) moveToNextAct(gameState *GameState, handState *HandState) {
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
}
