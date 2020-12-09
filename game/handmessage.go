package game

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
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

	cardsStr := poker.CardsToString(handState.BoardCards)
	boardCards := make([]uint32, len(handState.BoardCards))
	for i, card := range handState.BoardCards {
		boardCards[i] = uint32(card)
	}

	pots := make([]float32, 0)
	for _, pot := range handState.Pots {
		pots = append(pots, pot.Pot)
	}
	currentPot := pots[len(pots)-1]
	currentBettingRound := handState.RoundBetting[uint32(handState.CurrentState)]
	seatBets := currentBettingRound.SeatBet
	for _, bet := range seatBets {
		currentPot = currentPot + bet
	}

	currentHandState := CurrentHandState{
		HandNum:      handState.HandNum,
		GameType:     handState.GameType,
		CurrentRound: handState.CurrentState,
		BoardCards:   boardCards,
		BoardCards_2: nil,
		CardsStr:     cardsStr,
		Pots:         pots,
		PotUpdates:   currentPot,
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
	playerState := handState.GetPlayersState()
	for seatNo, playerID := range handState.GetPlayersInSeats() {
		if playerID == 0 {
			continue
		}
		currentHandState.PlayersStack[uint64(seatNo)] = playerState[playerID].Balance
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
	/*
		deck := poker.NewDeckFromBytes(handState.Deck, int(handState.DeckIndex))
		deck.Draw(1)
		handState.DeckIndex++
		cards := deck.Draw(3)
		handState.DeckIndex += 3
	*/
	boardCards := make([]uint32, 3)
	for i, card := range handState.FlopCards {
		boardCards[i] = card
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
	/*
		deck := poker.NewDeckFromBytes(handState.Deck, int(handState.DeckIndex))
		deck.Draw(1)
		handState.DeckIndex++
		turn := uint32(deck.Draw(1)[0].GetByte())
	*/
	handState.setupTurn(handState.TurnCard)
	game.saveHandState(gameState, handState)

	cardsStr := poker.CardsToString(handState.BoardCards)
	boardCards := make([]uint32, len(handState.BoardCards))
	for i, card := range handState.BoardCards {
		boardCards[i] = uint32(card)
	}
	turnMessage := &Turn{Board: boardCards, TurnCard: uint32(handState.TurnCard), CardsStr: cardsStr}
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
	/*
		deck := poker.NewDeckFromBytes(handState.Deck, int(handState.DeckIndex))
		deck.Draw(1)
		handState.DeckIndex++
		river := uint32(deck.Draw(1)[0].GetByte())
	*/

	handState.setupRiver(handState.RiverCard)
	game.saveHandState(gameState, handState)

	cardsStr := poker.CardsToString(handState.BoardCards)
	boardCards := make([]uint32, len(handState.BoardCards))
	for i, card := range handState.BoardCards {
		boardCards[i] = uint32(card)
	}
	riverMessage := &River{Board: boardCards, RiverCard: uint32(handState.RiverCard), CardsStr: cardsStr}
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

	// save the hand
	game.saveHand(gameState, handState, handResult, nil)

	handMessage.HandMessage = &HandMessage_HandResult{HandResult: handResult}
	game.broadcastHandMessage(handMessage)

	// send a message to game to start new hand
	gameMessage := &GameMessage{
		GameId:      game.gameID,
		MessageType: GameMoveToNextHand,
	}
	go game.SendGameMessage(gameMessage)

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
			playerID := handState.PlayersInSeats[handState.NextSeatAction.SeatNo-1]
			game.sendHandMessageToPlayer(nextSeatMessage, playerID)
			game.resetTimer(handState.NextSeatAction.SeatNo, playerID, canCheck)

			pots := make([]float32, 0)
			for _, pot := range handState.Pots {
				pots = append(pots, pot.Pot)
			}
			currentPot := pots[len(pots)-1]
			currentBettingRound := handState.RoundBetting[uint32(handState.CurrentState)]
			seatBets := currentBettingRound.SeatBet
			for _, bet := range seatBets {
				currentPot = currentPot + bet
			}

			// action moves to the next player
			actionChange := &ActionChange{
				SeatNo:     handState.NextSeatAction.SeatNo,
				Pots:       pots,
				PotUpdates: currentPot,
				SeatsPots:  handState.Pots,
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

		// save the hand
		game.saveHand(gameState, handState, handResult, evaluate)

		handMessage.HandMessage = &HandMessage_HandResult{HandResult: handResult}
		game.broadcastHandMessage(handMessage)

		// send a message to game to start new hand
		gameMessage := &GameMessage{
			GameId:      game.gameID,
			MessageType: GameMoveToNextHand,
		}
		go game.SendGameMessage(gameMessage)
		_ = 0
	}
}

func (g *Game) saveHand(gameState *GameState, h *HandState, handResult *HandResult, evaluate *HoldemWinnerEvaluate) error {
	/*
		message PlayerCards {
			repeated uint32 cards = 1;        // cards
			repeated uint32 best_cards = 2;   // best_cards
			uint32 rank = 3;                  // best rank
			HandStatus played_until = 4;      // played until what stage
		}

		message SaveResult  {
		uint64 game_id = 1;
		HandResult hand_result = 2;
		HighHand high_hand = 3;
		repeated uint64 reward_tracking_ids = 4;
		bytes board_cards = 5;
		bytes board_cards_2 = 6;  // run it twice
		bytes flop = 6;
		uint32 turn = 7;
		uint32 river = 8;
		map<uint64, PlayerCards> player_cards = 9;    // player cards with rank
		}
	*/
	var bestSeatHands map[uint32]*evaluatedCards
	if h.BoardCards != nil {
		if evaluate == nil {
			evaluate = NewHoldemWinnerEvaluate(gameState, h)
			if gameState.GameType == GameType_HOLDEM {
				evaluate.evaluate()
			}
		}
		bestSeatHands = evaluate.getEvaluatedCards()
		fmt.Printf("\n\n================================================================\n\n")
		for seatNo, hand := range bestSeatHands {
			fmt.Printf("Seat: %d, Cards:%+v, Str: %s Rank: %d, rankStr: %s\n", seatNo, hand.cards,
				poker.CardsToString(hand.cards), hand.rank, poker.RankString(hand.rank))
		}
		fmt.Printf("\n\n================================================================\n\n")
	}

	// saves hand result in the database
	result := &SaveResult{
		RewardTrackingIds: gameState.RewardTrackingIds,
		HandResult:        handResult,
		Turn:              h.TurnCard,
		River:             h.RiverCard,
	}
	if h.BoardCards != nil {
		result.BoardCards = make([]uint32, len(h.BoardCards))
		for i, card := range h.BoardCards {
			result.BoardCards[i] = uint32(card)
		}
	}

	if h.BoardCards_2 != nil {
		result.BoardCards_2 = make([]uint32, len(h.BoardCards_2))
		for i, card := range h.BoardCards {
			result.BoardCards_2[i] = uint32(card)
		}
	}

	if h.FlopCards != nil {
		result.Flop = make([]uint32, len(h.FlopCards))
		for i, card := range h.FlopCards {
			result.Flop[i] = uint32(card)
		}
	}

	result.PlayerCards = make(map[uint32]*PlayerCards, 0)
	for seatNo, cards := range h.PlayersCards {
		playerID := h.GetPlayersInSeats()[seatNo-1]
		if playerID == 0 {
			continue
		}
		playerState, _ := h.GetPlayersState()[playerID]
		playerCard := &PlayerCards{
			PlayedUntil: playerState.Round,
		}
		playerCard.Rank = uint32(0xFFFFFFFF)
		var evaluatedCards *evaluatedCards
		if bestSeatHands != nil {
			evaluatedCards = bestSeatHands[seatNo]
			if evaluatedCards != nil {
				playerCard.Rank = uint32(evaluatedCards.rank)
			}
		}

		playerCard.Cards = make([]uint32, len(cards))
		for i, card := range cards {
			playerCard.Cards[i] = uint32(card)
		}

		if evaluatedCards != nil {
			playerCard.BestCards = make([]uint32, len(evaluatedCards.cards))
			for i, card := range evaluatedCards.cards {
				playerCard.BestCards[i] = uint32(card)
			}
		}

		result.PlayerCards[seatNo] = playerCard
	}
	data, _ := protojson.Marshal(result)
	fmt.Printf("%s\n", string(data))
	_ = result
	return nil
}
