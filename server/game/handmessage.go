package game

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"voyager.com/server/crashtest"
	"voyager.com/server/poker"
	"voyager.com/server/util"
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

func (g *Game) onQueryCurrentHand(message *HandMessage) error {
	// get hand state
	handState, err := g.loadHandState()
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
		NoCards:       g.NumCards(g.config.GameType),
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

func (g *Game) onPlayerActed(message *HandMessage, handState *HandState) error {
	if message == nil {
		// Game server is replaying this code after a crash.
		if handState.ActionMsgInProgress == nil {
			// There is no saved message. We crashed before saving the
			// message. We rely on the client to retry the message in this case.
			channelGameLogger.Info().
				Uint32("club", g.config.ClubId).
				Str("game", g.config.GameCode).
				Msg("Game server restarted with no saved action message. Relying on the client to resend the action.")
			return nil
		}
		channelGameLogger.Info().
			Uint32("club", g.config.ClubId).
			Str("game", g.config.GameCode).
			Msg("Restoring action message from hand state.")
		message = handState.ActionMsgInProgress
	}

	messageSeatNo := message.GetPlayerActed().GetSeatNo()
	channelGameLogger.Info().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
		Uint32("player", messageSeatNo).
		Str("message", message.MessageType).
		Msg(fmt.Sprintf("%v", message))

	crashtest.Hit(g.config.GameCode, crashtest.CrashPoint_WAIT_FOR_NEXT_ACTION_1)

	if messageSeatNo == 0 && !RunningTests {
		errMsg := fmt.Sprintf("Invalid seat number [%d] for player ID %d. Ignoring the action message.", messageSeatNo, message.PlayerId)
		channelGameLogger.Error().
			Uint32("club", g.config.ClubId).
			Str("game", g.config.GameCode).
			Msgf(errMsg)
		return fmt.Errorf(errMsg)
	}

	if handState.NextSeatAction != nil && (message.GetPlayerActed().GetSeatNo() != handState.NextSeatAction.SeatNo) {
		// Unexpected seat acted.
		// One scenario this can happen is when a player made a last-second action and the timeout
		// was triggered at the same time. We get two actions in that case - one last-minute action
		// from the player, and the other default action created by the timeout handler on behalf
		// of the player. We are discarding whichever action that came last in that case.
		errMsg := fmt.Sprintf("Invalid seat %d made action. Ignored. The next valid action seat is: %d",
			message.GetPlayerActed().GetSeatNo(), handState.NextSeatAction.SeatNo)
		channelGameLogger.Error().
			Str("game", g.config.GameCode).
			Uint32("hand", handState.GetHandNum()).
			Msg(errMsg)
		if !message.GetPlayerActed().GetTimedOut() {
			// Acknowledge so that the client stops retrying.
			g.acknowledgeMsg(message)
		}
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

	// is it run it twice prompt response?
	if handState.RunItTwicePrompt {
		g.runItTwiceConfirmation(handState, message)
		return nil
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

	expectedState := FlowState_WAIT_FOR_NEXT_ACTION
	if handState.FlowState != expectedState {
		errMsg := fmt.Sprintf("onPlayerActed called in wrong flow state. Ignoring message. Expected state: %s, Actual state: %s", expectedState, handState.FlowState)
		channelGameLogger.Error().
			Uint32("club", g.config.ClubId).
			Str("game", g.config.GameCode).
			Uint32("player", messageSeatNo).
			Str("message", message.MessageType).
			Msg(errMsg)
		return nil
	}

	if messageSeatNo == g.timerSeatNo {
		// cancel action timer
		g.pausePlayTimer(messageSeatNo)
	}

	handState.ActionMsgInProgress = message
	g.saveHandState(handState)

	g.acknowledgeMsg(message)

	crashtest.Hit(g.config.GameCode, crashtest.CrashPoint_WAIT_FOR_NEXT_ACTION_2)

	handState.FlowState = FlowState_PREPARE_NEXT_ACTION
	g.saveHandState(handState)
	g.prepareNextAction(handState)
	return nil
}

func (g *Game) prepareNextAction(handState *HandState) error {
	expectedState := FlowState_PREPARE_NEXT_ACTION
	if handState.FlowState != expectedState {
		return fmt.Errorf("prepareNextAction called in wrong flow state. Expected state: %s, Actual state: %s", expectedState, handState.FlowState)
	}

	message := handState.ActionMsgInProgress
	if message == nil {
		errMsg := "Unable to get action message in progress. handState.ActionMsgInProgress is nil"
		channelGameLogger.Error().
			Uint32("club", g.config.ClubId).
			Str("game", g.config.GameCode).
			Msg(errMsg)
		return fmt.Errorf(errMsg)
	}

	crashtest.Hit(g.config.GameCode, crashtest.CrashPoint_PREPARE_NEXT_ACTION_1)

	var err error
	err = handState.actionReceived(message.GetPlayerActed())
	if err != nil {
		return errors.Wrap(err, "Error while updating handstate from action")
	}

	if err != nil {
		// This is retryable (redis connection temporarily down?). Don't acknowledge and force the client to resend.
		return err
	}

	crashtest.Hit(g.config.GameCode, crashtest.CrashPoint_PREPARE_NEXT_ACTION_2)

	// Send player's current stack to be updated in the UI
	seatNo := message.GetPlayerActed().GetSeatNo()
	playerAction := handState.PlayersActed[seatNo]
	if playerAction.State != PlayerActState_PLAYER_ACT_FOLDED {
		message.GetPlayerActed().Amount = playerAction.Amount
	} else {
		// the game folded this guy's hand
		message.GetPlayerActed().Action = ACTION_FOLD
		message.GetPlayerActed().Amount = 0
	}
	message.HandNum = handState.HandNum
	// broadcast this message to all the players
	message.MessageId = 0
	g.broadcastHandMessage(message)

	crashtest.Hit(g.config.GameCode, crashtest.CrashPoint_PREPARE_NEXT_ACTION_3)

	if !util.GameServerEnvironment.ShouldDisableDelays() {
		time.Sleep(time.Duration(g.delays.PlayerActed) * time.Millisecond)
	}

	crashtest.Hit(g.config.GameCode, crashtest.CrashPoint_PREPARE_NEXT_ACTION_4)

	if handState.NoActiveSeats == 1 {
		handState.FlowState = FlowState_ONE_PLAYER_REMAINING
		g.saveHandState(handState)
		g.onePlayerRemaining(handState)
	} else if g.runItTwice(handState) {
		// run it twice prompt
		handState.FlowState = FlowState_RUNITTWICE_UP_PROMPT
		g.runItTwicePrompt(handState)
		g.saveHandState(handState)
	} else if handState.isAllActivePlayersAllIn() {
		handState.FlowState = FlowState_ALL_PLAYERS_ALL_IN
		g.saveHandState(handState)
		g.allPlayersAllIn(handState)
	} else if handState.CurrentState == HandStatus_SHOW_DOWN {
		handState.FlowState = FlowState_SHOWDOWN
		g.saveHandState(handState)
		g.showdown(handState)
	} else if handState.LastState != handState.CurrentState {
		handState.FlowState = FlowState_MOVE_TO_NEXT_ROUND
		g.saveHandState(handState)
		g.moveToNextRound(handState)
	} else {
		handState.FlowState = FlowState_MOVE_TO_NEXT_ACTION
		g.saveHandState(handState)
		g.moveToNextAction(handState)
	}

	handState.ActionMsgInProgress = nil

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

func (g *Game) gotoFlop(handState *HandState) {
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
	flopMessage := &Flop{Board: flopCards, CardsStr: cardsStr, Pots: pots, SeatsPots: seatsInPots, PlayerBalance: balance}
	handMessage := &HandMessage{ClubId: g.config.ClubId,
		GameId:      g.config.GameId,
		HandNum:     handState.HandNum,
		MessageType: HandFlop,
		HandStatus:  handState.CurrentState}
	handMessage.HandMessage = &HandMessage_Flop{Flop: flopMessage}
	g.broadcastHandMessage(handMessage)
	if !util.GameServerEnvironment.ShouldDisableDelays() {
		time.Sleep(time.Duration(g.delays.GoToFlop) * time.Millisecond)
	}
}

func (g *Game) gotoTurn(handState *HandState) {
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

	turnMessage := &Turn{Board: boardCards, TurnCard: boardCards[3],
		CardsStr: cardsStr, Pots: pots, SeatsPots: seatsInPots, PlayerBalance: balance}
	handMessage := &HandMessage{ClubId: g.config.ClubId,
		GameId:      g.config.GameId,
		HandNum:     handState.HandNum,
		MessageType: HandTurn,
		HandStatus:  handState.CurrentState}
	handMessage.HandMessage = &HandMessage_Turn{Turn: turnMessage}
	g.broadcastHandMessage(handMessage)
	if !util.GameServerEnvironment.ShouldDisableDelays() {
		time.Sleep(time.Duration(g.delays.GoToTurn) * time.Millisecond)
	}
}

func (g *Game) gotoRiver(handState *HandState) {
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
	riverMessage := &River{Board: boardCards, RiverCard: uint32(handState.BoardCards[4]),
		CardsStr: cardsStr, Pots: pots, SeatsPots: seatsInPots, PlayerBalance: balance}
	handMessage := &HandMessage{ClubId: g.config.ClubId,
		GameId:      g.config.GameId,
		HandNum:     handState.HandNum,
		MessageType: HandRiver,
		HandStatus:  handState.CurrentState}
	handMessage.HandMessage = &HandMessage_River{River: riverMessage}
	g.broadcastHandMessage(handMessage)
	if !util.GameServerEnvironment.ShouldDisableDelays() {
		time.Sleep(time.Duration(g.delays.GoToRiver) * time.Millisecond)
	}
}

func (g *Game) handEnded(handState *HandState) error {
	expectedState := FlowState_HAND_ENDED
	if handState.FlowState != expectedState {
		return fmt.Errorf("handEnded called in wrong flow state. Expected state: %s, Actual state: %s", expectedState, handState.FlowState)
	}

	// wait 5 seconds to show the result
	// send a message to game to start new hand
	if !util.GameServerEnvironment.ShouldDisableDelays() {
		time.Sleep(time.Duration(g.delays.MoveToNextHand) * time.Millisecond)
	}

	// broadcast hand ended
	handMessage := &HandMessage{
		ClubId:      g.config.ClubId,
		GameId:      g.config.GameId,
		HandNum:     handState.HandNum,
		MessageType: HandEnded,
	}
	g.broadcastHandMessage(handMessage)

	gameMessage := &GameMessage{
		GameId:      g.config.GameId,
		MessageType: GameMoveToNextHand,
	}

	handState.FlowState = FlowState_MOVE_TO_NEXT_HAND
	g.saveHandState(handState)

	go g.SendGameMessageToChannel(gameMessage)

	return nil
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

func (g *Game) moveToNextRound(handState *HandState) error {
	expectedState := FlowState_MOVE_TO_NEXT_ROUND
	if handState.FlowState != expectedState {
		return fmt.Errorf("moveToNextRound called in wrong flow state. Expected state: %s, Actual state: %s", expectedState, handState.FlowState)
	}

	if handState.LastState == HandStatus_DEAL {
		// How do we get here?
		return nil
	}

	// remove folded players from the pots
	handState.removeFoldedPlayersFromPots()

	if handState.LastState == HandStatus_PREFLOP && handState.CurrentState == HandStatus_FLOP {
		g.gotoFlop(handState)
	} else if handState.LastState == HandStatus_FLOP && handState.CurrentState == HandStatus_TURN {
		g.gotoTurn(handState)
	} else if handState.LastState == HandStatus_TURN && handState.CurrentState == HandStatus_RIVER {
		g.gotoRiver(handState)
	}

	handState.FlowState = FlowState_MOVE_TO_NEXT_ACTION
	g.saveHandState(handState)
	g.moveToNextAction(handState)

	return nil
}

func (g *Game) moveToNextAction(handState *HandState) error {
	expectedState := FlowState_MOVE_TO_NEXT_ACTION
	if handState.FlowState != expectedState {
		return fmt.Errorf("moveToNextAction called in wrong flow state. Expected state: %s, Actual state: %s", expectedState, handState.FlowState)
	}

	if handState.NextSeatAction == nil {
		return fmt.Errorf("moveToNextAct called when handState.NextSeatAction == nil")
	}

	crashtest.Hit(g.config.GameCode, crashtest.CrashPoint_MOVE_TO_NEXT_ACTION_1)

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

	crashtest.Hit(g.config.GameCode, crashtest.CrashPoint_MOVE_TO_NEXT_ACTION_2)

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

	crashtest.Hit(g.config.GameCode, crashtest.CrashPoint_MOVE_TO_NEXT_ACTION_3)

	handState.FlowState = FlowState_WAIT_FOR_NEXT_ACTION
	g.saveHandState(handState)

	crashtest.Hit(g.config.GameCode, crashtest.CrashPoint_MOVE_TO_NEXT_ACTION_4)

	return nil
}

func (g *Game) allPlayersAllIn(handState *HandState) error {
	expectedState := FlowState_ALL_PLAYERS_ALL_IN
	if handState.FlowState != expectedState {
		return fmt.Errorf("allPlayersAllIn called in wrong flow state. Expected state: %s, Actual state: %s", expectedState, handState.FlowState)
	}

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
			g.gotoFlop(handState)
			handState.CurrentState = HandStatus_TURN
		case HandStatus_TURN:
			g.gotoTurn(handState)
			handState.CurrentState = HandStatus_RIVER
		case HandStatus_RIVER:
			g.gotoRiver(handState)
			handState.CurrentState = HandStatus_SHOW_DOWN
		}
	}

	handState.FlowState = FlowState_SHOWDOWN
	g.saveHandState(handState)
	g.showdown(handState)

	return nil
}

func (g *Game) showdown(handState *HandState) error {
	expectedState := FlowState_SHOWDOWN
	if handState.FlowState != expectedState {
		return fmt.Errorf("showdown called in wrong flow state. Expected state: %s, Actual state: %s", expectedState, handState.FlowState)
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
	g.generateAndSendResult(handState)

	handState.FlowState = FlowState_HAND_ENDED
	g.saveHandState(handState)
	g.handEnded(handState)
	return nil
}

func (g *Game) onePlayerRemaining(handState *HandState) error {
	expectedState := FlowState_ONE_PLAYER_REMAINING
	if handState.FlowState != expectedState {
		return fmt.Errorf("onePlayerRemaining called in wrong flow state. Expected state: %s, Actual state: %s", expectedState, handState.FlowState)
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
	handState.CurrentState = HandStatus_HAND_CLOSED
	g.generateAndSendResult(handState)

	handState.FlowState = FlowState_HAND_ENDED
	g.saveHandState(handState)
	g.handEnded(handState)
	return nil
}

func (g *Game) generateAndSendResult(handState *HandState) error {
	handResultProcessor := NewHandResultProcessor(handState, uint32(g.config.MaxPlayers), g.config.RewardTrackingIds)

	// send the hand to the database to store first
	handResult := handResultProcessor.getResult(true /*db*/)
	saveResult, _ := g.saveHandResult(handResult)

	// send to all the players
	handResult = handResultProcessor.getResult(false /*db*/)

	// update the player balance
	for seatNo, player := range handResult.Players {
		g.PlayersInSeats[seatNo].Stack = player.Balance.After
	}

	g.sendResult(handState, saveResult, handResult)

	return nil
}
