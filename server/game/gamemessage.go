package game

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/errors"
	"voyager.com/logging"
	"voyager.com/server/crashtest"
)

func (g *Game) handleGameMessage(message *GameMessage) error {
	g.logger.Trace().
		Msgf("Game message: %s. %v", message.MessageType, message)

	var err error
	switch message.MessageType {
	case GameSetupNextHand:
		err = g.onNextHandSetup(message)
		if err != nil {
			return errors.Wrap(err, "Could not setup next hand")
		}

	case GameDealHand:
		err = g.onDealHand(message)
		if err != nil {
			switch err.(type) {
			case NotReadyToDealError:
				// Wait for another resumeGame.
				g.logger.Error().Err(err).
					Str(logging.MsgTypeKey, message.MessageType).Msg("Ignoring game message")
				g.isHandInProgress = false
			default:
				return errors.Wrap(err, "Could not deal hand")
			}
		}

	case GameMoveToNextHand:
		var isPausedForPendingUpdates bool
		isPausedForPendingUpdates, err = g.onMoveToNextHand(message)
		if err != nil {
			switch err.(type) {
			case UnexpectedGameStatusError:
				// Wait for a resumeGame.
				g.logger.Error().Err(err).Msg("Not moving to next hand due to game status not being ready")
				g.isHandInProgress = false
			default:
				return errors.Wrap(err, "Could not move to next hand")
			}
		} else {
			if isPausedForPendingUpdates {
				// Wait for resumeGame once pending updates are done
				g.isHandInProgress = false
			}
		}

	case GameResume:
		if g.isHandInProgress {
			// Hand is already in progress. Don't try to restart.
			// We shouldn't really get here, but this is just to
			// handle potential error situation from api server
			// where it calls resumeGame multiple times.
			g.logger.Warn().
				Msgf("onResume called when hand is already in progress. Doing nothing.")
			break
		}

		g.isHandInProgress = true
		err = g.onResume(message)
		if err != nil {
			switch err.(type) {
			case UnexpectedGameStatusError:
				// Wait for another resumeGame.
				g.logger.Error().Err(err).Msg("Not moving to next hand due to game status not being ready")
				g.isHandInProgress = false
			default:
				return errors.Wrap(err, "Could not resume game")
			}
		}

	case GetHandLog:
		err = g.onGetHandLog(message)
		if err != nil {
			g.logger.Error().Err(err).Msg("Could not get hand log")
			// Considering this not fatal for now. Just log the error and move on.
		}

	case LeftGame:
		err = g.onPlayerLeftGame(message)
		if err != nil {
			g.logger.Error().Err(err).Msg("Could not handle player leaving game")
			// Considering this not fatal for now. Just log the error and move on.
		}
	}

	return nil
}

func (g *Game) onResume(message *GameMessage) error {
	var err error
	handState, err := g.loadHandState()
	if err != nil {
		if err != RedisKeyNotFound {
			return errors.Wrap(err, "Could not load hand state")
		}

		// There is no existing hand state. We should only get here during the initial
		// game start sequence (before the first hand was ever dealt).
		// We could also get here if the game server crashed before the first hand
		// or before the hand state of the first hand was persisted.
		// Go ahead and start the first hand.

		// Move the api server to the first hand (hand number 1).
		err = g.moveAPIServerToNextHandAndQueueDeal(nil)
		return err
	}

	g.logger.Debug().
		Msgf("Resuming game. Restarting hand at flow state [%s].", handState.FlowState)

	// We could be crash-restarting. Restore the encryption keys from the hand state.
	for playerID, encryptionKey := range handState.GetEncryptionKeys() {
		g.encryptionKeyCache.Add(playerID, encryptionKey)
	}

	switch handState.FlowState {
	case FlowState_DEAL_HAND:
		err = g.dealNewHand(nil)
	case FlowState_WAIT_FOR_NEXT_ACTION:
		err = g.onPlayerActed(nil, handState)
	case FlowState_PREPARE_NEXT_ACTION:
		err = g.prepareNextAction(handState, 0)
	case FlowState_MOVE_TO_NEXT_HAND:
		g.queueMoveToNextHand()
	case FlowState_WAIT_FOR_PENDING_UPDATE:
		err = g.moveAPIServerToNextHandAndQueueDeal(handState)
	default:
		err = fmt.Errorf("unhandled flow state in resumeGame: %s", handState.FlowState)
	}
	return err
}

