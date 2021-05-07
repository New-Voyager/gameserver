package game

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"voyager.com/server/crashtest"
	"voyager.com/server/poker"
	"voyager.com/server/util"
)

func (g *Game) handleGameMessage(message *GameMessage) {
	channelGameLogger.Trace().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
		Msg(fmt.Sprintf("Game message: %s. %v", message.MessageType, message))

	switch message.MessageType {
	case GameStatusChanged:
		g.onStatusChanged(message)

	case GameSetupNextHand:
		g.onNextHandSetup(message)

	case GameDealHand:
		g.onDealHand(message)

	case GameJoin:
		g.onJoinGame(message)

	case GameMoveToNextHand:
		g.onMoveToNextHand(message)

	case GamePendingUpdatesDone:
		g.onPendingUpdatesDone(message)

	case GetHandLog:
		g.onGetHandLog(message)

	case GameStart:
		break

	case PlayerConfigUpdateMsg:
		g.onPlayerConfigUpdate(message)
	}
}

func processPendingUpdates(apiServerURL string, gameID uint64) {
	// call api server processPendingUpdates
	channelGameLogger.Info().Msgf("Processing pending updates for the game %d", gameID)
	url := fmt.Sprintf("%s/internal/process-pending-updates/gameId/%d", apiServerURL, gameID)
	resp, _ := http.Post(url, "application/json", nil)

	// if the api server returns nil, do nothing
	if resp == nil {
		return
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		channelGameLogger.Fatal().Uint64("game", gameID).Msg(fmt.Sprintf("Failed to process pending updates. Error: %d", resp.StatusCode))
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
			go g.sendGameMessageToReceiver(gameMessage)
			return nil
		}
		return err
	}
	logData, err := json.Marshal(handState)
	if err != nil {
		return err
	}
	gameMessage.GameMessage = &GameMessage_HandLog{HandLog: logData}
	go g.sendGameMessageToReceiver(gameMessage)
	return nil
}

func (g *Game) onPendingUpdatesDone(message *GameMessage) error {
	g.inProcessPendingUpdates = false
	// move to next hand
	if g.Status == GameStatus_ACTIVE && g.TableStatus == TableStatus_GAME_RUNNING {
		// deal next hand
		handState, err := g.loadHandState()
		if err != nil {
			return err
		}
		handState.FlowState = FlowState_DEAL_HAND
		g.saveHandState(handState)

		gameMessage := &GameMessage{
			GameId:      g.config.GameId,
			MessageType: GameDealHand,
		}
		go g.SendGameMessageToChannel(gameMessage)
	}
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

	crashtest.Hit(g.config.GameCode, crashtest.CrashPoint_MOVE_TO_NEXT_HAND_1, 0)

	if !util.GameServerEnvironment.ShouldDisableDelays() {
		time.Sleep(time.Duration(g.delays.OnMoveToNextHand) * time.Millisecond)
	}

	// if this game is used by script test, don't look for pending updates
	if g.scriptTest {
		return nil
	}

	if g.inProcessPendingUpdates {
		channelGameLogger.Info().Msgf("******* Processing pending updates. How did we get here?")
		return nil
	}

	// before we move to next hand, query API server whether we have any pending updates
	// if there are no pending updates, deal next hand

	// check any pending updates
	pendingUpdates, _ := anyPendingUpdates(g.apiServerUrl, g.config.GameId, g.delays.PendingUpdatesRetry)
	if pendingUpdates {
		g.inProcessPendingUpdates = true
		go processPendingUpdates(g.apiServerUrl, g.config.GameId)
	} else {
		handState.FlowState = FlowState_DEAL_HAND
		g.saveHandState(handState)

		gameMessage := &GameMessage{
			GameId:      g.config.GameId,
			MessageType: GameDealHand,
		}
		crashtest.Hit(g.config.GameCode, crashtest.CrashPoint_MOVE_TO_NEXT_HAND_3, 0)
		go g.SendGameMessageToChannel(gameMessage)
		crashtest.Hit(g.config.GameCode, crashtest.CrashPoint_MOVE_TO_NEXT_HAND_4, 0)
	}

	return nil
}

func (g *Game) onStatusChanged(message *GameMessage) error {
	gameStatusChanged := message.GetStatusChange()
	g.Status = gameStatusChanged.NewStatus
	return nil
}

func (g *Game) onNextHandSetup(message *GameMessage) error {
	setupNextHand := message.GetNextHand()

	if setupNextHand.ButtonPos != 0 {
		g.ButtonPos = setupNextHand.ButtonPos
	}

	g.testButtonPos = int32(setupNextHand.ButtonPos)
	g.testDeckToUse = nil
	if setupNextHand.Deck != nil {
		g.testDeckToUse = poker.DeckFromBytes(setupNextHand.Deck)
	} else {
		g.testDeckToUse = poker.NewDeck(nil)
	}
	g.pauseBeforeNextHand = setupNextHand.Pause

	// Also persist the next deck in Redis to enable crash testing between hands.
	t := TestHandsSetup{
		Hands: []*TestHandSetup{
			{
				Deck:      setupNextHand.Deck,
				ButtonPos: setupNextHand.ButtonPos,
				Pause:     setupNextHand.Pause,
				AutoDeal:  setupNextHand.AutoDeal,
			},
		},
	}
	g.handSetupPersist.Save(g.config.GameCode, &t)
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

	gameTableState := &GameTableStateMessage{PlayersState: playersAtTable, Status: g.Status, TableStatus: g.TableStatus}
	var gameMessage GameMessage
	gameMessage.ClubId = g.config.ClubId
	gameMessage.GameId = g.config.GameId
	gameMessage.MessageType = GameTableState
	gameMessage.GameMessage = &GameMessage_TableState{TableState: gameTableState}

	if *g.messageReceiver != nil {
		(*g.messageReceiver).BroadcastGameMessage(&gameMessage)
	}
	return nil
}

func (g *Game) onJoinGame(message *GameMessage) error {
	joinMessage := message.GetJoinGame()
	g.players[joinMessage.PlayerId] = joinMessage.Name
	return nil
}

func (g *Game) onPlayerConfigUpdate(message *GameMessage) error {
	updateMessage := message.GetPlayerConfigUpdate()
	playerConfig := g.playerConfig.Load().(map[uint64]PlayerConfigUpdate)
	playerConfig[updateMessage.PlayerId] = PlayerConfigUpdate{
		PlayerId:         updateMessage.PlayerId,
		MuckLosingHand:   updateMessage.MuckLosingHand,
		RunItTwicePrompt: updateMessage.RunItTwicePrompt,
	}
	g.playerConfig.Store(playerConfig)
	return nil
}
