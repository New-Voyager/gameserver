package game

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/encoding/protojson"
	"voyager.com/logging"
	"voyager.com/server/crashtest"
	"voyager.com/server/poker"
	"voyager.com/server/util"
)

const MIN_FULLHOUSE_RANK = 322

func (g *Game) handleHandMessage(message *HandMessage) error {
	err := g.validateClientMsg(message)
	if err != nil {
		msg := "Client message validation failed"
		g.logger.Error().
			Err(err).
			Uint64(logging.PlayerIDKey, message.PlayerId).
			Uint32(logging.SeatNumKey, message.SeatNo).
			Msgf(msg)
		return nil
	}

	msgItem, err := g.getClientMsgItem(message)
	if err != nil {
		return err
	}

	switch msgItem.MessageType {
	case HandPlayerActed:
		handState, err := g.loadHandState()
		if err != nil {
			errMsg := "Could not load hand state before processing player action message"
			return errors.Wrap(err, errMsg)
		}
		if handState == nil {
			return fmt.Errorf("Cannot process player action. handState == nil")
		}
		err = g.onPlayerActed(message, handState)
		if err != nil {
			switch err.(type) {
			case InvalidMessageError:
				// No need to raise this error up further.
			default:
				errMsg := "Could not process player action"
				g.logger.Error().
					Err(err).
					Str(logging.MsgTypeKey, msgItem.MessageType).
					Uint64(logging.PlayerIDKey, message.PlayerId).
					Uint32(logging.HandNumKey, handState.GetHandNum()).
					Msgf(errMsg)
				return errors.Wrap(err, errMsg)
			}
		}
	case HandQueryCurrentHand:
		err := g.onQueryCurrentHand(message)
		if err != nil {
			errMsg := "Could not process hand message"
			g.logger.Error().
				Err(err).
				Str(logging.MsgTypeKey, msgItem.MessageType).
				Uint64(logging.PlayerIDKey, message.PlayerId).
				Msgf(errMsg)
		}
	case HandExtendTimer:
		err := g.onExtendTimer(message)
		if err != nil {
			errMsg := "Could not process hand message"
			g.logger.Error().
				Err(err).
				Str(logging.MsgTypeKey, msgItem.MessageType).
				Uint64(logging.PlayerIDKey, message.PlayerId).
				Msgf(errMsg)
		}
	case HandResetTimer:
		err := g.onResetCurrentTimer(message)
		if err != nil {
			errMsg := "Could not process hand message"
			g.logger.Error().
				Err(err).
				Str(logging.MsgTypeKey, msgItem.MessageType).
				Uint64(logging.PlayerIDKey, message.PlayerId).
				Msgf(errMsg)
		}
	}

	return nil
}

func (g *Game) validateClientMsg(message *HandMessage) error {
	// Messages from the client should only contain one item.
	msgItems := message.GetMessages()
	if len(msgItems) != 1 {
		return InvalidMessageError{
			Msg: fmt.Sprintf("Hand message from the client should only contain one item, but contains %d items", len(msgItems)),
		}
	}
	return nil
}

func (g *Game) getClientMsgItem(message *HandMessage) (*HandMessageItem, error) {
	msgItems := message.GetMessages()
	// Messages from the client should only contain one item.
	if len(msgItems) != 1 {
		return nil, InvalidMessageError{Msg: fmt.Sprintf("Client msg contains %d msg items", len(msgItems))}
	}
	return msgItems[0], nil
}

func (g *Game) onExtendTimer(playerMsg *HandMessage) error {
	playerID := playerMsg.GetPlayerId()
	if playerID == 0 {
		return fmt.Errorf("Player ID is 0")
	}
	msgItem, err := g.getClientMsgItem(playerMsg)
	if err != nil {
		return err
	}
	extendTimer := msgItem.GetExtendTimer()
	seatNo := extendTimer.GetSeatNo()
	if seatNo == 0 {
		return fmt.Errorf("Seat Number is 0")
	}
	extendBySec := extendTimer.GetExtendBySec()
	if extendBySec > 999 {
		return fmt.Errorf("Too large value (%d) for extendBySec", extendBySec)
	}
	extendBy := time.Duration(extendBySec) * time.Second
	remainingSec, err := g.extendTimer(seatNo, playerID, extendBy)
	if err != nil {
		return err
	}

	// Broadcast this message back so that other players know this player's time got extended.
	extendTimer.RemainingSec = remainingSec
	g.broadcastHandMessage(playerMsg)
	return nil
}

func (g *Game) onResetCurrentTimer(playerMsg *HandMessage) error {
	playerID := playerMsg.GetPlayerId()
	if playerID == 0 {
		return fmt.Errorf("Player ID is 0")
	}
	msgItem, err := g.getClientMsgItem(playerMsg)
	if err != nil {
		return err
	}
	resetTimer := msgItem.GetResetTimer()
	seatNo := resetTimer.GetSeatNo()
	if seatNo == 0 {
		return fmt.Errorf("Seat Number is 0")
	}
	newRemainingSec := resetTimer.GetRemainingSec()
	newRemainingTime := time.Duration(newRemainingSec) * time.Second
	err = g.resetTime(seatNo, playerID, newRemainingTime)
	if err != nil {
		return err
	}
	return nil
}

