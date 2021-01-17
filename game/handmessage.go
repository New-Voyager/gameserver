package game

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
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
		err := g.onPlayerActed(message)
		if err != nil {
			channelGameLogger.Error().Msgf("Error while processing %s message. Error: %s", HandPlayerActed, err.Error())
		}
	case HandQueryCurrentHand:
		err := g.onQueryCurrentHand(message)
		if err != nil {
			channelGameLogger.Error().Msgf("Error while processing %s message. Error: %s", HandQueryCurrentHand, err.Error())
		}
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
	bettingInProgress := handState.CurrentState == HandStatus_PREFLOP || handState.CurrentState == HandStatus_FLOP || handState.CurrentState == HandStatus_TURN || handState.CurrentState == HandStatus_RIVER
	if bettingInProgress {
		currentRoundState, ok := handState.RoundState[uint32(handState.CurrentState)]
		if !ok {
			b, err := json.Marshal(handState)
			if err != nil {
				if handState != nil {
					channelGameLogger.Error().Msgf("Unable to find current round state. handState.CurrentState: %d handState.RoundState: %+v", handState.CurrentState, handState.RoundState)
				} else {
					channelGameLogger.Error().Msg(err.Error())
				}
			} else {
				channelGameLogger.Error().Msgf("Unable to find current round state. handState: %s", string(b))
			}
		}
		currentBettingRound := currentRoundState.Betting
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
			playerSeatNo = uint32(seatNo)
			break
		}
	}

	for seatNo, action := range handState.GetPlayersActed() {
		if action.State == PlayerActState_PLAYER_ACT_EMPTY_SEAT {
			continue
		}
		currentHandState.PlayersActed[uint32(seatNo)] = action
	}

	if playerSeatNo != 0 {
		_, maskedCards := g.maskCards(handState.GetPlayersCards()[playerSeatNo],
			gameState.PlayersState[message.PlayerId].GameTokenInt)
		currentHandState.PlayerCards = fmt.Sprintf("%d", maskedCards)
		currentHandState.PlayerSeatNo = playerSeatNo
	}

	if bettingInProgress && handState.NextSeatAction != nil {
		currentHandState.NextSeatToAct = handState.NextSeatAction.SeatNo
		currentHandState.RemainingActionTime = g.remainingActionTime
		currentHandState.NextSeatAction = handState.NextSeatAction
	}
	currentHandState.PlayersStack = make(map[uint64]float32, 0)
	playerState := handState.GetPlayersState()
	for seatNo, playerID := range handState.GetPlayersInSeats() {
		if playerID == 0 {
			continue
		}
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

	// if the hand number does not match, ignore the message
	if message.HandNum != handState.HandNum {
		channelGameLogger.Error().
			Uint32("club", g.config.ClubId).
			Str("game", g.config.GameCode).
			Uint32("player", message.SeatNo).
			Str("message", message.MessageType).
			Msg(fmt.Sprintf("Invalid hand number: %d current hand number: %d", message.HandNum, handState.HandNum))
		return fmt.Errorf("Invalid hand number: %d current hand number: %d", message.HandNum, handState.HandNum)
	}

	if handState.NextSeatAction == nil {
		channelGameLogger.Error().
			Uint32("club", g.config.ClubId).
			Str("game", g.config.GameCode).
			Uint32("player", message.SeatNo).
			Str("message", message.MessageType).
			Msg(fmt.Sprintf("Invalid action. There is no next action"))
		return fmt.Errorf("Invalid action. There is no next action")
	}

	if handState.CurrentState == HandStatus_SHOW_DOWN {
		channelGameLogger.Error().
			Uint32("club", g.config.ClubId).
			Str("game", g.config.GameCode).
			Uint32("player", message.SeatNo).
			Str("message", message.MessageType).
			Msg(fmt.Sprintf("Invalid action. There is no next action"))
		return fmt.Errorf("Invalid action. There is no next action")
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

	ackMsg := &HandMessage{
		ClubId:      g.config.ClubId,
		GameId:      g.config.GameId,
		PlayerId:    message.GetPlayerId(),
		HandNum:     handState.HandNum,
		MessageType: HandMsgAck,
		HandStatus:  handState.CurrentState,
		SeatNo:      message.GetPlayerActed().GetSeatNo(),
		HandMessage: &HandMessage_MsgAck{
			MsgAck: &MsgAcknowledgement{
				MessageId:   message.GetMessageId(),
				MessageType: message.GetMessageType(),
			},
		},
	}
	g.sendHandMessageToPlayer(ackMsg, message.GetPlayerId())

	// Send player's current stack to be updated in the UI
	seatNo := message.GetPlayerActed().GetSeatNo()
	var stack float32
	if bettingState, ok := handState.RoundState[uint32(handState.CurrentState)]; ok {
		stack = bettingState.PlayerBalance[seatNo]
	}

	if stack == 0 {
		// get it from playerState
		playerID := handState.PlayersInSeats[seatNo]
		if playerID != 0 {
			stack = handState.PlayersState[playerID].Balance
		}
	}

	playerAction := handState.PlayersActed[seatNo]
	if playerAction.State != PlayerActState_PLAYER_ACT_FOLDED {
		message.GetPlayerActed().Amount = playerAction.Amount
	} else {
		// the game folded this guy's hand
		message.GetPlayerActed().Action = ACTION_FOLD
		message.GetPlayerActed().Amount = 0
	}
	message.HandNum = handState.HandNum
	message.GetPlayerActed().Stack = stack
	// broadcast this message to all the players
	g.broadcastHandMessage(message)

	go func(g *Game) {
		if !RunningTests {
			time.Sleep(time.Duration(g.delays.PlayerActed) * time.Millisecond)
		}
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
	balance := make(map[uint32]float32, 0)
	for seatNo, playerID := range handState.PlayersInSeats {
		if seatNo == 0 {
			continue
		}
		if playerState, ok := handState.PlayersState[playerID]; ok {
			balance[uint32(seatNo)] = playerState.Balance
		}
	}

	cardsStr := poker.CardsToString(boardCards)
	flopMessage := &Flop{Board: boardCards, CardsStr: cardsStr, Pots: pots, SeatsPots: seatsInPots, PlayerBalance: balance}
	handMessage := &HandMessage{ClubId: g.config.ClubId,
		GameId:      g.config.GameId,
		HandNum:     handState.HandNum,
		MessageType: HandFlop,
		HandStatus:  handState.CurrentState}
	handMessage.HandMessage = &HandMessage_Flop{Flop: flopMessage}
	g.broadcastHandMessage(handMessage)
	g.saveHandState(gameState, handState)
	if !RunningTests {
		time.Sleep(time.Duration(g.delays.GoToFlop) * time.Millisecond)
	}
}

func (g *Game) gotoTurn(gameState *GameState, handState *HandState) {
	channelGameLogger.Info().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
		Msg(fmt.Sprintf("Moving to %s", HandStatus_name[int32(handState.CurrentState)]))

	handState.setupTurn(handState.TurnCard)
	g.saveHandState(gameState, handState)

	cardsStr := poker.CardsToString(handState.BoardCards)
	boardCards := make([]uint32, len(handState.BoardCards))
	for i, card := range handState.BoardCards {
		boardCards[i] = uint32(card)
	}
	pots, seatsInPots := g.getPots(handState)

	balance := make(map[uint32]float32, 0)
	for seatNo, playerID := range handState.PlayersInSeats {
		if seatNo == 0 {
			continue
		}
		if playerState, ok := handState.PlayersState[playerID]; ok {
			balance[uint32(seatNo)] = playerState.Balance
		}
	}
	turnMessage := &Turn{Board: boardCards, TurnCard: uint32(handState.TurnCard),
		CardsStr: cardsStr, Pots: pots, SeatsPots: seatsInPots, PlayerBalance: balance}
	handMessage := &HandMessage{ClubId: g.config.ClubId,
		GameId:      g.config.GameId,
		HandNum:     handState.HandNum,
		MessageType: HandTurn,
		HandStatus:  handState.CurrentState}
	handMessage.HandMessage = &HandMessage_Turn{Turn: turnMessage}
	g.broadcastHandMessage(handMessage)
	g.saveHandState(gameState, handState)
	if !RunningTests {
		time.Sleep(time.Duration(g.delays.GoToTurn) * time.Millisecond)
	}
}

func (g *Game) gotoRiver(gameState *GameState, handState *HandState) {
	channelGameLogger.Info().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
		Msg(fmt.Sprintf("Moving to %s", HandStatus_name[int32(handState.CurrentState)]))

	handState.setupRiver(handState.RiverCard)
	g.saveHandState(gameState, handState)

	cardsStr := poker.CardsToString(handState.BoardCards)
	boardCards := make([]uint32, len(handState.BoardCards))
	for i, card := range handState.BoardCards {
		boardCards[i] = uint32(card)
	}
	pots, seatsInPots := g.getPots(handState)

	balance := make(map[uint32]float32, 0)
	for seatNo, playerID := range handState.PlayersInSeats {
		if seatNo == 0 {
			continue
		}
		if playerState, ok := handState.PlayersState[playerID]; ok {
			balance[uint32(seatNo)] = playerState.Balance
		}
	}
	riverMessage := &River{Board: boardCards, RiverCard: uint32(handState.RiverCard),
		CardsStr: cardsStr, Pots: pots, SeatsPots: seatsInPots, PlayerBalance: balance}
	handMessage := &HandMessage{ClubId: g.config.ClubId,
		GameId:      g.config.GameId,
		HandNum:     handState.HandNum,
		MessageType: HandRiver,
		HandStatus:  handState.CurrentState}
	handMessage.HandMessage = &HandMessage_River{River: riverMessage}
	g.broadcastHandMessage(handMessage)
	g.saveHandState(gameState, handState)
	if !RunningTests {
		time.Sleep(time.Duration(g.delays.GoToRiver) * time.Millisecond)
	}
}

func (g *Game) sendWinnerBeforeShowdown(gameState *GameState, handState *HandState) error {
	// every one folded except one player, send the pot to the player
	handState.everyOneFoldedWinners()

	handState.CurrentState = HandStatus_HAND_CLOSED
	err := g.saveHandState(gameState, handState)
	if err != nil {
		return err
	}

	handResultProcessor := NewHandResultProcessor(handState, gameState, g.config.RewardTrackingIds)

	// send the hand to the database to store first
	handResult := handResultProcessor.getResult(true /*db*/)
	saveResult, err := g.saveHandResult(handResult)

	// send to all the players
	handResult = handResultProcessor.getResult(false /*db*/)
	g.sendResult(handState, saveResult, handResult)

	go g.moveToNextHand()
	return nil
}

func (g *Game) moveToNextHand() {
	// wait 5 seconds to show the result
	// send a message to game to start new hand
	if !RunningTests {
		time.Sleep(time.Duration(g.delays.MoveToNextHand) * time.Millisecond)
	}
	gameMessage := &GameMessage{
		GameId:      g.config.GameId,
		MessageType: GameMoveToNextHand,
	}
	g.SendGameMessageToChannel(gameMessage)
}

func (g *Game) sendResult(handState *HandState, saveResult *SaveHandResult, handResult *HandResult) {

	// now send the data to users
	handMessage := &HandMessage{
		ClubId:      g.config.ClubId,
		GameId:      g.config.GameId,
		HandNum:     handState.HandNum,
		MessageType: HandResultMessage,
		HandStatus:  handState.CurrentState,
	}

	if saveResult != nil {
		if saveResult.HighHand != nil {
			// a player in this game hit a high hand
			handResult.HighHand = &HighHand{}
			handResult.HighHand.GameCode = saveResult.GameCode
			handResult.HighHand.HandNum = uint32(saveResult.HandNum)
			handResult.HighHand.Winners = make([]*HighHandWinner, 0)

			for _, winner := range saveResult.HighHand.Winners {
				playerSeatNo := 0

				winningPlayer, _ := strconv.ParseInt(winner.PlayerID, 10, 64)
				// get seat no
				for seatNo, playerID := range handState.ActiveSeats {
					if int64(playerID) == winningPlayer {
						playerSeatNo = seatNo
						break
					}
				}
				playerCards := make([]uint32, len(winner.PlayerCards))
				for i, card := range winner.PlayerCards {
					playerCards[i] = uint32(card)
				}
				hhCards := make([]uint32, len(winner.HhCards))
				for i, card := range winner.HhCards {
					hhCards[i] = uint32(card)
				}

				handResult.HighHand.Winners = append(handResult.HighHand.Winners, &HighHandWinner{
					PlayerId:    uint64(winningPlayer),
					PlayerName:  winner.PlayerName,
					PlayerCards: playerCards,
					HhCards:     hhCards,
					SeatNo:      uint32(playerSeatNo),
				})
			}

			if len(saveResult.HighHand.AssociatedGames) >= 1 {
				// announce the high hand to other games
				go g.announceHighHand(saveResult, handResult.HighHand)
			}
		}
	}
	handMessage.HandMessage = &HandMessage_HandResult{HandResult: handResult}
	g.broadcastHandMessage(handMessage)
}

func (g *Game) announceHighHand(saveResult *SaveHandResult, highHand *HighHand) {

	for _, gameCode := range saveResult.HighHand.AssociatedGames {
		gameMessage := &GameMessage{
			GameCode:    gameCode,
			MessageType: HighHandMsg,
		}
		gameMessage.GameMessage = &GameMessage_HighHand{
			HighHand: highHand,
		}
		g.broadcastGameMessage(gameMessage)
	}

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
			playerID := handState.PlayersInSeats[handState.NextSeatAction.SeatNo]
			g.sendHandMessageToPlayer(nextSeatMessage, playerID)
			g.resetTimer(handState.NextSeatAction.SeatNo, playerID, canCheck)

			pots := make([]float32, 0)
			for _, pot := range handState.Pots {
				pots = append(pots, pot.Pot)
			}
			currentPot := pots[len(pots)-1]
			roundState := handState.RoundState[uint32(handState.CurrentState)]
			currentBettingRound := roundState.Betting
			seatBets := currentBettingRound.SeatBet
			bettingRoundBets := float32(0)
			for _, bet := range seatBets {
				bettingRoundBets = bettingRoundBets + bet
			}

			if bettingRoundBets != 0 {
				currentPot = currentPot + bettingRoundBets
			} else {
				currentPot = 0
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
	saveResult, _ := g.saveHandResult(handResult)

	// send to all the players
	handResult = handResultProcessor.getResult(false /*db*/)

	// update the player balance
	for _, player := range handResult.Players {
		gameState.PlayersState[player.Id].CurrentBalance = player.Balance.After
	}
	// save the game state
	g.saveState(gameState)
	g.sendResult(handState, saveResult, handResult)
	go g.moveToNextHand()
}

func (g *Game) saveHandResult(result *HandResult) (*SaveHandResult, error) {
	// call the API server to save the hand result
	var m protojson.MarshalOptions
	m.EmitUnpopulated = true
	data, _ := m.Marshal(result)
	fmt.Printf("%s\n", string(data))

	url := fmt.Sprintf("%s/internal/post-hand/gameId/%d/handNum/%d", g.apiServerUrl, result.GameId, result.HandNum)
	resp, _ := http.Post(url, "application/json", bytes.NewBuffer(data))
	// if the api server returns nil, do nothing
	if resp == nil {
		return nil, fmt.Errorf("Saving hand failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		bodyBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			channelGameLogger.Error().Msgf("Failed to read save result for hand num: %d", result.HandNum)
		}
		bodyString := string(bodyBytes)
		fmt.Printf(bodyString)
		fmt.Printf("\n")
		fmt.Printf("Posted successfully")

		var saveResult SaveHandResult
		json.Unmarshal(bodyBytes, &saveResult)
		return &saveResult, nil
	} else {
		return nil, fmt.Errorf("faile to save hand")
	}
}
