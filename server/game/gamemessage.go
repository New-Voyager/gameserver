package game

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"
	"voyager.com/server/crashtest"
)

func (g *Game) handleGameMessage(message *GameMessage) {
	channelGameLogger.Trace().
		Str("game", g.gameCode).
		Msgf("Game message: %s. %v", message.MessageType, message)

	var err error
	switch message.MessageType {
	case GameSetupNextHand:
		err = g.onNextHandSetup(message)

	case GameDealHand:
		err = g.onDealHand(message)

	case GameMoveToNextHand:
		err = g.onMoveToNextHand(message)

	case GameResume:
		err = g.onResume(message)

	case GetHandLog:
		err = g.onGetHandLog(message)
	}

	if err != nil {
		err = errors.Wrapf(err, "Error while handling %s", message.MessageType)

		// TODO: Just logging for now but should do more in some of these cases.
		channelGameLogger.Error().
			Str("game", g.gameCode).
			Msg(err.Error())
	}
}

func (g *Game) onResume(message *GameMessage) error {
	var err error

	handState, err := g.loadHandState()
	if err != nil {
		if handState == nil {
			// There is no existing hand state. We get here during the initial game start sequence.
			// We could also get here if the game server crashed before the first hand
			// or before the hand state of the first hand was persisted.
			// Go ahead and start the first hand.
			err = g.moveAPIServerToNextHandAndScheduleDealHand(nil)
			if err != nil {
				return errors.Wrap(err, "Error while starting game")
			}
			return nil
		}

		return errors.Wrap(err, "Could not load hand state while resuming game")
	}

	channelGameLogger.Info().
		Str("game", g.gameCode).
		Msgf("Resuming game. Restarting hand at flow state [%s].", handState.FlowState)

	switch handState.FlowState {
	case FlowState_DEAL_HAND:
		err = g.dealNewHand()
	case FlowState_WAIT_FOR_NEXT_ACTION:
		err = g.onPlayerActed(nil, handState)
	case FlowState_PREPARE_NEXT_ACTION:
		err = g.prepareNextAction(handState, 0)
	case FlowState_MOVE_TO_NEXT_HAND:
		err = g.moveToNextHand(handState)
	case FlowState_WAIT_FOR_PENDING_UPDATE:
		// TODO: Do we need this?
		g.inProcessPendingUpdates = false
		err = g.moveAPIServerToNextHandAndScheduleDealHand(handState)
	default:
		err = fmt.Errorf("unhandled flow state in resumeGame: %s", handState.FlowState)
	}
	return err
}

func (g *Game) processPendingUpdates(apiServerURL string, gameID uint64, gameCode string) {
	// call api server processPendingUpdates
	channelGameLogger.Debug().Msgf("Processing pending updates for the game %d", gameID)
	url := fmt.Sprintf("%s/internal/process-pending-updates/gameId/%d", apiServerURL, gameID)

	retries := 0
	resp, err := http.Post(url, "application/json", nil)
	for err != nil && retries < int(g.maxRetries) {
		retries++
		channelGameLogger.Error().Msgf("Error in post %s: %s. Retrying (%d/%d)", url, err, retries, g.maxRetries)
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
		channelGameLogger.Fatal().Uint64("game", gameID).Msgf("Failed to process pending updates. Error: %d", resp.StatusCode)
	}

	// remove hand state
	g.removeHandState()
}

func (g *Game) onGetHandLog(message *GameMessage) error {
	gameMessage := &GameMessage{
		GameId:      g.gameID,
		MessageType: GetHandLog,
		PlayerId:    message.PlayerId,
	}

	handState, err := g.loadHandState()
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			go g.sendGameMessageToPlayer(gameMessage)
			return nil
		}
		return err
	}
	logData, err := json.Marshal(handState)
	if err != nil {
		return err
	}
	gameMessage.GameMessage = &GameMessage_HandLog{HandLog: logData}
	go g.sendGameMessageToPlayer(gameMessage)
	return nil
}

func (g *Game) onMoveToNextHand(message *GameMessage) error {
	handState, err := g.loadHandState()
	if err != nil {
		return err
	}
	return g.moveToNextHand(handState)
}

func (g *Game) moveToNextHand(handState *HandState) error {
	expectedState := FlowState_MOVE_TO_NEXT_HAND
	if handState.FlowState != expectedState {
		return fmt.Errorf("moveToNextHand called in wrong flow state. Expected state: %s, Actual state: %s", expectedState, handState.FlowState)
	}

	// if this game is used by script test, don't look for pending updates
	if g.isScriptTest {
		return nil
	}

	if g.inProcessPendingUpdates {
		return nil
	}

	// before we move tmoveToNextHando next hand, query API server whether we have any pending updates
	// if there are no pending updates, deal next hand

	// check any pending updates
	pendingUpdates, _ := anyPendingUpdates(g.apiServerURL, g.gameID, g.delays.PendingUpdatesRetry)
	if pendingUpdates {
		g.inProcessPendingUpdates = true
		go g.processPendingUpdates(g.apiServerURL, g.gameID, g.gameCode)
		handState.FlowState = FlowState_WAIT_FOR_PENDING_UPDATE
		g.saveHandState(handState)
	} else {
		err := g.moveAPIServerToNextHandAndScheduleDealHand(handState)
		if err != nil {
			return err
		}
	}

	return nil
}

func (g *Game) moveAPIServerToNextHandAndScheduleDealHand(handState *HandState) error {
	var handNum uint32
	if handState == nil {
		handNum = 0
	} else {
		handNum = handState.HandNum
	}
	err := g.moveAPIServerToNextHand(handNum)
	if err != nil {
		return errors.Wrap(err, "Could not move api server to next hand")
	}

	if handState != nil {
		handState.FlowState = FlowState_DEAL_HAND
		g.saveHandState(handState)
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
	err := g.dealNewHand()
	return err
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
		(*g.messageSender).BroadcastGameMessage(&gameMessage)
	}
	return nil
}