func (g *Game) onQueryCurrentHand(playerMsg *HandMessage) error {
	// get hand state
	handState, err := g.loadHandState()
	if err != nil && err != RedisKeyNotFound {
		return errors.Wrap(err, "Unable to load hand state")
	}

	if handState == nil || handState.HandNum == 0 || handState.CurrentState == HandStatus_HAND_CLOSED {
		currentHandState := CurrentHandState{
			HandNum: 0,
		}
		handStateMsg := &HandMessageItem{
			MessageType: HandQueryCurrentHand,
			Content:     &HandMessageItem_CurrentHandState{CurrentHandState: &currentHandState},
		}

		serverMsg := &HandMessage{
			PlayerId:  playerMsg.GetPlayerId(),
			HandNum:   0,
			MessageId: g.generateMsgID("CURRENT_HAND", handState.HandNum, handState.CurrentState, playerMsg.PlayerId, playerMsg.MessageId, handState.CurrentActionNum),
			Messages:  []*HandMessageItem{handStateMsg},
		}

		g.sendHandMessageToPlayer(serverMsg, playerMsg.GetPlayerId())
		return nil
	}

	boardCards := make([]uint32, len(handState.BoardCards))
	for i, card := range handState.BoardCards {
		boardCards[i] = uint32(card)
	}

	pots := make([]float64, 0)
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

	var boardCardsOut []uint32
	switch handState.CurrentState {
	case HandStatus_FLOP:
		boardCardsOut = boardCards[:3]
	case HandStatus_TURN:
		boardCardsOut = boardCards[:4]

	case HandStatus_RIVER:
	case HandStatus_RESULT:
	case HandStatus_SHOW_DOWN:
		boardCardsOut = boardCards

	default:
		boardCardsOut = make([]uint32, 0)
	}
	cardsStr := poker.CardsToString(boardCardsOut)

	currentHandState := CurrentHandState{
		HandNum:       handState.HandNum,
		GameType:      handState.GameType,
		CurrentRound:  handState.CurrentState,
		BoardCards:    boardCardsOut,
		BoardCards_2:  nil,
		CardsStr:      cardsStr,
		Pots:          pots,
		PotUpdates:    currentPot,
		ButtonPos:     handState.ButtonPos,
		SmallBlindPos: handState.SmallBlindPos,
		BigBlindPos:   handState.BigBlindPos,
		SmallBlind:    handState.SmallBlind,
		BigBlind:      handState.BigBlind,
		NoCards:       g.NumCards(handState.GameType),
	}
	currentHandState.PlayersActed = make(map[uint32]*PlayerActRound)

	var playerSeatNo uint32
	for seatNo, player := range handState.PlayersInSeats {
		if player.PlayerId == playerMsg.PlayerId {
			playerSeatNo = uint32(seatNo)
			break
		}
	}

	for seatNo, action := range handState.GetPlayersActed() {
		if action.Action == ACTION_EMPTY_SEAT {
			continue
		}
		currentHandState.PlayersActed[uint32(seatNo)] = action
	}

	if playerSeatNo != 0 {
		player := g.PlayersInSeats[playerSeatNo]
		_, maskedCards := g.MaskCards(handState.GetPlayersCards()[playerSeatNo], player.GameTokenInt)
		currentHandState.PlayerCards = fmt.Sprintf("%d", maskedCards)
		currentHandState.PlayerSeatNo = playerSeatNo
	}

	if bettingInProgress && handState.NextSeatAction != nil {
		remainingActionTime := g.GetRemainingActionTime()
		if playerSeatNo != handState.NextSeatAction.SeatNo {
			if remainingActionTime > g.timerCushionSec {
				remainingActionTime = remainingActionTime - g.timerCushionSec
			} else {
				remainingActionTime = 0
			}
		}
		currentHandState.NextSeatToAct = handState.NextSeatAction.SeatNo
		currentHandState.RemainingActionTime = remainingActionTime
		currentHandState.NextSeatAction = handState.NextSeatAction
	}
	currentHandState.PlayersStack = make(map[uint64]float64)
	for seatNo, player := range handState.PlayersInSeats {
		if seatNo == 0 || player.OpenSeat {
			continue
		}
		currentHandState.PlayersStack[uint64(seatNo)] = player.Stack
	}

	handStateMsg := &HandMessageItem{
		MessageType: HandQueryCurrentHand,
		Content:     &HandMessageItem_CurrentHandState{CurrentHandState: &currentHandState},
	}

	serverMsg := &HandMessage{
		PlayerId:   playerMsg.GetPlayerId(),
		HandNum:    handState.HandNum,
		HandStatus: handState.CurrentState,
		MessageId:  g.generateMsgID("CURRENT_HAND", handState.HandNum, handState.CurrentState, playerMsg.PlayerId, playerMsg.MessageId, handState.CurrentActionNum),
		Messages:   []*HandMessageItem{handStateMsg},
	}
	g.sendHandMessageToPlayer(serverMsg, playerMsg.GetPlayerId())
	return nil
}

func (g *Game) onPlayerActed(playerMsg *HandMessage, handState *HandState) error {
	if playerMsg == nil {
		// Game server is replaying this code after a crash.
		if handState.ActionMsgInProgress == nil {
			// There is no saved message. We crashed before saving the
			// message. We rely on the client to retry the message in this case.

			nextSeatAction := handState.NextSeatAction
			if nextSeatAction == nil {
				// Shouldn't get here.
				g.logger.Error().Msg("ActionMsgInProgress and NextSeatAction are both nil")
				return nil
			}

			now := time.Now()
			// Give some time for the client to retry before timing it out.
			retryWindowSec := 10
			actionExpiresAt := time.Unix(nextSeatAction.ActionTimesoutAt, 0)
			if actionExpiresAt.Before(now.Add(time.Duration(retryWindowSec) * time.Second)) {
				actionExpiresAt = now.Add(time.Duration(retryWindowSec) * time.Second)
			}
			g.logger.Info().
				Msgf("Game server restarted with no saved action message. Relying on the client to resend the action. Restarting the action timer. Current time: %s. Action expires at: %s (%.3f seconds from now).", now, actionExpiresAt.Format(time.RFC3339), actionExpiresAt.Sub(now).Seconds())

			var canCheck bool
			for _, action := range nextSeatAction.AvailableActions {
				if action == ACTION_CHECK {
					canCheck = true
					break
				}
			}
			player := handState.PlayersInSeats[nextSeatAction.SeatNo]
			if nextSeatAction.ActionTimesoutAt != 0 {
				g.resetTimer(nextSeatAction.SeatNo, player.PlayerId, canCheck, actionExpiresAt)
			}
			return nil
		}

		g.logger.Info().Msg("Restoring action message from hand state.")
		playerMsg = handState.ActionMsgInProgress
		if playerMsg == nil {
			return fmt.Errorf("Could not restore action message from hand state. Stored message is nil")
		}
	}

	crashtest.Hit(g.gameCode, crashtest.CrashPoint_WAIT_FOR_NEXT_ACTION_1, playerMsg.PlayerId)

	actionMsg, err := g.getClientMsgItem(playerMsg)
	if err != nil {
		return err
	}

	if err := validatePlayerAction(playerMsg, actionMsg, handState, g.isScriptTest); err != nil {
		// Ignore the action message.
		errMsg := "Invalid player action"
		var actionStr string
		var messageSeatNo uint32
		playerActed := actionMsg.GetPlayerActed()
		if playerActed != nil {
			actionStr = playerActed.Action.String()
			messageSeatNo = playerActed.GetSeatNo()
		}
		g.logger.Error().
			Err(err).
			Uint32(logging.HandNumKey, handState.GetHandNum()).
			Uint64(logging.PlayerIDKey, playerMsg.GetPlayerId()).
			Uint32(logging.SeatNumKey, messageSeatNo).
			Str(logging.MsgTypeKey, actionMsg.MessageType).
			Str(logging.ActionKey, actionStr).
			Msg(errMsg)

		// We could get invalid messages (duplicate message, wrong state, etc.)
		// due to client retries. Just acknowlede them so that they stop retrying.
		g.sendActionAck(handState, playerMsg, handState.CurrentActionNum)
		return InvalidMessageError{Msg: errMsg}
	}

	err = g.convertToServerUnits(actionMsg)
	if err != nil {
		g.logger.Error().Err(err).Msg("Could not convert action msg to server units")
		panic(err)
	}

	messageSeatNo := actionMsg.GetPlayerActed().GetSeatNo()

	expectedState := FlowState_WAIT_FOR_NEXT_ACTION
	if handState.FlowState != expectedState {
		errMsg := fmt.Sprintf("onPlayerActed called in wrong flow state. Expected state: %s, Current state: %s", expectedState, handState.FlowState)
		g.logger.Error().
			Uint64(logging.PlayerIDKey, playerMsg.PlayerId).
			Uint32(logging.SeatNumKey, messageSeatNo).
			Uint32(logging.HandNumKey, handState.GetHandNum()).
			Str(logging.ActionKey, actionMsg.GetPlayerActed().GetAction().String()).
			Str(logging.MsgTypeKey, actionMsg.MessageType).
			Msg(errMsg)
		g.sendActionAck(handState, playerMsg, handState.CurrentActionNum)
		return InvalidMessageError{Msg: errMsg}
	}

	// is it run it twice prompt response?
	if handState.RunItTwicePrompt {
		return g.handleRITResponse(playerMsg, actionMsg, handState)
	}

	actionResponseTime := g.actionTimer.GetElapsedTime()
	actedSeconds := uint32(actionResponseTime.Seconds())
	if messageSeatNo == g.actionTimer.GetCurrentTimerMsg().SeatNo {
		// cancel action timer
		g.pausePlayTimer(messageSeatNo)
	}

	handState.ActionMsgInProgress = playerMsg
	err = g.saveHandState(handState, FlowState_PREPARE_NEXT_ACTION)
	if err != nil {
		msg := fmt.Sprintf("Could not save hand state after saving action msg")
		g.logger.Error().
			Uint32(logging.HandNumKey, handState.GetHandNum()).
			Err(err).
			Msgf(msg)
		return errors.Wrap(err, msg)
	}
	g.sendActionAck(handState, playerMsg, handState.CurrentActionNum)

	crashtest.Hit(g.gameCode, crashtest.CrashPoint_WAIT_FOR_NEXT_ACTION_2, playerMsg.PlayerId)

	return g.prepareNextAction(handState, uint64(actedSeconds))
}

