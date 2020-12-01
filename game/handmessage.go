package game

import (
	"fmt"

	"voyager.com/server/poker"
)

func (game *Game) handleHandMessage(message *HandMessage) {
	channelGameLogger.Debug().
		Uint32("club", game.clubID).
		Uint64("game", game.gameID).
		Uint32("player", message.SeatNo).
		Str("message", message.MessageType).
		Msg(fmt.Sprintf("%v", message))

	switch message.MessageType {
	case HandPlayerActed:
		game.onPlayerActed(message)
	case HandQueryCurrentHand:
		game.onQueryCurrentHand(message)
	}
}

func (game *Game) onQueryCurrentHand(message *HandMessage) error {
	gameState, err := game.loadState()
	if err != nil {
		return err
	}

	// get hand state
	handState, err := game.loadHandState(gameState)
	if err != nil {
		return err
	}
	if handState == nil || handState.HandNum == 0 {
		currentHandState := CurrentHandState{
			HandNum: 0,
		}
		handStateMsg := &HandMessage{
			GameId:      game.gameID,
			PlayerId:    message.GetPlayerId(),
			HandNum:     0,
			MessageType: HandQueryCurrentHand,
			HandMessage: &HandMessage_CurrentHandState{CurrentHandState: &currentHandState},
		}

		game.sendHandMessageToPlayer(handStateMsg, message.GetPlayerId())
		return nil
	}

	currentHandState := CurrentHandState{
		HandNum:      handState.HandNum,
		GameType:     handState.GameType,
		CurrentRound: handState.CurrentState,
		BoardCards:   handState.BoardCards,
		BoardCards_2: handState.BoardCards_2,
	}
	currentHandState.PlayersActed = make(map[uint32]*PlayerActRound, 0)

	var playerSeatNo uint32
	for seatNo, pid := range handState.GetPlayersInSeats() {
		if pid == message.PlayerId {
			playerSeatNo = uint32(seatNo + 1)
			break
		}
	}

	for seatNo, action := range handState.GetPlayersActed() {
		if action.State == PlayerActState_PLAYER_ACT_EMPTY_SEAT {
			continue
		}
		currentHandState.PlayersActed[uint32(seatNo+1)] = action
	}

	if playerSeatNo != 0 {
		_, maskedCards := game.maskCards(handState.GetPlayersCards()[playerSeatNo],
			gameState.PlayersState[message.PlayerId].GameTokenInt)
		currentHandState.PlayerCards = fmt.Sprintf("%d", maskedCards)
	}
	currentHandState.NextSeatToAct = handState.NextSeatAction.SeatNo
	currentHandState.RemainingActionTime = game.remainingActionTime
	currentHandState.PlayersStack = make(map[uint64]float32, 0)
	for playerID, state := range handState.GetPlayersState() {
		currentHandState.PlayersStack[playerID] = state.Balance
	}
	currentHandState.NextSeatAction = handState.NextSeatAction

	handStateMsg := &HandMessage{
		ClubId:      game.clubID,
		GameId:      game.gameID,
		PlayerId:    message.GetPlayerId(),
		HandNum:     handState.HandNum,
		MessageType: HandQueryCurrentHand,
		HandMessage: &HandMessage_CurrentHandState{CurrentHandState: &currentHandState},
	}
	game.sendHandMessageToPlayer(handStateMsg, message.GetPlayerId())
	return nil
}

