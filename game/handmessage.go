package game

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/pkg/errors"
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
		gameState, err := g.loadState()
		if err != nil {
			channelGameLogger.Error().Msgf("Unable to load game state. Error: %s", err.Error())
			break
		}
		handState, err := g.loadHandState(gameState)
		if err != nil {
			channelGameLogger.Error().Msgf("Unable to load hand state. Error: %s", err.Error())
			break
		}

		err = g.onPlayerActed(message, gameState, handState)
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
		return errors.Wrap(err, "Unable to load game state")
	}

	// get hand state
	handState, err := g.loadHandState(gameState)
	if err != nil {
		return errors.Wrap(err, "Unable to load hand state")
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
		if !ok || currentRoundState == nil {
			b, err := json.Marshal(handState)
			if err != nil {
				return fmt.Errorf("Unable to find current round state. currentRoundState: %+v. handState.CurrentState: %d handState.RoundState: %+v", currentRoundState, handState.CurrentState, handState.RoundState)
			}
			return fmt.Errorf("Unable to find current round state. handState: %s", string(b))
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

func (g *Game) onPlayerActed(message *HandMessage, gameState *GameState, handState *HandState) error {

	// If we got here, gameState should be in WAIT_FOR_ACTION stage.

	messageSeatNo := message.GetPlayerActed().GetSeatNo()
	channelGameLogger.Info().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
		Uint32("player", messageSeatNo).
		Str("message", message.MessageType).
		Msg(fmt.Sprintf("%v", message))

	if messageSeatNo == 0 && !RunningTests {
		errMsg := fmt.Sprintf("Invalid seat number [%d] for player ID %d. Ignoring the action message.", messageSeatNo, message.PlayerId)
		channelGameLogger.Error().
			Uint32("club", g.config.ClubId).
			Str("game", g.config.GameCode).
			Msgf(errMsg)
		return fmt.Errorf(errMsg)
	}

	if !message.GetPlayerActed().GetTimedOut() {
		if message.MessageId == 0 && !RunningTests {
			errMsg := fmt.Sprintf("Invalid message ID [0] for player ID %d Seat %d. Ignoring the action message.", message.PlayerId, messageSeatNo)
			channelGameLogger.Error().
				Uint32("club", g.config.ClubId).
				Str("game", g.config.GameCode).
				Msgf(errMsg)
			return fmt.Errorf(errMsg)
		}
	}

	// if the hand number does not match, ignore the message
	if message.HandNum != handState.HandNum {
		errMsg := fmt.Sprintf("Invalid hand number: %d current hand number: %d", message.HandNum, handState.HandNum)
		channelGameLogger.Error().
			Uint32("club", g.config.ClubId).
			Str("game", g.config.GameCode).
			Uint32("player", messageSeatNo).
			Str("message", message.MessageType).
			Msg(errMsg)

		// This can happen if the action was already processed, but the client is retrying
		// because the acnowledgement got lost in the network. Just acknowledge so that
		// the client stops retrying.
		g.acknowledgeMsg(message)
		return fmt.Errorf(errMsg)
	}

	if handState.NextSeatAction == nil {
		errMsg := "Invalid action. There is no next action"
		channelGameLogger.Error().
			Uint32("club", g.config.ClubId).
			Str("game", g.config.GameCode).
			Uint32("player", messageSeatNo).
			Str("message", message.MessageType).
			Msg(errMsg)

		// This can happen if the action was already processed, but the client is retrying
		// because the acnowledgement got lost in the network. Just acknowledge so that
		// the client stops retrying.
		g.acknowledgeMsg(message)
		return fmt.Errorf(errMsg)
	}

	if handState.CurrentState == HandStatus_SHOW_DOWN {
		errMsg := "Invalid action. Hand is in show-down state"
		channelGameLogger.Error().
			Uint32("club", g.config.ClubId).
			Str("game", g.config.GameCode).
			Uint32("player", messageSeatNo).
			Str("message", message.MessageType).
			Msg(errMsg)

		// This can happen if the action was already processed, but the client is retrying
		// because the acnowledgement got lost in the network. Just acknowledge so that
		// the client stops retrying.
		g.acknowledgeMsg(message)
		return fmt.Errorf(errMsg)
	}

	if messageSeatNo == g.timerSeatNo {
		// cancel action timer
		g.pausePlayTimer(messageSeatNo)
	}

	handState.ActionMsgInProgress = message
	g.acknowledgeMsg(message)

	gameState.Stage = GameStage__PREPARE_NEXT_ACTION
	g.saveState(gameState)
	g.saveHandState(gameState, handState)
	g.prepareNextAction(gameState, handState)
	return nil
}

func (g *Game) prepareNextAction(gameState *GameState, handState *HandState) error {
	// If we got here, gameState should be in PREPARE_NEXT_ACTION stage.

	message := handState.ActionMsgInProgress
	if message == nil {
		errMsg := "Unable to get action message in progress. handState.ActionMsgInProgress is nil"
		channelGameLogger.Error().
			Uint32("club", g.config.ClubId).
			Str("game", g.config.GameCode).
			Msg(errMsg)
		return fmt.Errorf(errMsg)
	}

	var err error
	err = handState.actionReceived(message.GetPlayerActed())
	if err != nil {
		return errors.Wrap(err, "Error while updating handstate from action")
	}

	if err != nil {
		// This is retryable (redis connection temporarily down?). Don't acknowledge and force the client to resend.
		return err
	}

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
	message.MessageId = 0
	g.broadcastHandMessage(message)

	if !RunningTests {
		time.Sleep(time.Duration(g.delays.PlayerActed) * time.Millisecond)
	}

	g.saveHandState(gameState, handState)

	if handState.NoActiveSeats == 1 {
		gameState.Stage = GameStage__RESULT
		g.saveState(gameState)
		g.allButOneFolded(gameState, handState)
	} else if handState.isAllActivePlayersAllIn() {
		gameState.Stage = GameStage__RESULT
		g.saveState(gameState)
		g.handleNoMoreActions(gameState, handState)
	} else if handState.LastState != handState.CurrentState {
		gameState.Stage = GameStage__NEXT_ROUND
		g.saveState(gameState)
		g.moveToNextRound(gameState, handState)
	} else {
		gameState.Stage = GameStage__MOVE_TO_NEXT_ACTION
		g.saveState(gameState)
		g.moveToNextAction(gameState, handState)
	}

	return nil
}

func (g *Game) acknowledgeMsg(message *HandMessage) {
	if message.GetPlayerActed().GetTimedOut() {
		// Default action is generated by the server on action timeout. Don't acknowledge that.
		return
	}

	ack := &HandMessage{
		ClubId:      message.GetClubId(),
		GameId:      message.GetGameId(),
		PlayerId:    message.GetPlayerId(),
		HandNum:     message.GetHandNum(),
		MessageType: HandMsgAck,
		HandStatus:  message.GetHandStatus(),
		SeatNo:      message.GetPlayerActed().GetSeatNo(),
		HandMessage: &HandMessage_MsgAck{
			MsgAck: &MsgAcknowledgement{
				MessageId:   message.GetMessageId(),
				MessageType: message.GetMessageType(),
			},
		},
	}
	g.sendHandMessageToPlayer(ack, message.GetPlayerId())
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
	if !RunningTests {
		time.Sleep(time.Duration(g.delays.GoToRiver) * time.Millisecond)
	}
}

func (g *Game) handEnded(handNum uint32) {
	// wait 5 seconds to show the result
	// send a message to game to start new hand
	if !RunningTests {
		time.Sleep(time.Duration(g.delays.MoveToNextHand) * time.Millisecond)
	}

	// broadcast hand ended
	handMessage := &HandMessage{
		ClubId:      g.config.ClubId,
		GameId:      g.config.GameId,
		HandNum:     handNum,
		MessageType: HandEnded,
	}
	g.broadcastHandMessage(handMessage)

	gameMessage := &GameMessage{
		GameId:      g.config.GameId,
		MessageType: GameMoveToNextHand,
	}
	go g.SendGameMessageToChannel(gameMessage)
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
				g.announceHighHand(saveResult, handResult.HighHand)
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
	// If we got here, gameState should be in NEXT_ROUND stage.

	if handState.LastState == HandStatus_DEAL {
		// How do we get here?
		return
	}

	// remove folded players from the pots
	handState.removeFoldedPlayersFromPots()

	moreRounds := true
	if handState.LastState == HandStatus_PREFLOP && handState.CurrentState == HandStatus_FLOP {
		g.gotoFlop(gameState, handState)
	} else if handState.LastState == HandStatus_FLOP && handState.CurrentState == HandStatus_TURN {
		g.gotoTurn(gameState, handState)
	} else if handState.LastState == HandStatus_TURN && handState.CurrentState == HandStatus_RIVER {
		g.gotoRiver(gameState, handState)
	} else if handState.LastState == HandStatus_RIVER && handState.CurrentState == HandStatus_SHOW_DOWN {
		moreRounds = false
		g.gotoShowdown(gameState, handState)
	}

	if moreRounds {
		gameState.Stage = GameStage__MOVE_TO_NEXT_ACTION
		g.saveState(gameState)
		g.saveHandState(gameState, handState)
		g.moveToNextAction(gameState, handState)
	} else {
		gameState.Stage = GameStage__HAND_END
		g.saveState(gameState)
		g.saveHandState(gameState, handState)
		g.handEnded(handState.HandNum)
	}
}

func (g *Game) moveToNextAction(gameState *GameState, handState *HandState) error {
	// If we got here, gameState should be in MOVE_TO_NEXT_ACTION stage.

	if handState.NextSeatAction == nil {
		return fmt.Errorf("moveToNextAct called when handState.NextSeatAction == nil")
	}

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

	gameState.Stage = GameStage__WAIT_FOR_NEXT_ACTION
	g.saveHandState(gameState, handState)
	g.saveState(gameState)

	return nil
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

	gameState.Stage = GameStage__HAND_END
	g.saveState(gameState)
	g.saveHandState(gameState, handState)
	g.handEnded(handState.HandNum)
}

func (g *Game) gotoShowdown(gameState *GameState, handState *HandState) error {
	handState.removeEmptyPots()
	handState.HandCompletedAt = HandStatus_SHOW_DOWN
	g.generateAndSendResult(gameState, handState)
	return nil
}

func (g *Game) allButOneFolded(gameState *GameState, handState *HandState) error {
	// every one folded except one player, send the pot to the player
	handState.everyOneFoldedWinners()
	handState.CurrentState = HandStatus_HAND_CLOSED
	g.generateAndSendResult(gameState, handState)

	gameState.Stage = GameStage__HAND_END
	g.saveState(gameState)
	g.saveHandState(gameState, handState)
	g.handEnded(handState.HandNum)
	return nil
}

func (g *Game) generateAndSendResult(gameState *GameState, handState *HandState) error {
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

	g.sendResult(handState, saveResult, handResult)

	return nil
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
