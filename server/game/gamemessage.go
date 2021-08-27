package game

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"voyager.com/server/crashtest"
)

func (g *Game) handleGameMessage(message *GameMessage) {
	channelGameLogger.Trace().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
		Msg(fmt.Sprintf("Game message: %s. %v", message.MessageType, message))

	switch message.MessageType {
	// case GameStatusChanged:
	// 	g.onStatusChanged(message)

	case GameSetupNextHand:
		g.onNextHandSetup(message)

	case GameDealHand:
		g.onDealHand(message)

	case GameMoveToNextHand:
		g.onMoveToNextHand(message)

	// case GamePendingUpdatesDone:
	// 	g.onPendingUpdatesDone(message)

	case GameResume:
		g.onResume(message)

	case GetHandLog:
		g.onGetHandLog(message)

		// case GameCurrentStatus:
		// 	g.onStatusUpdate(message)

		// case PlayerConfigUpdateMsg:
		// 	g.onPlayerConfigUpdate(message)
	}
}

func (g *Game) onResume(message *GameMessage) error {
	return nil
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
}

func (g *Game) onGetHandLog(message *GameMessage) error {
	gameMessage := &GameMessage{
		GameId:      g.config.GameId,
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

func (g *Game) onPendingUpdatesDone(message *GameMessage) error {
	g.inProcessPendingUpdates = false
	if g.Status != GameStatus_ACTIVE || g.TableStatus != TableStatus_GAME_RUNNING {
		return nil
	}

	// deal next hand
	handState, err := g.loadHandState()
	if err != nil {
		return err
	}

	return g.moveAPIServerToNextHandAndScheduleDealHand(handState)
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

	// before we move to next hand, query API server whether we have any pending updates
	// if there are no pending updates, deal next hand

	// check any pending updates
	pendingUpdates, _ := anyPendingUpdates(g.apiServerURL, g.config.GameId, g.delays.PendingUpdatesRetry)
	if pendingUpdates {
		g.inProcessPendingUpdates = true
		go g.processPendingUpdates(g.apiServerURL, g.config.GameId, g.config.GameCode)
	} else {
		err := g.moveAPIServerToNextHandAndScheduleDealHand(handState)
		if err != nil {
			return err
		}
	}

	return nil
}

func (g *Game) moveAPIServerToNextHandAndScheduleDealHand(handState *HandState) error {
	err := g.moveAPIServerToNextHand(handState.HandNum)
	for err != nil {
		channelGameLogger.Error().Msg(err.Error())
		time.Sleep(5 * time.Second)
		err = g.moveAPIServerToNextHand(handState.HandNum)
	}

	handState.FlowState = FlowState_DEAL_HAND
	g.saveHandState(handState)

	gameMessage := &GameMessage{
		GameId:      g.config.GameId,
		MessageType: GameDealHand,
	}
	g.QueueGameMessage(gameMessage)
	return nil
}

// func (g *Game) onStatusChanged(message *GameMessage) error {
// 	gameStatusChanged := message.GetStatusChange()
// 	g.Status = gameStatusChanged.NewStatus
// 	return nil
// }

// func (g *Game) onStatusUpdate(message *GameMessage) error {
// 	gameStatusChanged := message.GetStatus()
// 	g.Status = gameStatusChanged.Status
// 	g.TableStatus = gameStatusChanged.TableStatus
// 	return nil
// }

func (g *Game) onNextHandSetup(message *GameMessage) error {
	nextHandSetup := message.GetNextHand()

	// Hand setup is persisted in Redis instead of stored in memory
	// so that we can continue with the same setup after crash during crash testing.
	g.handSetupPersist.Save(g.config.GameCode, nextHandSetup)
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
	gameMessage.ClubId = g.config.ClubId
	gameMessage.GameId = g.config.GameId
	gameMessage.MessageType = GameTableState
	gameMessage.GameMessage = &GameMessage_TableState{TableState: gameTableState}

	if *g.messageSender != nil {
		(*g.messageSender).BroadcastGameMessage(&gameMessage)
	}
	return nil
}

// func (g *Game) onPlayerConfigUpdate(message *GameMessage) error {
// 	updateMessage := message.GetPlayerConfigUpdate()
// 	playerConfig := g.playerConfig.Load().(map[uint64]PlayerConfigUpdate)
// 	playerConfig[updateMessage.PlayerId] = PlayerConfigUpdate{
// 		PlayerId:         updateMessage.PlayerId,
// 		MuckLosingHand:   updateMessage.MuckLosingHand,
// 		RunItTwicePrompt: updateMessage.RunItTwicePrompt,
// 	}
// 	g.playerConfig.Store(playerConfig)
// 	return nil
// }