func validatePlayerAction(playerMsg *HandMessage, actionMsg *HandMessageItem, handState *HandState, isScriptTest bool) error {
	action := actionMsg.GetPlayerActed()
	if action == nil {
		errMsg := "Invalid action. Msg item does not containe playerActed"
		return InvalidMessageError{Msg: errMsg}
	}

	messageSeatNo := action.GetSeatNo()

	if handState.NextSeatAction == nil {
		if !handState.RunItTwicePrompt {
			errMsg := "Invalid action. There is no next action"
			// This can happen if the action was already processed, but the client is retrying
			// because the acnowledgement got lost in the network.
			return InvalidMessageError{Msg: errMsg}
		}
	} else {
		if messageSeatNo != handState.NextSeatAction.SeatNo {
			// Unexpected seat acted.
			// One scenario this can happen is when a player made a last-second action and the timeout
			// was triggered at the same time. We get two actions in that case - one last-minute action
			// from the player, and the other default action created by the timeout handler on behalf
			// of the player. We are discarding whichever action that came last in that case.
			errMsg := fmt.Sprintf("Invalid seat made action. The next valid action seat is: %d",
				handState.NextSeatAction.SeatNo)
			return InvalidMessageError{Msg: errMsg}
		}
	}

	if (messageSeatNo == 0 || playerMsg.PlayerId == 0) && !isScriptTest {
		errMsg := "Invalid seat number/player ID"
		return InvalidMessageError{Msg: errMsg}
	}

	if !actionMsg.GetPlayerActed().GetTimedOut() {
		if playerMsg.MessageId == "" && !isScriptTest {
			b, err := protojson.Marshal(actionMsg)
			var msgStr string
			if err == nil {
				msgStr = string(b)
			} else {
				msgStr = playerMsg.String()
			}
			errMsg := fmt.Sprintf("Missing message ID. Msg: %s", msgStr)
			return InvalidMessageError{Msg: errMsg}
		}
	}

	if playerMsg.HandNum != handState.HandNum {
		errMsg := fmt.Sprintf("Invalid hand number: %d current hand number: %d", playerMsg.HandNum, handState.HandNum)
		// This can happen if the action was already processed, but the client is retrying
		// through the next hand because the acnowledgement got lost in the network.
		return InvalidMessageError{Msg: errMsg}
	}

	if handState.CurrentState == HandStatus_SHOW_DOWN {
		errMsg := "Unexpected player action. Hand is in show-down state"
		// This can happen if the action was already processed, but the client is retrying
		// because the acnowledgement got lost in the network.
		return InvalidMessageError{Msg: errMsg}
	}

	if action.Action == ACTION_CALL {
		if handState.GetNextSeatAction() == nil {
			return fmt.Errorf("handState.NextSeatAction is nil")
		}
		expectedCallAmount := util.CentsToChips(handState.GetNextSeatAction().CallAmount)
		if action.Amount != expectedCallAmount {
			return fmt.Errorf("Invalid call amount %v. Expected amount: %v", action.Amount, expectedCallAmount)
		}
	}

	if handState.RunItTwicePrompt {
		if !(action.Action == ACTION_RUN_IT_TWICE_YES || action.Action == ACTION_RUN_IT_TWICE_NO) {
			return InvalidMessageError{
				Msg: fmt.Sprintf("Unexpected action. Was expecting %v or %v", ACTION_RUN_IT_TWICE_YES, ACTION_RUN_IT_TWICE_NO),
			}
		}

		seatNo := actionMsg.GetPlayerActed().GetSeatNo()
		runItTwiceState := handState.GetRunItTwice()
		if (seatNo == runItTwiceState.Seat1 && runItTwiceState.Seat1Responded) ||
			(seatNo == runItTwiceState.Seat2 && runItTwiceState.Seat2Responded) {
			return InvalidMessageError{Msg: "Duplicate run-it-twice response"}
		}
	}

	return nil
}

func (g *Game) handleRITResponse(playerMsg *HandMessage, actionMsg *HandMessageItem, handState *HandState) error {
	seatNo := actionMsg.GetPlayerActed().GetSeatNo()
	msgItems, err := g.runItTwiceConfirmation(handState, playerMsg)
	if err != nil {
		return errors.Wrap(err, "Could not handle run-it-twice confirmation")
	}
	g.sendActionAck(handState, playerMsg, handState.CurrentActionNum)

	player := handState.PlayersInSeats[seatNo]
	if actionMsg.GetPlayerActed().GetTimedOut() {
		handState.TimeoutStats[player.PlayerId].ConsecutiveActionTimeouts++
	} else {
		handState.TimeoutStats[player.PlayerId].ConsecutiveActionTimeouts = 0
		handState.TimeoutStats[player.PlayerId].ActedAtLeastOnce = true
	}

	msg := HandMessage{
		HandNum:    handState.HandNum,
		HandStatus: handState.CurrentState,
		MessageId:  g.generateMsgID("RIT_CONFIRM", handState.HandNum, handState.CurrentState, playerMsg.PlayerId, playerMsg.MessageId, handState.CurrentActionNum),
		Messages:   msgItems,
	}

	g.broadcastHandMessage(&msg)
	rit := handState.GetRunItTwice()
	var nextFlowState FlowState
	if rit.Seat1Responded && rit.Seat2Responded {
		// Both players responded.
		nextFlowState = FlowState_MOVE_TO_NEXT_HAND
	} else {
		// Need to wait for the other player to respond.
		nextFlowState = FlowState_WAIT_FOR_NEXT_ACTION
	}
	err = g.saveHandState(handState, nextFlowState)
	if err != nil {
		msg := fmt.Sprintf("Could not save hand state after confirming run-it-twice")
		g.logger.Error().
			Uint32(logging.HandNumKey, handState.GetHandNum()).
			Err(err).
			Msgf(msg)
		return errors.Wrap(err, msg)
	}
	g.handleHandEnded(handState, handState.TotalResultPauseTime, msgItems)

	return nil
}

