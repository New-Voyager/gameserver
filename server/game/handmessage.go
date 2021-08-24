package game

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"voyager.com/server/crashtest"
	"voyager.com/server/poker"
	"voyager.com/server/util"
)

const pauseTime = uint32(3000)

func (g *Game) handleHandMessage(message *HandMessage) {
	err := g.validateClientMsg(message)
	if err != nil {
		channelGameLogger.Error().
			Uint32("club", g.config.ClubId).
			Str("game", g.config.GameCode).
			Uint32("player", message.SeatNo).
			Msgf(err.Error())
		return
	}

	msgItem := g.getClientMsgItem(message)
	channelGameLogger.Debug().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
		Uint32("player", message.SeatNo).
		Str("message", msgItem.MessageType).
		Msg(fmt.Sprintf("%v", message))

	switch msgItem.MessageType {
	case HandPlayerActed:
		handState, err := g.loadHandState()
		if err != nil {
			channelGameLogger.Error().Msgf("Unable to load hand state. Error: %s", err.Error())
			break
		}

		err = g.onPlayerActed(message, handState)
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

func (g *Game) validateClientMsg(message *HandMessage) error {
	// Messages from the client should only contain one item.
	msgItems := message.GetMessages()
	if len(msgItems) != 1 {
		return fmt.Errorf("Hand message from the client should only contain one item, but contains %d items", len(msgItems))
	}
	return nil
}

func (g *Game) getClientMsgItem(message *HandMessage) *HandMessageItem {
	msgItems := message.GetMessages()
	// Messages from the client should only contain one item.
	return msgItems[0]
}

func (g *Game) onQueryCurrentHand(playerMsg *HandMessage) error {
	// get hand state
	handState, err := g.loadHandState()
	if err != nil {
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
		NoCards:       g.NumCards(g.config.GameType),
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
		if action.State == PlayerActState_PLAYER_ACT_EMPTY_SEAT {
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
		currentHandState.NextSeatToAct = handState.NextSeatAction.SeatNo
		currentHandState.RemainingActionTime = g.GetRemainingActionTime()
		currentHandState.NextSeatAction = handState.NextSeatAction
	}
	currentHandState.PlayersStack = make(map[uint64]float32)
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
			now := time.Now()
			// Give some time for the client to retry before timing it out.
			retryWindowSec := 10
			actionExpiresAt := time.Unix(handState.NextSeatAction.ActionTimesoutAt, 0)
			if actionExpiresAt.Before(now.Add(time.Duration(retryWindowSec) * time.Second)) {
				actionExpiresAt = now.Add(time.Duration(retryWindowSec) * time.Second)
			}
			channelGameLogger.Info().
				Uint32("club", g.config.ClubId).
				Str("game", g.config.GameCode).
				Msgf("Game server restarted with no saved action message. Relying on the client to resend the action. Restarting the action timer. Current time: %s. Action expires at: %s (%f seconds from now).", now, actionExpiresAt, actionExpiresAt.Sub(now).Seconds())

			var canCheck bool
			for _, action := range handState.NextSeatAction.AvailableActions {
				if action == ACTION_CHECK {
					canCheck = true
					break
				}
			}
			player := handState.PlayersInSeats[handState.NextSeatAction.SeatNo]
			if handState.NextSeatAction.ActionTimesoutAt != 0 {
				g.resetTimer(handState.NextSeatAction.SeatNo, player.PlayerId, canCheck, actionExpiresAt)
			}
			return nil
		}
		channelGameLogger.Info().
			Uint32("club", g.config.ClubId).
			Str("game", g.config.GameCode).
			Msg("Restoring action message from hand state.")
		playerMsg = handState.ActionMsgInProgress
	}

	actionMsg := g.getClientMsgItem(playerMsg)
	messageSeatNo := actionMsg.GetPlayerActed().GetSeatNo()
	channelGameLogger.Debug().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
		Uint32("player", messageSeatNo).
		Str("messageType", actionMsg.MessageType).
		Msg(fmt.Sprintf("%v", playerMsg))

	crashtest.Hit(g.config.GameCode, crashtest.CrashPoint_WAIT_FOR_NEXT_ACTION_1, playerMsg.PlayerId)

	if messageSeatNo == 0 && !g.isScriptTest {
		errMsg := fmt.Sprintf("Invalid seat number [%d] for player ID %d. Ignoring the action message.", messageSeatNo, playerMsg.PlayerId)
		channelGameLogger.Error().
			Uint32("club", g.config.ClubId).
			Str("game", g.config.GameCode).
			Msgf(errMsg)
		return fmt.Errorf(errMsg)
	}

	if handState.NextSeatAction != nil && (actionMsg.GetPlayerActed().GetSeatNo() != handState.NextSeatAction.SeatNo) {
		// Unexpected seat acted.
		// One scenario this can happen is when a player made a last-second action and the timeout
		// was triggered at the same time. We get two actions in that case - one last-minute action
		// from the player, and the other default action created by the timeout handler on behalf
		// of the player. We are discarding whichever action that came last in that case.
		errMsg := fmt.Sprintf("Invalid seat %d made action. Ignored. The next valid action seat is: %d",
			actionMsg.GetPlayerActed().GetSeatNo(), handState.NextSeatAction.SeatNo)
		channelGameLogger.Error().
			Str("game", g.config.GameCode).
			Uint32("hand", handState.GetHandNum()).
			Msg(errMsg)
		if !actionMsg.GetPlayerActed().GetTimedOut() {
			// Acknowledge so that the client stops retrying.
			g.sendActionAck(playerMsg, handState.CurrentActionNum)
		}
		return fmt.Errorf(errMsg)
	}

	if !actionMsg.GetPlayerActed().GetTimedOut() {
		if playerMsg.MessageId == "" && !g.isScriptTest {
			errMsg := fmt.Sprintf("Missing message ID for player ID %d Seat %d. Ignoring the action message.", playerMsg.PlayerId, messageSeatNo)
			channelGameLogger.Error().
				Uint32("club", g.config.ClubId).
				Str("game", g.config.GameCode).
				Msgf(errMsg)
			return fmt.Errorf(errMsg)
		}
	}

	// if the hand number does not match, ignore the message
	if playerMsg.HandNum != handState.HandNum {
		errMsg := fmt.Sprintf("Invalid hand number: %d current hand number: %d", playerMsg.HandNum, handState.HandNum)
		channelGameLogger.Error().
			Uint32("club", g.config.ClubId).
			Str("game", g.config.GameCode).
			Uint32("player", messageSeatNo).
			Str("messageType", actionMsg.MessageType).
			Msg(errMsg)

		// This can happen if the action was already processed, but the client is retrying
		// because the acnowledgement got lost in the network. Just acknowledge so that
		// the client stops retrying.
		g.sendActionAck(playerMsg, handState.CurrentActionNum)
		return fmt.Errorf(errMsg)
	}

	if err := validatePlayerAction(actionMsg.GetPlayerActed(), handState); err != nil {
		// Ignore the action message.
		errMsg := fmt.Sprintf("Invalid player action: %s", err)
		channelGameLogger.Error().
			Uint32("club", g.config.ClubId).
			Str("game", g.config.GameCode).
			Uint32("player", messageSeatNo).
			Str("messageType", actionMsg.MessageType).
			Msg(errMsg)

		return fmt.Errorf(errMsg)
	}

	// is it run it twice prompt response?
	if handState.RunItTwicePrompt {
		actionMsg := g.getClientMsgItem(playerMsg)
		action := actionMsg.GetPlayerActed().Action
		if !(action == ACTION_RUN_IT_TWICE_YES || action == ACTION_RUN_IT_TWICE_NO) {
			return fmt.Errorf("Unexpected action %v. Was expecting %v or %v", action, ACTION_RUN_IT_TWICE_YES, ACTION_RUN_IT_TWICE_NO)
		}
		seatNo := actionMsg.GetPlayerActed().GetSeatNo()
		runItTwiceState := handState.GetRunItTwice()
		if (seatNo == runItTwiceState.Seat1 && runItTwiceState.Seat1Responded) ||
			(seatNo == runItTwiceState.Seat2 && runItTwiceState.Seat2Responded) {
			channelGameLogger.Info().
				Uint32("club", g.config.ClubId).
				Str("game", g.config.GameCode).
				Msgf("Received duplicate run-it-twice response for seat %d. This can happen if the player acted too late and the timeout was triggered at the same time.", seatNo)
			return nil
		}
		msgItems, err := g.runItTwiceConfirmation(handState, playerMsg)
		if err != nil {
			return err
		}
		if !actionMsg.GetPlayerActed().GetTimedOut() {
			// acknowledge so that player does not resend the message
			g.sendActionAck(playerMsg, handState.CurrentActionNum)
		}

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
		g.saveHandState(handState)
		g.handleHandEnded(handState.TotalResultPauseTime, msgItems)

		return nil
	}

	if handState.NextSeatAction == nil {
		errMsg := "Invalid action. There is no next action"
		channelGameLogger.Error().
			Uint32("club", g.config.ClubId).
			Str("game", g.config.GameCode).
			Uint32("player", messageSeatNo).
			Str("messageType", actionMsg.MessageType).
			Str("action", actionMsg.GetPlayerActed().Action.String()).
			Msg(errMsg)

		// This can happen if the action was already processed, but the client is retrying
		// because the acnowledgement got lost in the network. Just acknowledge so that
		// the client stops retrying.
		g.sendActionAck(playerMsg, handState.CurrentActionNum)
		return fmt.Errorf(errMsg)
	}

	if handState.CurrentState == HandStatus_SHOW_DOWN {
		errMsg := "Invalid action. Hand is in show-down state"
		channelGameLogger.Error().
			Uint32("club", g.config.ClubId).
			Str("game", g.config.GameCode).
			Uint32("player", messageSeatNo).
			Str("messageType", actionMsg.MessageType).
			Msg(errMsg)

		// This can happen if the action was already processed, but the client is retrying
		// because the acnowledgement got lost in the network. Just acknowledge so that
		// the client stops retrying.
		g.sendActionAck(playerMsg, handState.CurrentActionNum)
		return fmt.Errorf(errMsg)
	}

	expectedState := FlowState_WAIT_FOR_NEXT_ACTION
	if handState.FlowState != expectedState {
		errMsg := fmt.Sprintf("onPlayerActed called in wrong flow state. Ignoring message. Expected state: %s, Actual state: %s", expectedState, handState.FlowState)
		channelGameLogger.Error().
			Uint32("club", g.config.ClubId).
			Str("game", g.config.GameCode).
			Uint32("player", messageSeatNo).
			Str("messageType", actionMsg.MessageType).
			Msg(errMsg)
		return nil
	}

	actionResponseTime := g.actionTimer.GetElapsedTime()
	actedSeconds := uint32(actionResponseTime.Seconds())
	if messageSeatNo == g.actionTimer.GetCurrentTimerMsg().SeatNo {
		// cancel action timer
		g.pausePlayTimer(messageSeatNo)
	}
	handState.ActionMsgInProgress = playerMsg
	g.saveHandState(handState)
	g.sendActionAck(playerMsg, handState.CurrentActionNum)

	crashtest.Hit(g.config.GameCode, crashtest.CrashPoint_WAIT_FOR_NEXT_ACTION_2, playerMsg.PlayerId)

	handState.FlowState = FlowState_PREPARE_NEXT_ACTION
	g.saveHandState(handState)
	err := g.prepareNextAction(handState, uint64(actedSeconds))
	if err != nil {
		return err
	}
	return nil
}

func validatePlayerAction(actionMsg *HandAction, handState *HandState) error {

	// if handState.GetNextSeatAction() == nil {
	// 	return fmt.Errorf("Invalid next seat action")
	// }

	if actionMsg.Action == ACTION_CALL {
		if handState.GetNextSeatAction() == nil {
			return fmt.Errorf("Invalid seat action")
		}
		expectedCallAmount := handState.GetNextSeatAction().CallAmount
		if actionMsg.Amount != expectedCallAmount {
			return fmt.Errorf("Invalid call amount %f. Expected amount: %f", actionMsg.Amount, expectedCallAmount)
		}
	}
	return nil
}

func (g *Game) prepareNextAction(handState *HandState, actionResponseTime uint64) error {
	expectedState := FlowState_PREPARE_NEXT_ACTION
	if handState.FlowState != expectedState {
		return fmt.Errorf("prepareNextAction called in wrong flow state. Expected state: %s, Actual state: %s", expectedState, handState.FlowState)
	}

	playerMsg := handState.ActionMsgInProgress
	if playerMsg == nil {
		errMsg := "Unable to get action message in progress. handState.ActionMsgInProgress is nil"
		channelGameLogger.Error().
			Uint32("club", g.config.ClubId).
			Str("game", g.config.GameCode).
			Msg(errMsg)
		return fmt.Errorf(errMsg)
	}

	actionMsg := g.getClientMsgItem(playerMsg)

	var allMsgItems []*HandMessageItem
	var msgItems []*HandMessageItem
	var err error
	err = handState.actionReceived(actionMsg.GetPlayerActed(), actionResponseTime)
	if err != nil {
		return errors.Wrap(err, "Error while updating handstate from action")
	}

	seatNo := actionMsg.GetPlayerActed().GetSeatNo()
	player := handState.PlayersInSeats[seatNo]

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

	// Send player's current stack to be updated in the UI
	playerAction := handState.PlayersActed[seatNo]
	if playerAction.State != PlayerActState_PLAYER_ACT_FOLDED {
		actionMsg.GetPlayerActed().Amount = playerAction.Amount
	} else {
		// the game folded this guy's hand
		actionMsg.GetPlayerActed().Action = ACTION_FOLD
		actionMsg.GetPlayerActed().Amount = 0
	}
	// broadcast this message to all the players (let everyone know this player acted)
	allMsgItems = append(allMsgItems, actionMsg)

	if handState.NoActiveSeats == 1 {
		handState.FlowState = FlowState_ONE_PLAYER_REMAINING
		msgItems, err = g.onePlayerRemaining(handState)
	} else if g.runItTwice(handState, playerAction) {
		// run it twice prompt
		handState.FlowState = FlowState_RUNITTWICE_UP_PROMPT
		msgItems, err = g.runItTwicePrompt(handState)
	} else if handState.isAllActivePlayersAllIn() || handState.allActionComplete() {
		handState.FlowState = FlowState_ALL_PLAYERS_ALL_IN
		msgItems, err = g.allPlayersAllIn(handState)
	} else if handState.CurrentState == HandStatus_SHOW_DOWN {
		handState.FlowState = FlowState_SHOWDOWN
		msgItems, err = g.showdown(handState)
	} else if handState.LastState != handState.CurrentState {
		handState.FlowState = FlowState_MOVE_TO_NEXT_ROUND
		msgItems, err = g.moveToNextRound(handState)
	} else {
		handState.FlowState = FlowState_MOVE_TO_NEXT_ACTION
		msgItems, err = g.moveToNextAction(handState)
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

	crashtest.Hit(g.config.GameCode, crashtest.CrashPoint_PREPARE_NEXT_ACTION_1, playerMsg.PlayerId)
	g.broadcastHandMessage(&serverMsg)
	handState.ActionMsgInProgress = nil

	crashtest.Hit(g.config.GameCode, crashtest.CrashPoint_PREPARE_NEXT_ACTION_2, playerMsg.PlayerId)
	g.saveHandState(handState)

	crashtest.Hit(g.config.GameCode, crashtest.CrashPoint_PREPARE_NEXT_ACTION_3, playerMsg.PlayerId)
	g.handleHandEnded(handState.TotalResultPauseTime, allMsgItems)
	return nil
}

func (g *Game) handleHandEnded(totalPauseTime uint32, allMsgItems []*HandMessageItem) {
	// if the last message is hand ended (pause for the result animation)
	handEnded := false
	for _, message := range allMsgItems {
		if message.MessageType == HandEnded {
			handEnded = true
		}
	}

	if handEnded {
		if totalPauseTime > 0 {
			fmt.Printf("Waiting for result animation\n")
			time.Sleep(time.Duration(totalPauseTime) * time.Millisecond)
			fmt.Printf("Waiting for result animation done\n")
		}
		gameMessage := &GameMessage{
			GameId:      g.config.GameId,
			MessageType: GameMoveToNextHand,
		}
		g.QueueGameMessage(gameMessage)
	}
}

func (g *Game) sendActionAck(playerMsg *HandMessage, currentActionNum uint32) {
	actionMsg := g.getClientMsgItem(playerMsg)
	if actionMsg.GetPlayerActed().GetTimedOut() {
		// Default action is generated by the server on action timeout. Don't acknowledge that.
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
		PlayerId:   playerMsg.GetPlayerId(),
		HandNum:    playerMsg.GetHandNum(),
		HandStatus: playerMsg.GetHandStatus(),
		SeatNo:     playerMsg.GetSeatNo(),
		MessageId:  g.generateMsgID("ACK", playerMsg.GetHandNum(), playerMsg.GetHandStatus(), playerMsg.GetPlayerId(), playerMsg.GetMessageId(), currentActionNum),
		Messages:   []*HandMessageItem{ack},
	}
	g.sendHandMessageToPlayer(serverMsg, playerMsg.GetPlayerId())
	channelGameLogger.Debug().
		Str("game", g.config.GameCode).
		Msg(fmt.Sprintf("Acknowledgment sent to %d. Message Id: %s", playerMsg.GetPlayerId(), playerMsg.GetMessageId()))
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
	channelGameLogger.Debug().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
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
	balance := make(map[uint32]float32)
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
	channelGameLogger.Debug().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
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

	balance := make(map[uint32]float32)
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
	channelGameLogger.Debug().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
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

	balance := make(map[uint32]float32)
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
	expectedState := FlowState_HAND_ENDED
	if handState.FlowState != expectedState {
		return nil, fmt.Errorf("handEnded called in wrong flow state. Expected state: %s, Actual state: %s", expectedState, handState.FlowState)
	}

	// wait 5 seconds to show the result
	// send a message to game to start new hand
	// if !util.GameServerEnvironment.ShouldDisableDelays() {
	// 	time.Sleep(time.Duration(g.delays.MoveToNextHand) * time.Millisecond)
	// }

	handEnded := &HandMessageItem{
		MessageType: HandEnded,
	}

	handState.FlowState = FlowState_MOVE_TO_NEXT_HAND

	return []*HandMessageItem{handEnded}, nil
}

// func (g *Game) sendResult(handState *HandState, saveResult *SaveHandResult, handResult *HandResult) ([]*HandMessageItem, error) {
// 	if saveResult != nil {
// 		if saveResult.HighHand != nil {
// 			// a player in this game hit a high hand
// 			handResult.HighHand = &HighHand{}
// 			handResult.HighHand.GameCode = saveResult.GameCode
// 			handResult.HighHand.HandNum = uint32(saveResult.HandNum)
// 			handResult.HighHand.Winners = make([]*HighHandWinner, 0)

// 			for _, winner := range saveResult.HighHand.Winners {
// 				playerSeatNo := 0

// 				winningPlayer, _ := strconv.ParseInt(winner.PlayerID, 10, 64)
// 				// get seat no
// 				for seatNo, playerID := range handState.ActiveSeats {
// 					if int64(playerID) == winningPlayer {
// 						playerSeatNo = seatNo
// 						break
// 					}
// 				}
// 				playerCards := make([]uint32, len(winner.PlayerCards))
// 				for i, card := range winner.PlayerCards {
// 					playerCards[i] = uint32(card)
// 				}
// 				hhCards := make([]uint32, len(winner.HhCards))
// 				for i, card := range winner.HhCards {
// 					hhCards[i] = uint32(card)
// 				}

// 				handResult.HighHand.Winners = append(handResult.HighHand.Winners, &HighHandWinner{
// 					PlayerId:    uint64(winningPlayer),
// 					PlayerName:  winner.PlayerName,
// 					PlayerCards: playerCards,
// 					HhCards:     hhCards,
// 					SeatNo:      uint32(playerSeatNo),
// 				})
// 			}

// 			if len(saveResult.HighHand.AssociatedGames) >= 1 {
// 				// announce the high hand to other games
// 				g.announceHighHand(saveResult, handResult.HighHand)
// 			}
// 		}
// 	}

// 	// update pause time
// 	if handResult.HandLog.RunItTwice {
// 		if handResult.HandLog.RunItTwiceResult.Board_1Winners != nil {
// 			for _, potWinner := range handResult.HandLog.RunItTwiceResult.Board_1Winners {
// 				potWinner.PauseTime = pauseTime
// 			}
// 			for _, potWinner := range handResult.HandLog.RunItTwiceResult.Board_2Winners {
// 				potWinner.PauseTime = pauseTime
// 			}
// 		}
// 	} else {
// 		for _, potWinner := range handResult.HandLog.PotWinners {
// 			potWinner.PauseTime = pauseTime
// 		}
// 	}

// 	msgItem := &HandMessageItem{
// 		MessageType: HandResultMessage,
// 		Content:     &HandMessageItem_HandResult{HandResult: handResult},
// 	}

// 	msgItems := make([]*HandMessageItem, 0)
// 	msgItems = append(msgItems, msgItem)
// 	return msgItems, nil
// }

func (g *Game) sendResult2(hs *HandState, handResultClient *HandResultClient) ([]*HandMessageItem, error) {
	//handResultClient.PlayerInfo = make(map[uint32]*PlayerHandInfo)
	hs.CurrentState = HandStatus_RESULT

	for seatNo, player := range hs.PlayersInSeats {
		if seatNo == 0 || !player.Inhand || player.OpenSeat {
			continue
		}

		before := float32(0.0)
		after := float32(0.0)
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
		rakePaid := float32(0.0)
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
	handResultClient.HandNum = hs.HandNum
	msgItem2 := &HandMessageItem{
		MessageType: HandResultMessage2,
		Content:     &HandMessageItem_HandResultClient{HandResultClient: handResultClient},
	}
	msgItems := make([]*HandMessageItem, 0)
	msgItems = append(msgItems, msgItem2)
	return msgItems, nil
}

func (g *Game) moveToNextRound(handState *HandState) ([]*HandMessageItem, error) {
	expectedState := FlowState_MOVE_TO_NEXT_ROUND
	if handState.FlowState != expectedState {
		return nil, fmt.Errorf("moveToNextRound called in wrong flow state. Expected state: %s, Actual state: %s", expectedState, handState.FlowState)
	}

	if handState.LastState == HandStatus_DEAL {
		// How do we get here?
		channelGameLogger.Warn().
			Uint32("club", g.config.ClubId).
			Str("game", g.config.GameCode).
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

	handState.FlowState = FlowState_MOVE_TO_NEXT_ACTION
	msgItems, err = g.moveToNextAction(handState)
	if err != nil {
		return nil, err
	}
	allMsgItems = append(allMsgItems, msgItems...)

	return allMsgItems, nil
}

func (g *Game) moveToNextAction(handState *HandState) ([]*HandMessageItem, error) {
	expectedState := FlowState_MOVE_TO_NEXT_ACTION
	if handState.FlowState != expectedState {
		return nil, fmt.Errorf("moveToNextAction called in wrong flow state. Expected state: %s, Actual state: %s", expectedState, handState.FlowState)
	}

	if handState.NextSeatAction == nil {
		return nil, fmt.Errorf("moveToNextAct called when handState.NextSeatAction == nil")
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
	actionTimesoutAt := time.Now().Add(time.Duration(g.config.ActionTime) * time.Second)
	handState.NextSeatAction.ActionTimesoutAt = actionTimesoutAt.Unix()
	g.resetTimer(handState.NextSeatAction.SeatNo, player.PlayerId, canCheck, actionTimesoutAt)
	allMsgItems = append(allMsgItems, yourActionMsg)

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

	nextActionMsg := &HandMessageItem{
		MessageType: HandNextAction,
		Content:     &HandMessageItem_ActionChange{ActionChange: actionChange},
	}

	allMsgItems = append(allMsgItems, nextActionMsg)

	handState.FlowState = FlowState_WAIT_FOR_NEXT_ACTION

	return allMsgItems, nil
}

func (g *Game) allPlayersAllIn(handState *HandState) ([]*HandMessageItem, error) {
	expectedState := FlowState_ALL_PLAYERS_ALL_IN
	if handState.FlowState != expectedState {
		return nil, fmt.Errorf("allPlayersAllIn called in wrong flow state. Expected state: %s, Actual state: %s", expectedState, handState.FlowState)
	}

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

	handState.FlowState = FlowState_SHOWDOWN
	msgItems, err = g.showdown(handState)
	if err != nil {
		return nil, err
	}
	allMsgItems = append(allMsgItems, msgItems...)
	return allMsgItems, nil
}

func (g *Game) showdown(handState *HandState) ([]*HandMessageItem, error) {
	expectedState := FlowState_SHOWDOWN
	if handState.FlowState != expectedState {
		return nil, fmt.Errorf("showdown called in wrong flow state. Expected state: %s, Actual state: %s", expectedState, handState.FlowState)
	}
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
	handState.FlowState = FlowState_HAND_ENDED

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
	expectedState := FlowState_ONE_PLAYER_REMAINING
	if handState.FlowState != expectedState {
		return nil, fmt.Errorf("onePlayerRemaining called in wrong flow state. Expected state: %s, Actual state: %s", expectedState, handState.FlowState)
	}
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

	handState.FlowState = FlowState_HAND_ENDED
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

	handResultProcessor := NewHandResultProcessor(handState, uint32(g.config.MaxPlayers), g.config.RewardTrackingIds)

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
	allMsgItems = append(allMsgItems, msgItems...)
	// send the hand to the database to store first
	// handResult := handResultProcessor.getResult(true /*db*/)
	// handResult.NoCards = g.NumCards(handState.GameType)
	// handResult.SmallBlind = handState.SmallBlind
	// handResult.BigBlind = handState.BigBlind
	// handResult.MaxPlayers = handState.MaxSeats

	sendResultToApi := !g.isScriptTest
	if sendResultToApi {
		handResultServer := &HandResultServer{
			GameId:     hs.GameId,
			HandNum:    hs.HandNum,
			GameType:   hs.GameType,
			ButtonPos:  hs.ButtonPos,
			NoCards:    g.NumCards(handState.GameType),
			HandLog:    hs.getLog(),
			HandStats:  hs.GetHandStats(),
			RunItTwice: hs.RunItTwiceConfirmed,
			SmallBlind: hs.SmallBlind,
			BigBlind:   hs.BigBlind,
			MaxPlayers: hs.MaxSeats,
			Result:     handResult2Client,
		}
		saveResult, _ := g.saveHandResult2ToAPIServer(handResultServer)
		if saveResult != nil {
			// retry here
		}
	}

	return allMsgItems, nil
}