func (g *Game) processPendingUpdates(apiServerURL string, gameID uint64, gameCode string, handNum uint32) {
	// call api server processPendingUpdates
	g.logger.Debug().
		Uint32(logging.HandNumKey, handNum).
		Msgf("Processing pending updates")
	url := fmt.Sprintf("%s/internal/process-pending-updates/gameId/%d", apiServerURL, gameID)

	retries := 0
	resp, err := http.Post(url, "application/json", nil)
	for err != nil && retries < int(g.maxRetries) {
		retries++
		g.logger.Error().Msgf("Error in post %s: %s. Retrying (%d/%d)", url, err, retries, g.maxRetries)
		time.Sleep(time.Duration(g.retryDelayMillis) * time.Millisecond)
		resp, err = http.Post(url, "application/json", nil)
	}

	// Server crashes right after pending updates were processed.
	// At this point pending updates were done and the server has
	// received the done message, but has not processed it.
	crashtest.Hit(gameCode, crashtest.CrashPoint_PENDING_UPDATES_1, 0)

	// if the api server returns nil, do nothing
	if resp == nil {
		return
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		g.logger.Error().Msgf("Failed to process pending updates. Error: %d", resp.StatusCode)
	}
}

func (g *Game) onGetHandLog(message *GameMessage) error {
	gameMessage := &GameMessage{
		GameId:      g.gameID,
		MessageType: GetHandLog,
		PlayerId:    message.PlayerId,
	}

	handState, err := g.loadHandState()
	if err != nil {
		if err == RedisKeyNotFound {
			go g.sendGameMessageToPlayer(gameMessage)
			return nil
		}
		return errors.Wrap(err, "Could not load hand state")
	}
	logData, err := json.Marshal(handState)
	if err != nil {
		return err
	}
	gameMessage.GameMessage = &GameMessage_HandLog{HandLog: logData}
	go g.sendGameMessageToPlayer(gameMessage)
	return nil
}

func (g *Game) onMoveToNextHand(message *GameMessage) (bool, error) {
	handState, err := g.loadHandState()
	if err != nil {
		return false, err
	}
	return g.moveToNextHand(handState)
}

func (g *Game) moveToNextHand(handState *HandState) (bool, error) {
	if handState == nil {
		return false, fmt.Errorf("Hand state is nil in moveToNextHand")
	}

	// for tournaments don't move to next hand, the tournament server is responsible to move to next hand
	if g.tournamentID != 0 {
		return false, nil
	}

	isPausedForPendingUpdates := false

	expectedState := FlowState_MOVE_TO_NEXT_HAND
	if handState.FlowState != expectedState {
		return isPausedForPendingUpdates, fmt.Errorf("moveToNextHand called in wrong flow state. Expected state: %s, Actual state: %s", expectedState, handState.FlowState)
	}

	// if this game is used by script test, don't look for pending updates
	if g.isScriptTest {
		return isPausedForPendingUpdates, nil
	}

	// before we move to next hand, query API server whether we have any pending updates
	// if there are no pending updates, deal next hand

	// check any pending updates
	pendingUpdates, _ := g.anyPendingUpdates(g.apiServerURL, g.gameID, g.delays.PendingUpdatesRetry)
	if pendingUpdates {
		go g.processPendingUpdates(g.apiServerURL, g.gameID, g.gameCode, handState.GetHandNum())
		crashtest.Hit(g.gameCode, crashtest.CrashPoint_PENDING_UPDATES_2, 0)
		err := g.saveHandState(handState, FlowState_WAIT_FOR_PENDING_UPDATE)
		if err != nil {
			msg := "Could not save hand state after requesting pending update"
			g.logger.Error().
				Uint32(logging.HandNumKey, handState.GetHandNum()).
				Err(err).
				Msgf(msg)
			return isPausedForPendingUpdates, errors.Wrap(err, msg)
		}
		// We pause the game here and wait for the api server.
		// We'll get a rest call (resume) from the api server once it completes
		// the pending update.
		isPausedForPendingUpdates = true
	} else {
		// No pending updates. Move straight on to the next hand.
		err := g.moveAPIServerToNextHandAndQueueDeal(handState)
		return isPausedForPendingUpdates, err
	}

	return isPausedForPendingUpdates, nil
}