func (g *Game) prepareNextAction(handState *HandState, actionResponseTime uint64) error {
	expectedState := FlowState_PREPARE_NEXT_ACTION
	if handState.FlowState != expectedState {
		return fmt.Errorf("prepareNextAction called in wrong flow state. Expected state: %s, Current state: %s", expectedState, handState.FlowState)
	}

	if handState == nil {
		errMsg := "Unable to prepare next action. handState is nil"
		return fmt.Errorf(errMsg)
	}

	playerMsg := handState.ActionMsgInProgress
	if playerMsg == nil {
		errMsg := "Unable to get action message in progress. handState.ActionMsgInProgress is nil"
		return fmt.Errorf(errMsg)
	}

	var err error
	actionMsg, err := g.getClientMsgItem(playerMsg)
	if err != nil {
		return err
	}

	allMsgItems := make([]*HandMessageItem, 0)
	var msgItems []*HandMessageItem
	seatNo := actionMsg.GetPlayerActed().GetSeatNo()
	player := handState.PlayersInSeats[seatNo]

	// Send player's current stack to be updated in the UI
	handStage := handState.CurrentState

	err = handState.actionReceived(actionMsg.GetPlayerActed(), actionResponseTime)
	if err != nil {
		return errors.Wrap(err, "Could not update hand state from action")
	}

	playerAction := handState.PlayersActed[seatNo]
	bettingState := handState.RoundState[uint32(handStage)]
	potUpdates := float64(0)
	for _, bet := range bettingState.Betting.SeatBet {
		potUpdates += bet
	}
	playerBalance := bettingState.PlayerBalance[seatNo]

	actionMsg.GetPlayerActed().Stack = playerBalance
	actionMsg.GetPlayerActed().PotUpdates = potUpdates
	if playerAction.Action != ACTION_FOLD {
		actionMsg.GetPlayerActed().Amount = playerAction.Amount
	} else {
		// the game folded this guy's hand
		actionMsg.GetPlayerActed().Action = ACTION_FOLD
		actionMsg.GetPlayerActed().Amount = 0
	}
	// broadcast this message to all the players (let everyone know this player acted)
	allMsgItems = append(allMsgItems, actionMsg)

	if actionMsg.GetPlayerActed().GetTimedOut() {
		handState.TimeoutStats[player.PlayerId].ConsecutiveActionTimeouts++
	} else {
		handState.TimeoutStats[player.PlayerId].ConsecutiveActionTimeouts = 0

		// When the consecutive timeout counts get reported to the api server,
		// the api server needs to know if the player has acted at all this hand
		// so that it can clear the count from the previous hand and start a new counter
		// instead of adding to it.
		handState.TimeoutStats[player.PlayerId].ActedAtLeastOnce = true
	}

	// This number is used to generate hand message IDs uniquely and deterministically across the server crashes.
	handState.CurrentActionNum++

	var nextFlowState FlowState
	if handState.NoActiveSeats == 1 {
		msgItems, err = g.onePlayerRemaining(handState)
		nextFlowState = FlowState_MOVE_TO_NEXT_HAND
	} else if g.runItTwice(handState, playerAction) {
		msgItems, err = g.runItTwicePrompt(handState)
		nextFlowState = FlowState_WAIT_FOR_NEXT_ACTION
	} else if handState.isAllActivePlayersAllIn() || handState.allActionComplete() {
		msgItems, err = g.allPlayersAllIn(handState)
		nextFlowState = FlowState_MOVE_TO_NEXT_HAND
	} else if handState.CurrentState == HandStatus_SHOW_DOWN {
		msgItems, err = g.showdown(handState)
		nextFlowState = FlowState_MOVE_TO_NEXT_HAND
	} else if handState.LastState != handState.CurrentState {
		msgItems, err = g.moveToNextRound(handState)
		nextFlowState = FlowState_WAIT_FOR_NEXT_ACTION
	} else {
		msgItems, err = g.moveToNextAction(handState)
		nextFlowState = FlowState_WAIT_FOR_NEXT_ACTION
	}

	if err != nil {
		return err
	}
	allMsgItems = append(allMsgItems, msgItems...)

	// Create hand message with all of the message items.
	serverMsg := HandMessage{
		HandNum:    handState.HandNum,
		HandStatus: handState.CurrentState,
		MessageId:  g.generateMsgID("ACTION", handState.HandNum, handState.CurrentState, playerMsg.PlayerId, playerMsg.MessageId, handState.CurrentActionNum),
		Messages:   allMsgItems,
	}

	crashtest.Hit(g.gameCode, crashtest.CrashPoint_PREPARE_NEXT_ACTION_1, playerMsg.PlayerId)
	g.logHandState(handState)
	g.broadcastHandMessage(&serverMsg)
	handState.ActionMsgInProgress = nil

	crashtest.Hit(g.gameCode, crashtest.CrashPoint_PREPARE_NEXT_ACTION_2, playerMsg.PlayerId)

	err = g.saveHandState(handState, nextFlowState)
	if err != nil {
		msg := fmt.Sprintf("Could not save hand state after sending next action")
		g.logger.Error().
			Uint32(logging.HandNumKey, handState.GetHandNum()).
			Err(err).
			Msgf(msg)
		return errors.Wrap(err, msg)
	}

	crashtest.Hit(g.gameCode, crashtest.CrashPoint_PREPARE_NEXT_ACTION_3, playerMsg.PlayerId)
	g.handleHandEnded(handState, handState.TotalResultPauseTime, allMsgItems)
	return nil
}

func (g *Game) logHandState(handState *HandState) {
	logLevel := g.logger.GetLevel()
	if logLevel == zerolog.DebugLevel || logLevel == zerolog.TraceLevel {
		nextSeatAction := handState.GetNextSeatAction()
		b, err := protojson.Marshal(nextSeatAction)
		if err != nil {
			g.logger.Warn().Msgf("Cannot log next seat action as json: %s", err)
		} else {
			g.logger.Debug().
				Uint32(logging.HandNumKey, handState.GetHandNum()).
				Msgf("Next seat action: %s", string(b))
		}
	}
}

