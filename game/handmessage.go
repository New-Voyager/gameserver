package game

import (
	"bytes"
	"fmt"
	"net/http"

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
		HandNum:       handState.HandNum,
		GameType:      handState.GameType,
		CurrentRound:  handState.CurrentState,
		BoardCards:    boardCards,
		BoardCards_2:  nil,
		CardsStr:      cardsStr,
		Pots:          pots,
		PotUpdates:    currentPot,
		ButtonPos:     handState.ButtonPos,
		SmallBlindPos: handState.SmallBlindPos,
		BigBlindPos:   handState.BigBlindPos,
		SmallBlind:    handState.SmallBlind,
		BigBlind:      handState.BigBlind,
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
	for seatNoIdx, playerID := range handState.GetPlayersInSeats() {
		if playerID == 0 {
			continue
		}
		seatNo := seatNoIdx + 1
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

	// Send player's current stack to be updated in the UI
	seatNo := message.GetPlayerActed().GetSeatNo()
	playerID := handState.PlayersInSeats[seatNo-1]

	message.GetPlayerActed().Stack = handState.PlayersState[playerID].Balance
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
	pots := make([]float32, 0)
	for _, pot := range handState.Pots {
		pots = append(pots, pot.Pot)
	}

	cardsStr := poker.CardsToString(boardCards)
	flopMessage := &Flop{Board: boardCards, CardsStr: cardsStr, Pots: pots, SeatsPots: handState.Pots}
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
	pots := make([]float32, 0)
	for _, pot := range handState.Pots {
		pots = append(pots, pot.Pot)
	}
	turnMessage := &Turn{Board: boardCards, TurnCard: uint32(handState.TurnCard), CardsStr: cardsStr, Pots: pots, SeatsPots: handState.Pots}
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
	pots := make([]float32, 0)
	for _, pot := range handState.Pots {
		pots = append(pots, pot.Pot)
	}
	riverMessage := &River{Board: boardCards, RiverCard: uint32(handState.RiverCard), CardsStr: cardsStr, Pots: pots, SeatsPots: handState.Pots}
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

	// now send the data to users
	handMessage := &HandMessage{
		ClubId:      game.clubID,
		GameId:      game.gameID,
		HandNum:     handState.HandNum,
		MessageType: HandResultMessage,
		HandStatus:  handState.CurrentState,
	}

	// send the hand to the database to store first
	handResult := game.getHandResult(gameState, handState, nil, true /*db*/)
	game.saveHandResult(handResult)

	// send to all the players
	handResult = game.getHandResult(gameState, handState, nil, false /*db*/)
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

		// now send the data to users
		handMessage := &HandMessage{
			ClubId:      game.clubID,
			GameId:      game.gameID,
			HandNum:     handState.HandNum,
			MessageType: HandResultMessage,
			HandStatus:  handState.CurrentState,
		}

		// send the hand to the database to store first
		handResult := game.getHandResult(gameState, handState, nil, true /*db*/)
		game.saveHandResult(handResult)

		handResult = game.getHandResult(gameState, handState, nil, false /*db*/)
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

func (g *Game) getHandResult(gameState *GameState, h *HandState, evaluate *HoldemWinnerEvaluate, database bool) *HandResult {
	var bestSeatHands map[uint32]*evaluatedCards
	var highHands map[uint32]*evaluatedCards

	if h.BoardCards != nil {
		if evaluate == nil {
			evaluate = NewHoldemWinnerEvaluate(gameState, h)
			if gameState.GameType == GameType_HOLDEM {
				evaluate.evaluate()
			}
		}
		// evaluate player's high hands always
		evaluate.evaluatePlayerHighHand()

		bestSeatHands = evaluate.getEvaluatedCards()
		highHands = evaluate.getHighhandCards()
		fmt.Printf("\n\n================================================================\n\n")
		for seatNo, hand := range bestSeatHands {
			highHand := highHands[seatNo]
			fmt.Printf("Seat: %d, Cards:%+v, Str: %s Rank: %d, rankStr: %s, hhHand: %s rank: %d rankStr: %s\n",
				seatNo,
				hand.cards,
				poker.CardsToString(hand.cards), hand.rank, poker.RankString(hand.rank),
				poker.CardToString(highHand.cards), highHand.rank, poker.RankString((highHand.rank)))
		}
		fmt.Printf("\n\n================================================================\n\n")
	}
	handResult := &HandResult{
		GameId:   gameState.GameId,
		HandNum:  h.HandNum,
		GameType: h.GameType,
	}

	handResult.HandLog = h.getLog()

	handResult.RewardTrackingIds = g.rewardTrackingIds
	handResult.Turn = h.TurnCard
	handResult.River = h.RiverCard
	if h.BoardCards != nil {
		handResult.BoardCards = make([]uint32, len(h.BoardCards))
		for i, card := range h.BoardCards {
			handResult.BoardCards[i] = uint32(card)
		}
	}

	if h.BoardCards_2 != nil {
		handResult.BoardCards_2 = make([]uint32, len(h.BoardCards_2))
		for i, card := range h.BoardCards {
			handResult.BoardCards_2[i] = uint32(card)
		}
	}

	if h.FlopCards != nil {
		handResult.Flop = make([]uint32, len(h.FlopCards))
		for i, card := range h.FlopCards {
			handResult.Flop[i] = uint32(card)
		}
	}
	handResult.Players = make(map[uint32]*PlayerInfo, 0)
	for seatNoIdx, playerID := range h.GetPlayersInSeats() {

		// no player in the seat
		if playerID == 0 {
			continue
		}

		// determine whether the player has folded
		playerFolded := false
		if h.ActiveSeats[seatNoIdx] == 0 {
			playerFolded = true
		}

		seatNo := uint32(seatNoIdx + 1)
		balanceBefore := float32(0)
		balanceAfter := float32(0)
		for _, playerBalance := range h.BalanceBeforeHand {
			if playerID == playerBalance.PlayerId {
				balanceBefore = playerBalance.Balance
				break
			}
		}

		for _, playerBalance := range h.BalanceAfterHand {
			if playerID == playerBalance.PlayerId {
				balanceAfter = playerBalance.Balance
				break
			}
		}

		// calculate high rank only the player hasn't folded
		rank := uint32(0xFFFFFFFF)
		highHandRank := uint32(0xFFFFFFFF)
		var bestCards []uint32
		var highHandBestCards []uint32

		cards := h.PlayersCards[seatNo]
		playerCards := make([]uint32, len(cards))
		for i, card := range cards {
			playerCards[i] = uint32(card)
		}
		if !playerFolded {
			var evaluatedCards *evaluatedCards
			if bestSeatHands != nil {
				evaluatedCards = bestSeatHands[seatNo]
				if evaluatedCards != nil {
					rank = uint32(evaluatedCards.rank)
				}
			}

			if evaluatedCards != nil {
				bestCards = make([]uint32, len(evaluatedCards.cards))
				for i, card := range evaluatedCards.cards {
					bestCards[i] = uint32(card)
				}
			}
			if highHands != nil {
				if highHands[seatNo] != nil {
					highHandRank = uint32(highHands[seatNo].rank)
					highHandBestCards = highHands[seatNo].getCards()
				}
			}
		}

		playerState, _ := h.GetPlayersState()[playerID]
		playerInfo := &PlayerInfo{
			Id:          playerID,
			PlayedUntil: playerState.Round,
			Balance: &HandPlayerBalance{
				Before: balanceBefore,
				After:  balanceAfter,
			},
		}

		if !playerFolded || database {
			// player is active or the result is stored in database
			playerInfo.Cards = playerCards
			playerInfo.BestCards = bestCards
			playerInfo.Rank = rank
			playerInfo.HhCards = highHandBestCards
			playerInfo.HhRank = highHandRank
		}
		handResult.Players[seatNo] = playerInfo
	}

	return handResult
}

func (g *Game) saveHandResult(result *HandResult) {
	// call the API server to save the hand result
	var m protojson.MarshalOptions
	m.EmitUnpopulated = true
	data, _ := m.Marshal(result)
	fmt.Printf("%s\n", string(data))

	url := fmt.Sprintf("%s/internal/post-hand/gameId/%d/handNum/%d", g.apiServerUrl, result.GameId, result.HandNum)
	resp, _ := http.Post(url, "application/json", bytes.NewBuffer(data))
	// if the api server returns nil, do nothing
	if resp == nil {
		return
	}
	defer resp.Body.Close()
	fmt.Printf("Posted successfully")
}