func (g *Game) moveAPIServerToNextHandAndQueueDeal(handState *HandState) error {
	if g.tournamentID != 0 {
		// don't move for tournaments
		return nil
	}
	var currentHandNum uint32
	if handState == nil {
		currentHandNum = 0
	} else {
		currentHandNum = handState.HandNum
	}
	resp, err := g.moveAPIServerToNextHand(currentHandNum)
	if err != nil {
		return errors.Wrap(err, "Could not move api server to next hand")
	}

	if resp.HandNum == 0 {
		return fmt.Errorf("Received next hand number = 0 from api server")
	}

	if resp.GameStatus != GameStatus_ACTIVE || resp.TableStatus != TableStatus_GAME_RUNNING {
		return UnexpectedGameStatusError{
			GameStatus:  resp.GameStatus,
			TableStatus: resp.TableStatus,
		}
	}

	crashtest.Hit(g.gameCode, crashtest.CrashPoint_PENDING_UPDATES_3, 0)

	if handState != nil {
		err = g.saveHandState(handState, FlowState_DEAL_HAND)
		if err != nil {
			msg := fmt.Sprintf("Could not save hand state before moving to next hand")
			g.logger.Error().
				Uint32(logging.HandNumKey, handState.GetHandNum()).
				Err(err).
				Msgf(msg)
			return errors.Wrap(err, msg)
		}
	}

	gameMessage := &GameMessage{
		GameId:      g.gameID,
		MessageType: GameDealHand,
	}
	g.QueueGameMessage(gameMessage)
	return nil
}

func (g *Game) onNextHandSetup(message *GameMessage) error {
	nextHandSetup := message.GetNextHand()

	// Hand setup is persisted in Redis instead of stored in memory
	// so that we can continue with the same setup after crash during crash testing.
	g.handSetupPersist.Save(g.gameCode, nextHandSetup)
	return nil
}

func (g *Game) onDealHand(message *GameMessage) error {
	return g.dealNewHand(nil)
}

func (g *Game) broadcastTableState() error {
	// get active players on the table
	playersAtTable, err := g.getPlayersAtTable()
	if err != nil {
		return err
	}

	gameTableState := &TestGameTableStateMessage{PlayersState: playersAtTable}
	var gameMessage GameMessage
	gameMessage.GameId = g.gameID
	gameMessage.MessageType = GameTableState
	gameMessage.GameMessage = &GameMessage_TableState{TableState: gameTableState}

	if *g.messageSender != nil {
		(*g.messageSender).BroadcastGameMessage(&gameMessage, false)
	}
	return nil
}

func (g *Game) onPlayerLeftGame(message *GameMessage) error {
	handState, err := g.loadHandState()
	if err != nil {
		return errors.Wrap(err, "Could not load hand state")
	}
	handState.playerLeftGame(message.PlayerId)
	err = g.saveHandState(handState, handState.GetFlowState())
	if err != nil {
		msg := "Could not save hand state when recording a player who left the game"
		g.logger.Error().
			Err(err).
			Uint32(logging.HandNumKey, handState.GetHandNum()).
			Uint64(logging.PlayerIDKey, message.PlayerId).
			Msgf(msg)
		return err
	}
	return nil
}
