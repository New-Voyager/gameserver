package game

import (
	"bytes"
	"fmt"
	"net/http"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"voyager.com/server/poker"
)

func (g *Game) handleHandMessage(message *HandMessage) {
	channelGameLogger.Debug().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
		Uint32("player", message.SeatNo).
		Str("message", message.MessageType).
		Msg(fmt.Sprintf("%v", message))

	switch message.MessageType {
	case HandPlayerActed:
		g.onPlayerActed(message)
	case HandQueryCurrentHand:
		g.onQueryCurrentHand(message)
	}
}

func (g *Game) onQueryCurrentHand(message *HandMessage) error {
	gameState, err := g.loadState()
	if err != nil {
		return err
	}

	// get hand state
	handState, err := g.loadHandState(gameState)
	if err != nil {
		return err
	}

	if handState == nil || handState.HandNum == 0 || handState.CurrentState == HandStatus_HAND_CLOSED {
		currentHandState := CurrentHandState{
			HandNum: 0,
		}
		handStateMsg := &HandMessage{
			GameId:      g.config.GameId,
			PlayerId:    message.GetPlayerId(),
			HandNum:     0,
			MessageType: HandQueryCurrentHand,
			HandMessage: &HandMessage_CurrentHandState{CurrentHandState: &currentHandState},
		}

		g.sendHandMessageToPlayer(handStateMsg, message.GetPlayerId())
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
	bettingInProgress := handState.CurrentState == HandStatus_PREFLOP || handState.CurrentState == HandStatus_FLOP || handState.CurrentState == HandStatus_TURN || handState.CurrentState == HandStatus_RIVER
	if bettingInProgress {
		for _, bet := range currentBettingRound.SeatBet {
			currentPot = currentPot + bet
		}
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
		NoCards:       g.NumCards(gameState.GameType),
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
		_, maskedCards := g.maskCards(handState.GetPlayersCards()[playerSeatNo],
			gameState.PlayersState[message.PlayerId].GameTokenInt)
		currentHandState.PlayerCards = fmt.Sprintf("%d", maskedCards)
		currentHandState.PlayerSeatNo = playerSeatNo
	}

	if bettingInProgress {
		currentHandState.NextSeatToAct = handState.NextSeatAction.SeatNo
		currentHandState.RemainingActionTime = g.remainingActionTime
		currentHandState.NextSeatAction = handState.NextSeatAction
	}
	currentHandState.PlayersStack = make(map[uint64]float32, 0)
	playerState := handState.GetPlayersState()
	for seatNoIdx, playerID := range handState.GetPlayersInSeats() {
		if playerID == 0 {
			continue
		}
		seatNo := seatNoIdx + 1
		currentHandState.PlayersStack[uint64(seatNo)] = playerState[playerID].Balance
	}

	handStateMsg := &HandMessage{
		ClubId:      g.config.ClubId,
		GameId:      g.config.GameId,
		PlayerId:    message.GetPlayerId(),
		HandNum:     handState.HandNum,
		MessageType: HandQueryCurrentHand,
		HandMessage: &HandMessage_CurrentHandState{CurrentHandState: &currentHandState},
	}
	g.sendHandMessageToPlayer(handStateMsg, message.GetPlayerId())
	return nil
}

func (g *Game) onPlayerActed(message *HandMessage) error {

	channelGameLogger.Info().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
		Uint32("player", message.SeatNo).
		Str("message", message.MessageType).
		Msg(fmt.Sprintf("%v", message))

	if message.SeatNo == g.timerSeatNo {
		// pause play timer
		g.pausePlayTimer(message.SeatNo)
	}

	gameState, err := g.loadState()
	if err != nil {
		return err
	}

	// get hand state
	handState, err := g.loadHandState(gameState)
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

	err = g.saveHandState(gameState, handState)
	if err != nil {
		return err
	}

	// Send player's current stack to be updated in the UI
	seatNo := message.GetPlayerActed().GetSeatNo()
	playerID := handState.PlayersInSeats[seatNo-1]

	playerAction := handState.PlayersActed[seatNo-1]
	stack := handState.PlayersState[playerID].Balance
	if playerAction.State != PlayerActState_PLAYER_ACT_FOLDED {
		message.GetPlayerActed().Amount = playerAction.Amount
	} else {
		// the game folded this guy's hand
		message.GetPlayerActed().Action = ACTION_FOLD
		message.GetPlayerActed().Amount = 0
	}
	message.GetPlayerActed().Stack = stack
	// broadcast this message to all the players
	g.broadcastHandMessage(message)

	go func(g *Game) {
		time.Sleep(1 * time.Second)
		gameState, err := g.loadState()
		if err != nil {
			return
		}
		handState, err := g.loadHandState(gameState)
		if err != nil {
			return
		} // if only one player is remaining in the hand, we have a winner
		if handState.NoActiveSeats == 1 {
			g.sendWinnerBeforeShowdown(gameState, handState)
			// result of the hand is sent

			// wait for the animation to complete before we send the next hand
			// if it is not auto deal, we return from here
			//if !g.autoDeal {
			//	return nil
			//}
		} else {
			// if the current player is where the action ends, move to the next round
			g.moveToNextAct(gameState, handState)
		}
	}(g)

	return nil
}

func (g *Game) getPots(handState *HandState) ([]float32, []*SeatsInPots) {
	pots := make([]float32, 0)
	seatsInPots := make([]*SeatsInPots, 0)
	for _, pot := range handState.Pots {
		if pot.Pot == 0 {
			continue
		}
		pots = append(pots, pot.Pot)
		seatsInPots = append(seatsInPots, pot)
	}
	return pots, seatsInPots
}

func (g *Game) gotoFlop(gameState *GameState, handState *HandState) {
	channelGameLogger.Info().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
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
	g.saveHandState(gameState, handState)
	pots, seatsInPots := g.getPots(handState)

	cardsStr := poker.CardsToString(boardCards)
	flopMessage := &Flop{Board: boardCards, CardsStr: cardsStr, Pots: pots, SeatsPots: seatsInPots}
	handMessage := &HandMessage{ClubId: g.config.ClubId,
		GameId:      g.config.GameId,
		HandNum:     handState.HandNum,
		MessageType: HandFlop,
		HandStatus:  handState.CurrentState}
	handMessage.HandMessage = &HandMessage_Flop{Flop: flopMessage}
	g.broadcastHandMessage(handMessage)
	g.saveHandState(gameState, handState)
}

func (g *Game) gotoTurn(gameState *GameState, handState *HandState) {
	channelGameLogger.Info().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
		Msg(fmt.Sprintf("Moving to %s", HandStatus_name[int32(handState.CurrentState)]))

	// send turn card to the board
	/*
		deck := poker.NewDeckFromBytes(handState.Deck, int(handState.DeckIndex))
		deck.Draw(1)
		handState.DeckIndex++
		turn := uint32(deck.Draw(1)[0].GetByte())
	*/
	handState.setupTurn(handState.TurnCard)
	g.saveHandState(gameState, handState)

	cardsStr := poker.CardsToString(handState.BoardCards)
	boardCards := make([]uint32, len(handState.BoardCards))
	for i, card := range handState.BoardCards {
		boardCards[i] = uint32(card)
	}
	pots, seatsInPots := g.getPots(handState)
	turnMessage := &Turn{Board: boardCards, TurnCard: uint32(handState.TurnCard), CardsStr: cardsStr, Pots: pots, SeatsPots: seatsInPots}
	handMessage := &HandMessage{ClubId: g.config.ClubId,
		GameId:      g.config.GameId,
		HandNum:     handState.HandNum,
		MessageType: HandTurn,
		HandStatus:  handState.CurrentState}
	handMessage.HandMessage = &HandMessage_Turn{Turn: turnMessage}
	g.broadcastHandMessage(handMessage)
	g.saveHandState(gameState, handState)
}

func (g *Game) gotoRiver(gameState *GameState, handState *HandState) {
	channelGameLogger.Info().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
		Msg(fmt.Sprintf("Moving to %s", HandStatus_name[int32(handState.CurrentState)]))

	// send river card to the board
	/*
		deck := poker.NewDeckFromBytes(handState.Deck, int(handState.DeckIndex))
		deck.Draw(1)
		handState.DeckIndex++
		river := uint32(deck.Draw(1)[0].GetByte())
	*/

	handState.setupRiver(handState.RiverCard)
	g.saveHandState(gameState, handState)

	cardsStr := poker.CardsToString(handState.BoardCards)
	boardCards := make([]uint32, len(handState.BoardCards))
	for i, card := range handState.BoardCards {
		boardCards[i] = uint32(card)
	}
	pots, seatsInPots := g.getPots(handState)
	riverMessage := &River{Board: boardCards, RiverCard: uint32(handState.RiverCard), CardsStr: cardsStr, Pots: pots, SeatsPots: seatsInPots}
	handMessage := &HandMessage{ClubId: g.config.ClubId,
		GameId:      g.config.GameId,
		HandNum:     handState.HandNum,
		MessageType: HandRiver,
		HandStatus:  handState.CurrentState}
	handMessage.HandMessage = &HandMessage_River{River: riverMessage}
	g.broadcastHandMessage(handMessage)
	g.saveHandState(gameState, handState)
}

func (g *Game) sendWinnerBeforeShowdown(gameState *GameState, handState *HandState) error {
	// every one folded except one player, send the pot to the player
	status := handState.CurrentState
	handState.everyOneFoldedWinners()

	handState.CurrentState = HandStatus_HAND_CLOSED
	err := g.saveHandState(gameState, handState)
	if err != nil {
		return err
	}

	// now send the data to users
	handMessage := &HandMessage{
		ClubId:      g.config.ClubId,
		GameId:      g.config.GameId,
		HandNum:     handState.HandNum,
		MessageType: HandResultMessage,
		HandStatus:  status,
	}

	handResultProcessor := NewHandResultProcessor(handState, gameState, g.config.RewardTrackingIds)

	// send the hand to the database to store first
	handResult := handResultProcessor.getResult(true /*db*/)
	g.saveHandResult(handResult)

	// send to all the players
	handResult = handResultProcessor.getResult(false /*db*/)
	handMessage.HandMessage = &HandMessage_HandResult{HandResult: handResult}
	g.broadcastHandMessage(handMessage)
	go g.moveToNextHand()
	return nil
}

func (g *Game) moveToNextHand() {
	// wait 5 minutes to show the result
	// send a message to game to start new hand
	time.Sleep(5 * time.Second)
	gameMessage := &GameMessage{
		GameId:      g.config.GameId,
		MessageType: GameMoveToNextHand,
	}
	g.SendGameMessage(gameMessage)
}

func (g *Game) moveToNextRound(gameState *GameState, handState *HandState) {
	if handState.LastState == HandStatus_DEAL {
		return
	}

	// remove folded players from the pots
	handState.removeFoldedPlayersFromPots()

	if handState.LastState == HandStatus_PREFLOP && handState.CurrentState == HandStatus_FLOP {
		g.gotoFlop(gameState, handState)
	} else if handState.LastState == HandStatus_FLOP && handState.CurrentState == HandStatus_TURN {
		g.gotoTurn(gameState, handState)
	} else if handState.LastState == HandStatus_TURN && handState.CurrentState == HandStatus_RIVER {
		g.gotoRiver(gameState, handState)
	} else if handState.LastState == HandStatus_RIVER && handState.CurrentState == HandStatus_SHOW_DOWN {
		g.gotoShowdown(gameState, handState)
	}
}

func (g *Game) moveToNextAct(gameState *GameState, handState *HandState) {
	if handState.isAllActivePlayersAllIn() {
		g.handleNoMoreActions(gameState, handState)
	} else {

		if handState.LastState != handState.CurrentState {
			// move to next round
			g.moveToNextRound(gameState, handState)
		}

		if handState.NextSeatAction != nil {
			// tell the next player to act
			nextSeatMessage := &HandMessage{
				ClubId:      g.config.ClubId,
				GameId:      g.config.GameId,
				HandNum:     handState.HandNum,
				MessageType: HandPlayerAction,
				HandStatus:  handState.CurrentState,
				SeatNo:      handState.NextSeatAction.SeatNo,
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
			g.sendHandMessageToPlayer(nextSeatMessage, playerID)
			g.resetTimer(handState.NextSeatAction.SeatNo, playerID, canCheck)

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
				ClubId:      g.config.ClubId,
				GameId:      g.config.GameId,
				HandNum:     handState.HandNum,
				HandStatus:  handState.CurrentState,
				MessageType: HandNextAction,
			}
			message.HandMessage = &HandMessage_ActionChange{ActionChange: actionChange}
			g.broadcastHandMessage(message)
		}
	}
}

func (g *Game) handleNoMoreActions(gameState *GameState, handState *HandState) {

	_, seatsInPots := g.getPots(handState)

	// broadcast the players no more actions
	handMessage := &NoMoreActions{
		Pots: seatsInPots,
	}
	message := &HandMessage{
		ClubId:      g.config.ClubId,
		GameId:      g.config.GameId,
		HandNum:     handState.HandNum,
		HandStatus:  handState.CurrentState,
		MessageType: HandNoMoreActions,
	}
	message.HandMessage = &HandMessage_NoMoreActions{NoMoreActions: handMessage}
	g.broadcastHandMessage(message)
	for handState.CurrentState != HandStatus_SHOW_DOWN {
		switch handState.CurrentState {
		case HandStatus_FLOP:
			g.gotoFlop(gameState, handState)
			handState.CurrentState = HandStatus_TURN
		case HandStatus_TURN:
			g.gotoTurn(gameState, handState)
			handState.CurrentState = HandStatus_RIVER
		case HandStatus_RIVER:
			g.gotoRiver(gameState, handState)
			handState.CurrentState = HandStatus_SHOW_DOWN
		}
	}
	g.gotoShowdown(gameState, handState)
}

func (g *Game) gotoShowdown(gameState *GameState, handState *HandState) {
	handState.removeEmptyPots()

	handState.HandCompletedAt = HandStatus_SHOW_DOWN
	handResultProcessor := NewHandResultProcessor(handState, gameState, g.config.RewardTrackingIds)
	// send the hand to the database to store first
	handResult := handResultProcessor.getResult(true /*db*/)
	g.saveHandResult(handResult)

	// send to all the players
	handResult = handResultProcessor.getResult(false /*db*/)

	// update the player balance
	for _, player := range handResult.Players {
		gameState.PlayersState[player.Id].CurrentBalance = player.Balance.After
	}
	// save the game state
	g.saveState(gameState)

	// now send the data to users
	handMessage := &HandMessage{
		ClubId:      g.config.ClubId,
		GameId:      g.config.GameId,
		HandNum:     handState.HandNum,
		MessageType: HandResultMessage,
		HandStatus:  handState.CurrentState,
	}
	handMessage.HandMessage = &HandMessage_HandResult{HandResult: handResult}
	g.broadcastHandMessage(handMessage)
	go g.moveToNextHand()
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