func (game *Game) onPlayerActed(message *HandMessage) error {

	channelGameLogger.Info().
		Uint32("club", game.clubID).
		Uint64("game", game.gameID).
		Uint32("player", message.SeatNo).
		Str("message", message.MessageType).
		Msg(fmt.Sprintf("%v", message))

	// pause play timer
	game.pausePlayTimer()

	gameState, err := game.loadState()
	if err != nil {
		return err
	}

	// get hand state
	handState, err := game.loadHandState(gameState)
	if err != nil {
		return err
	}

	if handState.NextSeatAction != nil && handState.NextSeatAction.SeatNo != message.GetPlayerActed().GetSeatNo() {
		// Unexpected seat acted.
		// This can happen when a player made a last-second action and the timeout was triggered
		// at the same time. We get two actions in that case - one last-minute action from the player,
		// and the other default action from the timeout handler. Discard the second action.
		return nil
	}

	err = handState.actionReceived(message.GetPlayerActed())
	if err != nil {
		return err
	}

	err = game.saveHandState(gameState, handState)
	if err != nil {
		return err
	}

	// broadcast this message to all the players
	game.broadcastHandMessage(message)

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

func (game *Game) gotoFlop(gameState *GameState, handState *HandState) {
	channelGameLogger.Info().
		Uint32("club", game.clubID).
		Uint64("game", game.gameID).
		Msg(fmt.Sprintf("Moving to %s", HandStatus_name[int32(handState.CurrentState)]))

	// we need to send flop cards to the board
	deck := poker.NewDeckFromBytes(handState.Deck, int(handState.DeckIndex))
	deck.Draw(1)
	handState.DeckIndex++
	cards := deck.Draw(3)
	handState.DeckIndex += 3
	boardCards := make([]uint32, 3)
	for i, card := range cards {
		boardCards[i] = uint32(card.GetByte())
	}
	handState.setupFlop(boardCards)
	game.saveHandState(gameState, handState)

	cardsStr := poker.CardsToString(boardCards)
	flopMessage := &Flop{Board: boardCards, CardsStr: cardsStr}
	handMessage := &HandMessage{ClubId: game.clubID,
		GameId:      game.gameID,
		HandNum:     handState.HandNum,
		MessageType: HandFlop,
		HandStatus:  handState.CurrentState}
	handMessage.HandMessage = &HandMessage_Flop{Flop: flopMessage}
	game.broadcastHandMessage(handMessage)
	game.saveHandState(gameState, handState)
}

func (game *Game) gotoTurn(gameState *GameState, handState *HandState) {
	channelGameLogger.Info().
		Uint32("club", game.clubID).
		Uint64("game", game.gameID).
		Msg(fmt.Sprintf("Moving to %s", HandStatus_name[int32(handState.CurrentState)]))

	// send turn card to the board
	deck := poker.NewDeckFromBytes(handState.Deck, int(handState.DeckIndex))
	deck.Draw(1)
	handState.DeckIndex++
	turn := uint32(deck.Draw(1)[0].GetByte())
	handState.setupTurn(turn)
	game.saveHandState(gameState, handState)

	cardsStr := poker.CardsToString(handState.BoardCards)
	boardCards := make([]uint32, len(handState.BoardCards))
	for i, card := range handState.BoardCards {
		boardCards[i] = uint32(card)
	}
	turnMessage := &Turn{Board: boardCards, TurnCard: uint32(turn), CardsStr: cardsStr}
	handMessage := &HandMessage{ClubId: game.clubID,
		GameId:      game.gameID,
		HandNum:     handState.HandNum,
		MessageType: HandTurn,
		HandStatus:  handState.CurrentState}
	handMessage.HandMessage = &HandMessage_Turn{Turn: turnMessage}
	game.broadcastHandMessage(handMessage)
	game.saveHandState(gameState, handState)
}

func (game *Game) gotoRiver(gameState *GameState, handState *HandState) {
	channelGameLogger.Info().
		Uint32("club", game.clubID).
		Uint64("game", game.gameID).
		Msg(fmt.Sprintf("Moving to %s", HandStatus_name[int32(handState.CurrentState)]))

	// send river card to the board
	deck := poker.NewDeckFromBytes(handState.Deck, int(handState.DeckIndex))
	deck.Draw(1)
	handState.DeckIndex++
	river := uint32(deck.Draw(1)[0].GetByte())
	handState.setupRiver(river)
	game.saveHandState(gameState, handState)

	cardsStr := poker.CardsToString(handState.BoardCards)
	boardCards := make([]uint32, len(handState.BoardCards))
	for i, card := range handState.BoardCards {
		boardCards[i] = uint32(card)
	}
	riverMessage := &River{Board: boardCards, RiverCard: uint32(river), CardsStr: cardsStr}
	handMessage := &HandMessage{ClubId: game.clubID,
		GameId:      game.gameID,
		HandNum:     handState.HandNum,
		MessageType: HandRiver,
		HandStatus:  handState.CurrentState}
	handMessage.HandMessage = &HandMessage_River{River: riverMessage}
	game.broadcastHandMessage(handMessage)
	game.saveHandState(gameState, handState)
}

func (game *Game) sendWinnerBeforeShowdown(gameState *GameState, handState *HandState) error {
	// every one folded except one player, send the pot to the player
	handState.everyOneFoldedWinners()
	err := game.saveHandState(gameState, handState)
	if err != nil {
		return err
	}
	// send the hand to the database to store first
	handResult := handState.getResult()

	// now send the data to users
	handMessage := &HandMessage{
		ClubId:      game.clubID,
		GameId:      game.gameID,
		HandNum:     handState.HandNum,
		MessageType: HandResultMessage,
		HandStatus:  handState.CurrentState,
	}

	handMessage.HandMessage = &HandMessage_HandResult{HandResult: handResult}
	game.broadcastHandMessage(handMessage)

	// send a message to game to start new hand
	gameMessage := &GameMessage{
		GameId:      game.gameID,
		MessageType: GameMoveToNextHand,
	}
	game.SendGameMessage(gameMessage)

	return nil
}

func (game *Game) moveToNextRound(gameState *GameState, handState *HandState) {
	if handState.LastState == HandStatus_DEAL {
		return
	}

	if handState.LastState == HandStatus_PREFLOP && handState.CurrentState == HandStatus_FLOP {
		game.gotoFlop(gameState, handState)
	} else if handState.LastState == HandStatus_FLOP && handState.CurrentState == HandStatus_TURN {
		game.gotoTurn(gameState, handState)
	} else if handState.LastState == HandStatus_TURN && handState.CurrentState == HandStatus_RIVER {
		game.gotoRiver(gameState, handState)
	} else if handState.LastState == HandStatus_RIVER && handState.CurrentState == HandStatus_SHOW_DOWN {
		game.gotoShowdown(gameState, handState)
	}
}

func (game *Game) moveToNextAct(gameState *GameState, handState *HandState) {
	if handState.isAllActivePlayersAllIn() {
		game.handleNoMoreActions(gameState, handState)
	} else {

		if handState.LastState != handState.CurrentState {
			// move to next round
			game.moveToNextRound(gameState, handState)
		}

		if handState.NextSeatAction != nil {
			// tell the next player to act
			nextSeatMessage := &HandMessage{
				ClubId:      game.clubID,
				GameId:      game.gameID,
				HandNum:     handState.HandNum,
				MessageType: HandPlayerAction,
			}
			var canCheck bool
			for _, action := range handState.NextSeatAction.AvailableActions {
				if action == ACTION_CHECK {
					canCheck = true
					break
				}
			}
			nextSeatMessage.HandMessage = &HandMessage_SeatAction{SeatAction: handState.NextSeatAction}
			game.broadcastHandMessage(nextSeatMessage)
			game.resetTimer(handState.NextSeatAction.SeatNo, handState.PlayersInSeats[handState.NextSeatAction.SeatNo], canCheck)

			// action moves to the next player
			actionChange := &ActionChange{
				SeatNo: handState.NextSeatAction.SeatNo,
				Pots:   handState.Pots,
			}
			message := &HandMessage{
				ClubId:      game.clubID,
				GameId:      game.gameID,
				HandNum:     handState.HandNum,
				HandStatus:  handState.CurrentState,
				MessageType: HandNextAction,
			}
			message.HandMessage = &HandMessage_ActionChange{ActionChange: actionChange}
			game.broadcastHandMessage(message)
		}
	}
}

func (game *Game) handleNoMoreActions(gameState *GameState, handState *HandState) {

	// broadcast the players no more actions
	handMessage := &NoMoreActions{
		Pots: handState.Pots,
	}
	message := &HandMessage{
		ClubId:      game.clubID,
		GameId:      game.gameID,
		HandNum:     handState.HandNum,
		HandStatus:  handState.CurrentState,
		MessageType: HandNoMoreActions,
	}
	message.HandMessage = &HandMessage_NoMoreActions{NoMoreActions: handMessage}
	game.broadcastHandMessage(message)
	for handState.CurrentState != HandStatus_SHOW_DOWN {
		switch handState.CurrentState {
		case HandStatus_FLOP:
			game.gotoFlop(gameState, handState)
			handState.CurrentState = HandStatus_TURN
		case HandStatus_TURN:
			game.gotoTurn(gameState, handState)
			handState.CurrentState = HandStatus_RIVER
		case HandStatus_RIVER:
			game.gotoRiver(gameState, handState)
			handState.CurrentState = HandStatus_SHOW_DOWN
		}
	}
	game.gotoShowdown(gameState, handState)
}

func (game *Game) gotoShowdown(gameState *GameState, handState *HandState) {
	evaluate := NewHoldemWinnerEvaluate(gameState, handState)
	if gameState.GameType == GameType_HOLDEM {
		evaluate.evaluate()
		handState.HandCompletedAt = HandStatus_SHOW_DOWN
		handState.setWinners(evaluate.winners)

		// send the hand to the database to store first
		handResult := handState.getResult()

		// now send the data to users
		handMessage := &HandMessage{
			ClubId:      game.clubID,
			GameId:      game.gameID,
			HandNum:     handState.HandNum,
			MessageType: HandResultMessage,
			HandStatus:  handState.CurrentState,
		}

		handMessage.HandMessage = &HandMessage_HandResult{HandResult: handResult}
		game.broadcastHandMessage(handMessage)

		// send a message to game to start new hand
		gameMessage := &GameMessage{
			GameId:      game.gameID,
			MessageType: GameMoveToNextHand,
		}
		game.SendGameMessage(gameMessage)
		_ = 0
	}
}