func (g *Game) handleHandEnded(handState *HandState, totalPauseTime uint32, allMsgItems []*HandMessageItem) {
	// if the last message is hand ended (pause for the result animation)
	handEnded := false
	for _, message := range allMsgItems {
		if message.MessageType == HandEnded {
			handEnded = true
		}
	}

	if handEnded {
		if totalPauseTime > 0 {
			if !util.Env.ShouldDisableDelays() {
				g.logger.Debug().
					Msgf("Sleeping %d milliseconds for result animation", totalPauseTime)
				time.Sleep(time.Duration(totalPauseTime) * time.Millisecond)
			}
		}

		handState.CurrentState = HandStatus_HAND_CLOSED

		g.queueMoveToNextHand()
	}
}

func (g *Game) queueMoveToNextHand() {
	gameMessage := &GameMessage{
		GameId:      g.gameID,
		MessageType: GameMoveToNextHand,
	}
	g.QueueGameMessage(gameMessage)
}

func (g *Game) sendActionAck(handState *HandState, playerMsg *HandMessage, currentActionNum uint32) {
	actionMsg, err := g.getClientMsgItem(playerMsg)
	if err != nil {
		g.logger.Error().Err(err).Msg("Invalid client msg in sendActionAck")
		return
	}

	if actionMsg.GetPlayerActed().GetTimedOut() {
		// Default action is generated by the server on action timeout. Don't acknowledge that.
		return
	}

	if playerMsg.SeatNo > uint32(len(handState.PlayersInSeats)) {
		g.logger.Warn().Msgf("Not sending ack. Invalid seat number %d", playerMsg.SeatNo)
		return
	}

	player := handState.PlayersInSeats[playerMsg.SeatNo]
	if player == nil {
		g.logger.Warn().Msg("Not sending ack. Player in seat is nil")
		return
	}
	if player.PlayerId == 0 {
		g.logger.Warn().Msg("Not sending ack. Player ID is 0")
		return
	}

	ack := &HandMessageItem{
		MessageType: HandMsgAck,
		Content: &HandMessageItem_MsgAck{
			MsgAck: &MsgAcknowledgement{
				MessageId:   playerMsg.GetMessageId(),
				MessageType: actionMsg.GetMessageType(),
			},
		},
	}

	serverMsg := &HandMessage{
		PlayerId:   player.PlayerId,
		HandNum:    playerMsg.GetHandNum(),
		HandStatus: playerMsg.GetHandStatus(),
		SeatNo:     playerMsg.GetSeatNo(),
		MessageId:  g.generateMsgID("ACK", playerMsg.GetHandNum(), playerMsg.GetHandStatus(), playerMsg.GetPlayerId(), playerMsg.GetMessageId(), currentActionNum),
		Messages:   []*HandMessageItem{ack},
	}
	g.sendHandMessageToPlayer(serverMsg, player.PlayerId)
	g.logger.Debug().
		Uint64(logging.PlayerIDKey, playerMsg.GetPlayerId()).
		Uint32(logging.SeatNumKey, playerMsg.GetSeatNo()).
		Uint32(logging.HandNumKey, handState.GetHandNum()).
		Str(logging.ActionKey, actionMsg.GetPlayerActed().GetAction().String()).
		Msgf("Acknowledgment sent to player. Message Id: %s", playerMsg.GetMessageId())
}

