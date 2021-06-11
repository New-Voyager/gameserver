package game

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"voyager.com/server/crashtest"
	"voyager.com/server/poker"
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
			GameId:    g.config.GameId,
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
	currentHandState.PlayersActed = make(map[uint32]*PlayerActRound, 0)

	var playerSeatNo uint32
	for seatNo, pid := range handState.GetPlayersInSeats() {
		if pid == playerMsg.PlayerId {
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
		_, maskedCards := g.maskCards(handState.GetPlayersCards()[playerSeatNo], player.GameTokenInt)
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
		currentHandState.PlayersStack[uint64(seatNo)] = playerState[playerID].Stack
	}

	handStateMsg := &HandMessageItem{
		MessageType: HandQueryCurrentHand,
		Content:     &HandMessageItem_CurrentHandState{CurrentHandState: &currentHandState},
	}

	serverMsg := &HandMessage{
		ClubId:     g.config.ClubId,
		GameId:     g.config.GameId,
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
			playerID := handState.PlayersInSeats[handState.NextSeatAction.SeatNo]
			if handState.NextSeatAction.ActionTimesoutAt != 0 {
				g.resetTimer(handState.NextSeatAction.SeatNo, playerID, canCheck, actionExpiresAt)
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
	channelGameLogger.Info().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
		Uint32("player", messageSeatNo).
		Str("messageType", actionMsg.MessageType).
		Msg(fmt.Sprintf("%v", playerMsg))

	crashtest.Hit(g.config.GameCode, crashtest.CrashPoint_WAIT_FOR_NEXT_ACTION_1, playerMsg.PlayerId)

	if messageSeatNo == 0 && !RunningTests {
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
		if playerMsg.MessageId == "" && !RunningTests {
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

	// is it run it twice prompt response?
	if handState.RunItTwicePrompt {
		msgItems, err := g.runItTwiceConfirmation(handState, playerMsg)
		if err != nil {
			return err
		}
		// acknowledge so that player does not resend the message
		g.sendActionAck(playerMsg, handState.CurrentActionNum)

		msg := HandMessage{
			ClubId:     g.config.ClubId,
			GameId:     g.config.GameId,
			HandNum:    handState.HandNum,
			HandStatus: handState.CurrentState,
			MessageId:  g.generateMsgID("RIT_CONFIRM", handState.HandNum, handState.CurrentState, playerMsg.PlayerId, playerMsg.MessageId, handState.CurrentActionNum),
			Messages:   msgItems,
		}

		g.saveHandState(handState)
		g.broadcastHandMessage(&msg)
		g.handleHandEnded(msgItems)

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

	actionResponseTime := time.Now().Sub(g.actionTimeStart)
	actedSeconds := uint32(actionResponseTime.Seconds())
	if messageSeatNo == g.timerSeatNo {
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
	playerID := handState.PlayersInSeats[seatNo]

	if actionMsg.GetPlayerActed().GetTimedOut() {
		handState.PlayerStats[playerID].ConsecutiveActionTimeouts++
	} else {
		handState.PlayerStats[playerID].ConsecutiveActionTimeouts = 0
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
	} else if handState.isAllActivePlayersAllIn() {
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
	for _, m := range msgItems {
		allMsgItems = append(allMsgItems, m)
	}

	// Create hand message with all of the message items.
	serverMsg := HandMessage{
		ClubId:     g.config.ClubId,
		GameId:     g.config.GameId,
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
	g.handleHandEnded(allMsgItems)
	return nil
}

func (g *Game) handleHandEnded(allMsgItems []*HandMessageItem) {
	// if the last message is hand ended (pause for the result animation)
	handEnded := false
	var handResult *HandResult
	for _, message := range allMsgItems {
		if message.MessageType == HandEnded {
			handEnded = true
		}
	}

	// wait 5 seconds to show the result
	// send a message to game to start new hand
	if handEnded { //&& !util.GameServerEnvironment.ShouldDisableDelays() {
		for _, message := range allMsgItems {
			if message.MessageType == HandResultMessage {
				handResult = message.GetHandResult()
			}
		}
		totalPauseTime := 0
		if handResult == nil {
			totalPauseTime = int(pauseTime)
		} else {
			if handResult.RunItTwice {
				if handResult.HandLog.RunItTwiceResult.Board_1Winners != nil {
					for _, potWinner := range handResult.HandLog.RunItTwiceResult.Board_1Winners {
						totalPauseTime = totalPauseTime + int(potWinner.PauseTime)
					}
				}
				if handResult.HandLog.RunItTwiceResult.Board_2Winners != nil {
					for _, potWinner := range handResult.HandLog.RunItTwiceResult.Board_2Winners {
						totalPauseTime = totalPauseTime + int(potWinner.PauseTime)
					}
				}
			} else {
				for _, potWinner := range handResult.HandLog.PotWinners {
					totalPauseTime = totalPauseTime + int(potWinner.PauseTime)
				}
			}
		}
		now := time.Now()

		fmt.Printf("\n\n===============================\n[%s] Hand ended. Pausing the game. Pause: %d\n",
			now.String(), totalPauseTime)
		time.Sleep(time.Duration(totalPauseTime) * time.Millisecond)
		//time.Sleep(5000 * time.Millisecond)
		fmt.Printf("[%s] Resuming the game\n===============================\n\n", time.Now().String())

		gameMessage := &GameMessage{
			GameId:      g.config.GameId,
			MessageType: GameMoveToNextHand,
		}
		go g.SendGameMessageToChannel(gameMessage)
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
		ClubId:     playerMsg.GetClubId(),
		GameId:     playerMsg.GetGameId(),
		PlayerId:   playerMsg.GetPlayerId(),
		HandNum:    playerMsg.GetHandNum(),
		HandStatus: playerMsg.GetHandStatus(),
		SeatNo:     playerMsg.GetSeatNo(),
		MessageId:  g.generateMsgID("ACK", playerMsg.GetHandNum(), playerMsg.GetHandStatus(), playerMsg.GetPlayerId(), playerMsg.GetMessageId(), currentActionNum),
		Messages:   []*HandMessageItem{ack},
	}
	g.sendHandMessageToPlayer(serverMsg, playerMsg.GetPlayerId())
	channelGameLogger.Info().
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

func (g *Game) gotoFlop(handState *HandState) ([]*HandMessageItem, error) {
	channelGameLogger.Info().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
		Msg(fmt.Sprintf("Moving to %s", HandStatus_name[int32(handState.CurrentState)]))

	flopCards := make([]uint32, 3)
	for i, card := range handState.BoardCards[:3] {
		flopCards[i] = uint32(card)
	}
	handState.setupFlop()
	pots, seatsInPots := g.getPots(handState)
	balance := make(map[uint32]float32, 0)
	for seatNo, playerID := range handState.PlayersInSeats {
		if seatNo == 0 {
			continue
		}
		if playerState, ok := handState.PlayersState[playerID]; ok {
			balance[uint32(seatNo)] = playerState.Stack
		}
	}

	// update player stats
	for _, playerID := range handState.ActiveSeats {
		if playerID == 0 {
			continue
		}
		handState.PlayerStats[playerID].InFlop = true
	}

	cardsStr := poker.CardsToString(flopCards)
	flop := &Flop{Board: flopCards, CardsStr: cardsStr, Pots: pots, SeatsPots: seatsInPots, PlayerBalance: balance}
	msgItem := &HandMessageItem{
		MessageType: HandFlop,
		Content:     &HandMessageItem_Flop{Flop: flop},
	}

	return []*HandMessageItem{msgItem}, nil
}

func (g *Game) gotoTurn(handState *HandState) ([]*HandMessageItem, error) {
	channelGameLogger.Info().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
		Msg(fmt.Sprintf("Moving to %s", HandStatus_name[int32(handState.CurrentState)]))

	handState.setupTurn()

	cardsStr := poker.CardsToString(handState.BoardCards)

	boardCards := make([]uint32, 4)
	for i, card := range handState.BoardCards[:4] {
		boardCards[i] = uint32(card)
	}

	pots, seatsInPots := g.getPots(handState)

	balance := make(map[uint32]float32, 0)
	for seatNo, playerID := range handState.PlayersInSeats {
		if seatNo == 0 {
			continue
		}
		if playerState, ok := handState.PlayersState[playerID]; ok {
			balance[uint32(seatNo)] = playerState.Stack
		}
	}

	// update player stats
	for _, playerID := range handState.ActiveSeats {
		if playerID == 0 {
			continue
		}
		handState.PlayerStats[playerID].InTurn = true
	}

	turn := &Turn{Board: boardCards, TurnCard: boardCards[3],
		CardsStr: cardsStr, Pots: pots, SeatsPots: seatsInPots, PlayerBalance: balance}
	msgItem := &HandMessageItem{
		MessageType: HandTurn,
		Content:     &HandMessageItem_Turn{Turn: turn},
	}

	return []*HandMessageItem{msgItem}, nil
}

func (g *Game) gotoRiver(handState *HandState) ([]*HandMessageItem, error) {
	channelGameLogger.Info().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
		Msg(fmt.Sprintf("Moving to %s", HandStatus_name[int32(handState.CurrentState)]))

	handState.setupRiver()

	cardsStr := poker.CardsToString(handState.BoardCards)
	boardCards := make([]uint32, 5)
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
			balance[uint32(seatNo)] = playerState.Stack
		}
	}

	// update player stats
	for _, playerID := range handState.ActiveSeats {
		if playerID == 0 {
			continue
		}
		handState.PlayerStats[playerID].InRiver = true
	}
	river := &River{Board: boardCards, RiverCard: uint32(handState.BoardCards[4]),
		CardsStr: cardsStr, Pots: pots, SeatsPots: seatsInPots, PlayerBalance: balance}
	msgItem := &HandMessageItem{
		MessageType: HandRiver,
		Content:     &HandMessageItem_River{River: river},
	}

	return []*HandMessageItem{msgItem}, nil
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

func (g *Game) sendResult(handState *HandState, saveResult *SaveHandResult, handResult *HandResult) ([]*HandMessageItem, error) {
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

	// update pause time
	if handResult.HandLog.RunItTwice {
		if handResult.HandLog.RunItTwiceResult.Board_1Winners != nil {
			for _, potWinner := range handResult.HandLog.RunItTwiceResult.Board_1Winners {
				potWinner.PauseTime = pauseTime
			}
			for _, potWinner := range handResult.HandLog.RunItTwiceResult.Board_2Winners {
				potWinner.PauseTime = pauseTime
			}
		}
	} else {
		for _, potWinner := range handResult.HandLog.PotWinners {
			potWinner.PauseTime = pauseTime
		}
	}

	msgItem := &HandMessageItem{
		MessageType: HandResultMessage,
		Content:     &HandMessageItem_HandResult{HandResult: handResult},
	}

	return []*HandMessageItem{msgItem}, nil
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
	for _, m := range msgItems {
		allMsgItems = append(allMsgItems, m)
	}

	handState.FlowState = FlowState_MOVE_TO_NEXT_ACTION
	msgItems, err = g.moveToNextAction(handState)
	if err != nil {
		return nil, err
	}
	for _, m := range msgItems {
		allMsgItems = append(allMsgItems, m)
	}

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
	playerID := handState.PlayersInSeats[handState.NextSeatAction.SeatNo]
	actionTimesoutAt := time.Now().Add(time.Duration(g.config.ActionTime) * time.Second)
	handState.NextSeatAction.ActionTimesoutAt = actionTimesoutAt.Unix()
	g.resetTimer(handState.NextSeatAction.SeatNo, playerID, canCheck, actionTimesoutAt)
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
		for _, m := range msgItems {
			allMsgItems = append(allMsgItems, m)
		}
	}

	handState.FlowState = FlowState_SHOWDOWN
	msgItems, err = g.showdown(handState)
	if err != nil {
		return nil, err
	}
	for _, m := range msgItems {
		allMsgItems = append(allMsgItems, m)
	}

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

	var allMsgItems []*HandMessageItem
	var msgItems []*HandMessageItem
	var err error
	msgItems, err = g.generateAndSendResult(handState)
	if err != nil {
		return nil, err
	}
	for _, m := range msgItems {
		allMsgItems = append(allMsgItems, m)
	}

	msgItems, err = g.handEnded(handState)
	if err != nil {
		return nil, err
	}
	for _, m := range msgItems {
		allMsgItems = append(allMsgItems, m)
	}
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
	for _, m := range msgItems {
		allMsgItems = append(allMsgItems, m)
	}

	handState.FlowState = FlowState_HAND_ENDED
	msgItems, err = g.handEnded(handState)
	if err != nil {
		return nil, err
	}
	for _, m := range msgItems {
		allMsgItems = append(allMsgItems, m)
	}
	return allMsgItems, nil
}

func (g *Game) generateAndSendResult(handState *HandState) ([]*HandMessageItem, error) {
	handResultProcessor := NewHandResultProcessor(handState, uint32(g.config.MaxPlayers), g.config.RewardTrackingIds)

	// send the hand to the database to store first
	handResult := handResultProcessor.getResult(true /*db*/)
	handResult.NoCards = g.NumCards(handState.GameType)
	handResult.SmallBlind = handState.SmallBlind
	handResult.BigBlind = handState.BigBlind
	handResult.MaxPlayers = handState.MaxSeats

	saveResult, _ := g.saveHandResult(handResult)

	// send to all the players
	handResult = handResultProcessor.getResult(false /*db*/)
	handResult.NoCards = g.NumCards(handState.GameType)
	handResult.SmallBlind = handState.SmallBlind
	handResult.BigBlind = handState.BigBlind
	handResult.MaxPlayers = handState.MaxSeats

	// update the player balance
	for seatNo, player := range handResult.Players {
		g.PlayersInSeats[seatNo].Stack = player.Balance.After
	}

	msgItems, err := g.sendResult(handState, saveResult, handResult)
	if err != nil {
		return nil, err
	}

	return msgItems, nil
}