func (g *Game) getPots(handState *HandState) ([]float64, []*SeatsInPots) {
	pots := make([]float64, 0)
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

func (g *Game) getPlayerCardRank(handState *HandState, boardCards []uint32) map[uint32]string {
	// get rank
	playerCardRank := make(map[uint32]string)

	// get player card ranking
	board := make([]byte, 0)
	for _, c := range boardCards {
		board = append(board, byte(c))
	}

	for seatNo, playerID := range handState.ActiveSeats {
		if playerID == 0 {
			continue
		}
		playersCards := handState.PlayersCards[uint32(seatNo)]

		cards := make([]byte, len(board)+len(playersCards))
		copy(cards, board)

		playersCardsInBytes := make([]byte, len(playersCards))
		i := len(board)
		for idx, c := range playersCards {
			cards[i] = byte(c)
			playersCardsInBytes[idx] = byte(c)
			i++
		}
		pokerCards := poker.FromByteCards(cards)
		pokerBoardCards := poker.FromByteCards(board)
		pokerPlayerCards := poker.FromByteCards(playersCardsInBytes)

		var rank int32
		if handState.GameType == GameType_HOLDEM {
			rank, _ = poker.Evaluate(pokerCards)
		} else if handState.GameType == GameType_PLO ||
			handState.GameType == GameType_PLO_HILO ||
			handState.GameType == GameType_FIVE_CARD_PLO_HILO ||
			handState.GameType == GameType_FIVE_CARD_PLO {
			result := poker.EvaluateOmaha(pokerPlayerCards, pokerBoardCards)
			rank = result.HiRank
		}

		if rank != 0 {
			playerCardRank[uint32(seatNo)] = poker.RankString(rank)
		}
	}

	return playerCardRank
}

func (g *Game) gotoFlop(handState *HandState) ([]*HandMessageItem, error) {
	g.logger.Debug().
		Uint32(logging.HandNumKey, handState.GetHandNum()).
		Msgf("Moving to %s", HandStatus_name[int32(handState.CurrentState)])

	flopCards := make([]uint32, 3)
	for i, card := range handState.BoardCards[:3] {
		flopCards[i] = uint32(card)
	}
	err := handState.setupFlop()
	if err != nil {
		return nil, err
	}
	pots, seatsInPots := g.getPots(handState)
	balance := make(map[uint32]float64)
	for seatNo, player := range handState.PlayersInSeats {
		if seatNo == 0 {
			continue
		}
		balance[uint32(seatNo)] = player.Stack
	}

	// update player stats
	for _, playerID := range handState.ActiveSeats {
		if playerID == 0 {
			continue
		}
		handState.PlayerStats[playerID].InFlop = true
	}
	playerCardRanks := g.getPlayerCardRank(handState, flopCards)
	if util.Env.IsEncryptionEnabled() {
		var err error
		playerCardRanks, err = g.encryptPlayerCardRanks(playerCardRanks, handState.PlayersInSeats)
		if err != nil {
			return nil, err
		}
	}

	boards := make([]*Board, 0)
	for _, board := range handState.Boards {
		flopCards := make([]uint32, 3)
		for i, card := range board.Cards[:3] {
			flopCards[i] = uint32(card)
		}
		board1 := &Board{
			BoardNo: board.BoardNo,
			Cards:   flopCards,
		}
		boards = append(boards, board1)
	}
	cardsStr := poker.CardsToString(flopCards)
	flop := &Flop{
		Board:           flopCards,
		Boards:          boards,
		CardsStr:        cardsStr,
		Pots:            pots,
		SeatsPots:       seatsInPots,
		PlayerBalance:   balance,
		PlayerCardRanks: playerCardRanks,
	}
	msgItem := &HandMessageItem{
		MessageType: HandFlop,
		Content:     &HandMessageItem_Flop{Flop: flop},
	}

	return []*HandMessageItem{msgItem}, nil
}

func (g *Game) gotoTurn(handState *HandState) ([]*HandMessageItem, error) {
	g.logger.Debug().
		Uint32(logging.HandNumKey, handState.GetHandNum()).
		Msgf("Moving to %s", HandStatus_name[int32(handState.CurrentState)])

	err := handState.setupTurn()
	if err != nil {
		return nil, err
	}

	boardCards := make([]uint32, 4)
	for i, card := range handState.BoardCards[:4] {
		boardCards[i] = uint32(card)
	}

	cardsStr := poker.CardsToString(boardCards)

	pots, seatsInPots := g.getPots(handState)

	balance := make(map[uint32]float64)
	for seatNo, player := range handState.PlayersInSeats {
		if seatNo == 0 {
			continue
		}
		balance[uint32(seatNo)] = player.Stack
	}

	// update player stats
	for _, playerID := range handState.ActiveSeats {
		if playerID == 0 {
			continue
		}
		handState.PlayerStats[playerID].InTurn = true
	}

	playerCardRanks := g.getPlayerCardRank(handState, boardCards)
	if util.Env.IsEncryptionEnabled() {
		var err error
		playerCardRanks, err = g.encryptPlayerCardRanks(playerCardRanks, handState.PlayersInSeats)
		if err != nil {
			return nil, err
		}
	}

	boards := make([]*Board, 0)
	for _, board := range handState.Boards {
		turnCards := make([]uint32, 4)
		for i, card := range board.Cards[:4] {
			turnCards[i] = uint32(card)
		}
		board1 := &Board{
			BoardNo: board.BoardNo,
			Cards:   turnCards,
		}
		boards = append(boards, board1)
	}
	turn := &Turn{
		Board:           boardCards,
		Boards:          boards,
		TurnCard:        boardCards[3],
		CardsStr:        cardsStr,
		Pots:            pots,
		SeatsPots:       seatsInPots,
		PlayerBalance:   balance,
		PlayerCardRanks: playerCardRanks,
	}
	msgItem := &HandMessageItem{
		MessageType: HandTurn,
		Content:     &HandMessageItem_Turn{Turn: turn},
	}

	return []*HandMessageItem{msgItem}, nil
}

func (g *Game) gotoRiver(handState *HandState) ([]*HandMessageItem, error) {
	g.logger.Debug().
		Uint32(logging.HandNumKey, handState.GetHandNum()).
		Msgf("Moving to %s", HandStatus_name[int32(handState.CurrentState)])

	err := handState.setupRiver()
	if err != nil {
		return nil, err
	}

	cardsStr := poker.CardsToString(handState.BoardCards)
	boardCards := make([]uint32, 5)
	for i, card := range handState.BoardCards {
		boardCards[i] = uint32(card)
	}

	pots, seatsInPots := g.getPots(handState)

	balance := make(map[uint32]float64)
	for seatNo, player := range handState.PlayersInSeats {
		if seatNo == 0 {
			continue
		}
		balance[uint32(seatNo)] = player.Stack
	}

	// update player stats
	for _, playerID := range handState.ActiveSeats {
		if playerID == 0 {
			continue
		}
		handState.PlayerStats[playerID].InRiver = true
	}

	playerCardRanks := g.getPlayerCardRank(handState, boardCards)
	if util.Env.IsEncryptionEnabled() {
		var err error
		playerCardRanks, err = g.encryptPlayerCardRanks(playerCardRanks, handState.PlayersInSeats)
		if err != nil {
			return nil, err
		}
	}
	boards := make([]*Board, 0)
	for _, board := range handState.Boards {
		riverCards := make([]uint32, 5)
		for i, card := range board.Cards[:5] {
			riverCards[i] = uint32(card)
		}
		board1 := &Board{
			BoardNo: board.BoardNo,
			Cards:   riverCards,
		}
		boards = append(boards, board1)
	}

	river := &River{
		Board:           boardCards,
		Boards:          boards,
		RiverCard:       uint32(handState.BoardCards[4]),
		CardsStr:        cardsStr,
		Pots:            pots,
		SeatsPots:       seatsInPots,
		PlayerBalance:   balance,
		PlayerCardRanks: playerCardRanks,
	}
	msgItem := &HandMessageItem{
		MessageType: HandRiver,
		Content:     &HandMessageItem_River{River: river},
	}

	return []*HandMessageItem{msgItem}, nil
}

func (g *Game) encryptPlayerCardRanks(playerCardRanks map[uint32]string, playersInSeats []*PlayerInSeatState) (map[uint32]string, error) {
	encryptedRanks := make(map[uint32]string)
	for seatNo, cardRank := range playerCardRanks {
		if seatNo == 0 {
			continue
		}
		player := playersInSeats[seatNo]
		encryptedCardRank, err := g.EncryptAndB64ForPlayer([]byte(cardRank), player.PlayerId)
		if err != nil {
			return nil, err
		}
		encryptedRanks[seatNo] = encryptedCardRank
	}
	return encryptedRanks, nil
}

func (g *Game) handEnded(handState *HandState) ([]*HandMessageItem, error) {
	handEnded := &HandMessageItem{
		MessageType: HandEnded,
	}

	return []*HandMessageItem{handEnded}, nil
}

func (g *Game) sendResult2(hs *HandState, handResultClient *HandResultClient) ([]*HandMessageItem, error) {
	//handResultClient.PlayerInfo = make(map[uint32]*PlayerHandInfo)
	hs.CurrentState = HandStatus_RESULT

	for seatNo, player := range hs.PlayersInSeats {
		if seatNo == 0 || !player.Inhand || player.OpenSeat {
			continue
		}

		before := float64(0.0)
		after := float64(0.0)
		for _, playerBalance := range hs.BalanceBeforeHand {
			if playerBalance.SeatNo == uint32(seatNo) {
				before = playerBalance.Balance
				break
			}
		}
		if balance, ok := handResultClient.PlayerInfo[uint32(seatNo)]; ok {
			after = balance.Balance.After
		} else {
			after = player.Stack
		}
		rakePaid := float64(0.0)
		if playerRake, ok := hs.RakePaid[player.PlayerId]; ok {
			rakePaid = playerRake
		}
		if _, ok := handResultClient.PlayerInfo[uint32(seatNo)]; !ok {
			handResultClient.PlayerInfo[uint32(seatNo)] = &PlayerHandInfo{
				Id: player.PlayerId,
				Balance: &HandPlayerBalance{
					Before: before,
					After:  after,
				},
				Received: player.PlayerReceived,
				RakePaid: rakePaid,
			}
		}
	}
	var highHandWinners []*HighHandWinner

	// determine high hand winners
	if hs.HighHandTracked {
		highHandWinners = make([]*HighHandWinner, 0)
		// walk through each player's rank
		highRankFound := false
		highRank := uint32(0)
		for _, board := range handResultClient.Boards {
			for _, playerRank := range board.PlayerRank {
				if playerRank.HhRank == 0 ||
					playerRank.HhRank > MIN_FULLHOUSE_RANK {
					continue
				}
				if hs.HighHandRank == 0 {
					highRankFound = true
					highRank = playerRank.HhRank
				}
				if playerRank.HhRank <= hs.HighHandRank {
					highRankFound = true
					highRank = playerRank.HiRank
				}
			}
		}

		if highRankFound {
			for _, board := range handResultClient.Boards {
				for seatNo, playerRank := range board.PlayerRank {
					if playerRank.HhRank == highRank {
						player := hs.PlayersInSeats[seatNo]
						winner := &HighHandWinner{
							PlayerId:    player.PlayerId,
							PlayerName:  player.Name,
							SeatNo:      seatNo,
							HhRank:      playerRank.HhRank,
							HhCards:     playerRank.HhCards,
							BoardNo:     board.BoardNo,
							PlayerCards: poker.ByteCardsToUint32Cards(hs.PlayersCards[seatNo]),
						}
						highHandWinners = append(highHandWinners, winner)
					}
				}
			}
		}
	}

	handResultClient.HandNum = hs.HandNum
	handResultClient.HighHandWinners = highHandWinners
	msgItem2 := &HandMessageItem{
		MessageType: HandResultMessage2,
		Content:     &HandMessageItem_HandResultClient{HandResultClient: handResultClient},
	}
	msgItems := make([]*HandMessageItem, 0)
	msgItems = append(msgItems, msgItem2)
	return msgItems, nil
}

func (g *Game) moveToNextRound(handState *HandState) ([]*HandMessageItem, error) {
	if handState.LastState == HandStatus_DEAL {
		// How do we get here?
		g.logger.Warn().
			Msg("handState.LastState == HandStatus_DEAL in moveToNextRound")
		return []*HandMessageItem{}, nil
	}

	// remove folded players from the pots
	handState.removeFoldedPlayersFromPots()

	var allMsgItems []*HandMessageItem
	var msgItems []*HandMessageItem
	var err error

	if handState.LastState == HandStatus_PREFLOP && handState.CurrentState == HandStatus_FLOP {
		msgItems, err = g.gotoFlop(handState)
	} else if handState.LastState == HandStatus_FLOP && handState.CurrentState == HandStatus_TURN {
		msgItems, err = g.gotoTurn(handState)
	} else if handState.LastState == HandStatus_TURN && handState.CurrentState == HandStatus_RIVER {
		msgItems, err = g.gotoRiver(handState)
	}
	if err != nil {
		return nil, err
	}
	allMsgItems = append(allMsgItems, msgItems...)

	msgItems, err = g.moveToNextAction(handState)
	if err != nil {
		return nil, err
	}
	allMsgItems = append(allMsgItems, msgItems...)

	return allMsgItems, nil
}

func (g *Game) moveToNextAction(handState *HandState) ([]*HandMessageItem, error) {
	if handState.NextSeatAction == nil {
		return nil, fmt.Errorf("moveToNextAction called when handState.NextSeatAction == nil")
	}

	var allMsgItems []*HandMessageItem

	var canCheck bool
	for _, action := range handState.NextSeatAction.AvailableActions {
		if action == ACTION_CHECK {
			canCheck = true
			break
		}
	}
	// tell the next player to act
	yourActionMsg := &HandMessageItem{
		MessageType: HandPlayerAction,
		Content:     &HandMessageItem_SeatAction{SeatAction: handState.NextSeatAction},
	}
	player := handState.PlayersInSeats[handState.NextSeatAction.SeatNo]
	// Additional time for network delay, client animation delay, etc.
	// This doesn't need to be accurate. When the action times out,
	// the client will submit a default action. This is just a fallback
	// in case the client is unable to do that.
	actionTimesoutAt := time.Now().Add(time.Duration(handState.ActionTime+g.timerCushionSec) * time.Second)
	handState.NextSeatAction.ActionTimesoutAt = actionTimesoutAt.Unix()
	g.resetTimer(handState.NextSeatAction.SeatNo, player.PlayerId, canCheck, actionTimesoutAt)
	allMsgItems = append(allMsgItems, yourActionMsg)

	pots := make([]float64, 0)
	for _, pot := range handState.Pots {
		pots = append(pots, pot.Pot)
	}
	currentPot := pots[len(pots)-1]
	roundState := handState.RoundState[uint32(handState.CurrentState)]
	currentBettingRound := roundState.Betting
	seatBets := currentBettingRound.SeatBet
	bettingRoundBets := float64(0)
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
		BetAmount:  handState.getMaxBet(),
	}

	nextActionMsg := &HandMessageItem{
		MessageType: HandNextAction,
		Content:     &HandMessageItem_ActionChange{ActionChange: actionChange},
	}

	allMsgItems = append(allMsgItems, nextActionMsg)

	return allMsgItems, nil
}

func (g *Game) allPlayersAllIn(handState *HandState) ([]*HandMessageItem, error) {
	var allMsgItems []*HandMessageItem
	var msgItems []*HandMessageItem
	var err error

	_, seatsInPots := g.getPots(handState)

	// broadcast the players no more actions
	noMoreActions := &NoMoreActions{
		Pots: seatsInPots,
	}
	msgItem := &HandMessageItem{
		MessageType: HandNoMoreActions,
		Content:     &HandMessageItem_NoMoreActions{NoMoreActions: noMoreActions},
	}

	allMsgItems = append(allMsgItems, msgItem)

	for handState.CurrentState != HandStatus_SHOW_DOWN {
		switch handState.CurrentState {
		case HandStatus_FLOP:
			msgItems, err = g.gotoFlop(handState)
			handState.CurrentState = HandStatus_TURN
		case HandStatus_TURN:
			msgItems, err = g.gotoTurn(handState)
			handState.CurrentState = HandStatus_RIVER
		case HandStatus_RIVER:
			msgItems, err = g.gotoRiver(handState)
			handState.CurrentState = HandStatus_SHOW_DOWN
		}
		if err != nil {
			return nil, err
		}
		allMsgItems = append(allMsgItems, msgItems...)
	}

	msgItems, err = g.showdown(handState)
	if err != nil {
		return nil, errors.Wrap(err, "Error from showdown")
	}
	allMsgItems = append(allMsgItems, msgItems...)
	return allMsgItems, nil
}

func (g *Game) showdown(handState *HandState) ([]*HandMessageItem, error) {
	// update hand stats
	handState.HandStats.EndedAtShowdown = true
	// update player stats
	for _, playerID := range handState.ActiveSeats {
		if playerID == 0 {
			continue
		}
		handState.PlayerStats[playerID].WentToShowdown = true
	}

	handState.removeFoldedPlayersFromPots()
	handState.removeEmptyPots()
	handState.HandCompletedAt = HandStatus_SHOW_DOWN

	// track whether the player is active in this round or not
	for seatNo, playerID := range handState.ActiveSeats {
		if playerID == 0 {
			continue
		}
		player := handState.PlayersInSeats[seatNo]
		if player.Inhand {
			player.Round = HandStatus_SHOW_DOWN
		}
	}

	var allMsgItems []*HandMessageItem
	var msgItems []*HandMessageItem
	var err error
	msgItems, err = g.generateAndSendResult(handState)
	if err != nil {
		return nil, err
	}
	allMsgItems = append(allMsgItems, msgItems...)

	msgItems, err = g.handEnded(handState)
	if err != nil {
		return nil, err
	}
	allMsgItems = append(allMsgItems, msgItems...)

	return allMsgItems, nil
}

func (g *Game) onePlayerRemaining(handState *HandState) ([]*HandMessageItem, error) {
	switch handState.CurrentState {
	case HandStatus_DEAL:
		handState.HandStats.EndedAtPreflop = true
	case HandStatus_FLOP:
		handState.HandStats.EndedAtFlop = true
	case HandStatus_TURN:
		handState.HandStats.EndedAtTurn = true
	case HandStatus_RIVER:
		handState.HandStats.EndedAtRiver = true
	}
	// every one folded except one player, send the pot to the player
	handState.everyOneFoldedWinners()
	handState.CurrentState = HandStatus_RESULT

	var allMsgItems []*HandMessageItem
	var msgItems []*HandMessageItem
	var err error
	msgItems, err = g.generateAndSendResult(handState)
	if err != nil {
		return nil, err
	}
	allMsgItems = append(allMsgItems, msgItems...)

	msgItems, err = g.handEnded(handState)
	if err != nil {
		return nil, err
	}
	allMsgItems = append(allMsgItems, msgItems...)

	return allMsgItems, nil
}

func (g *Game) generateAndSendResult(handState *HandState) ([]*HandMessageItem, error) {
	hs := handState
	for i := len(hs.Pots) - 1; i >= 0; i-- {
		currentPot := hs.Pots[len(hs.Pots)-1]
		if currentPot.Pot == 0 {
			hs.Pots = hs.Pots[:len(hs.Pots)-1]
			continue
		}
		// if current pot has only one player, return the money to the player
		if len(currentPot.Seats) == 1 {
			activePlayer := currentPot.Seats[0]
			player := hs.PlayersInSeats[activePlayer]
			player.Stack += currentPot.Pot
			// remove the pot
			hs.Pots = hs.Pots[:len(hs.Pots)-1]
		}
	}

	handResultProcessor := NewHandResultProcessor(handState, g.chipUnit, uint32(handState.MaxSeats), nil)

	handResult2Client := handResultProcessor.determineWinners()
	allMsgItems := make([]*HandMessageItem, 0)
	handResult2Client.PlayerStats = handState.GetPlayerStats()
	handResult2Client.TimeoutStats = handState.GetTimeoutStats()

	msgItems, err := g.sendResult2(handState, handResult2Client)
	if err != nil {
		return nil, err
	}

	// determine total pause time
	totalPauseTime := uint32(0)
	for _, pot := range handResult2Client.PotWinners {
		for _, board := range pot.BoardWinners {
			totalPauseTime = totalPauseTime + uint32(len(board.HiWinners))*hs.ResultPauseTime
			totalPauseTime = totalPauseTime + uint32(len(board.LowWinners))*hs.ResultPauseTime
		}
	}
	hs.TotalResultPauseTime = totalPauseTime

	// don't pause too long if we didn't go to showdown
	if handResult2Client.WonAt != HandStatus_SHOW_DOWN {
		hs.TotalResultPauseTime = 5000
		handResult2Client.PauseTimeSecs = 5000
	} else {
		handResult2Client.PauseTimeSecs = hs.ResultPauseTime
	}

	allMsgItems = append(allMsgItems, msgItems...)
	// send the hand to the database to store first
	// handResult := handResultProcessor.getResult(true /*db*/)
	// handResult.NoCards = g.NumCards(handState.GameType)
	// handResult.SmallBlind = handState.SmallBlind
	// handResult.BigBlind = handState.BigBlind
	// handResult.MaxPlayers = handState.MaxSeats

	handResultServer := &HandResultServer{
		GameId:        hs.GameId,
		HandNum:       hs.HandNum,
		GameType:      hs.GameType,
		ButtonPos:     hs.ButtonPos,
		NoCards:       g.NumCards(handState.GameType),
		HandLog:       hs.getLog(),
		HandStats:     hs.GetHandStats(),
		RunItTwice:    hs.RunItTwiceConfirmed,
		SmallBlind:    hs.SmallBlind,
		BigBlind:      hs.BigBlind,
		MaxPlayers:    hs.MaxSeats,
		Result:        handResult2Client,
		CollectedAnte: hs.CollectedAnte,
	}

	err = g.analyzeResult(handResultServer)
	if err != nil {
		var msg string
		b, e := protojson.Marshal(handResultServer)
		if e != nil {
			msg = "Result analysis found issues."
		} else {
			msg = fmt.Sprintf("Result analysis found issues. Result: %s", string(b))
		}
		g.logger.Error().Err(err).
			Uint32(logging.HandNumKey, hs.GetHandNum()).
			Msg(msg)

		if g.isScriptTest || util.Env.IsSystemTest() {
			panic(msg)
		}
	}

	sendResultToAPI := !g.isScriptTest
	if sendResultToAPI {
		saveResult, err := g.saveHandResult2ToAPIServer(handResultServer)
		if err != nil {
			return nil, errors.Wrapf(err, "Could not save hand result to api server")
		}
		if saveResult != nil {
			// retry here
		}
	}

	return allMsgItems, nil
}

func (g *Game) analyzeResult(handResult *HandResultServer) error {
	var playerBalanceBefore float64
	var playerBalanceAfter float64
	var rakeCollectedTotal float64
	errMsgs := make([]string, 0)
	result := handResult.Result
	for _, pi := range result.PlayerInfo {
		playerBalanceBefore += pi.Balance.Before
		playerBalanceAfter += pi.Balance.After
		rakeCollectedTotal += pi.RakePaid
	}

	expectedAfter := playerBalanceBefore - rakeCollectedTotal
	after := playerBalanceAfter
	if expectedAfter != after {
		errMsgs = append(errMsgs, fmt.Sprintf("Chips don't add up. Before: %f, Rake: %f, After: %f", playerBalanceBefore, rakeCollectedTotal, playerBalanceAfter))
	}

	if len(errMsgs) > 0 {
		return fmt.Errorf(strings.Join(errMsgs, " "))
	}
	return nil
}
