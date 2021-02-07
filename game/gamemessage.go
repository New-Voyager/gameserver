package game

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"voyager.com/server/poker"
)

func (g *Game) handleGameMessage(message *GameMessage) {
	channelGameLogger.Trace().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
		Msg(fmt.Sprintf("Game message: %s. %v", message.MessageType, message))

	switch message.MessageType {
	case GamePlayerInSeats:
		g.onPlayerInSeats(message)

	case GameStatusChanged:
		g.onStatusChanged(message)

	case GameSetupNextHand:
		g.onNextHandSetup(message)

	case GameDealHand:
		g.onDealHand(message)

	case GameJoin:
		g.onJoinGame(message)

	// case PlayerUpdate:
	// 	g.onPlayerUpdate(message)

	case GameMoveToNextHand:
		g.onMoveToNextHand(message)

	case GamePendingUpdatesDone:
		g.onPendingUpdatesDone(message)

	case GetHandLog:
		g.onGetHandLog(message)

	case GameStart:
		break
	}
}

func processPendingUpdates(apiServerUrl string, gameID uint64) {
	// call api server processPendingUpdates
	channelGameLogger.Info().Msgf("Processing pending updates for the game %d", gameID)
	url := fmt.Sprintf("%s/internal/process-pending-updates/gameId/%d", apiServerUrl, gameID)
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
		gameMessage := &GameMessage{
			GameId:      g.config.GameId,
			MessageType: GameDealHand,
		}
		go g.SendGameMessageToChannel(gameMessage)
	}
	return nil
}

func (g *Game) onMoveToNextHand(message *GameMessage) error {

	if !RunningTests {
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
		gameMessage := &GameMessage{
			GameId:      g.config.GameId,
			MessageType: GameDealHand,
		}
		go g.SendGameMessageToChannel(gameMessage)
	}

	return nil
}

// This is only for testing.
func (g *Game) onPlayerInSeats(message *GameMessage) error {
	g.PlayersInSeats = make([]SeatPlayer, g.config.MaxPlayers)
	for _, player := range message.GetPlayersInSeats().GetPlayerInSeats() {
		seatNo := player.SeatNo
		g.PlayersInSeats[seatNo-1] = SeatPlayer{
			SeatNo:   seatNo,
			OpenSeat: false,
			PlayerID: player.PlayerId,
			Name:     player.Name,
			BuyIn:    player.BuyIn,
			Stack:    player.BuyIn,
			Status:   PlayerStatus_PLAYING,
		}
	}
	return nil
}

// func (g *Game) onPlayerTakeSeat(message *GameMessage) error {
// 	gameSit := message.GetTakeSeat()

// 	if gameSit.SeatNo < 1 || gameSit.SeatNo > uint32(g.config.MaxPlayers) {
// 		channelGameLogger.Error().
// 			Uint32("club", g.config.ClubId).
// 			Str("game", g.config.GameCode).
// 			Str("message", "GameSitMessage").
// 			Msg(fmt.Sprintf("Invalid seat no: %d Allowed values from 1-%d", gameSit.SeatNo, g.config.MaxPlayers))
// 		return fmt.Errorf("Invalid seat no: %d Allowed values from 1-%d", gameSit.SeatNo, g.config.MaxPlayers)
// 	}

// 	playersInSeat := g.PlayersInSeats
// 	if playersInSeat[gameSit.SeatNo-1].Status == PlayerStatus_PLAYING {
// 		// there is already a player in the seat
// 		channelGameLogger.Error().
// 			Uint32("club", g.config.ClubId).
// 			Str("game", g.config.GameCode).
// 			Str("message", "GameSitMessage").
// 			Msg(fmt.Sprintf("A player is already sitting in the seat: %d", gameSit.SeatNo))
// 		return fmt.Errorf("A player is already sitting in the seat: %d", gameSit.SeatNo)
// 	}

// 	channelGameLogger.Info().
// 		Uint32("club", g.config.ClubId).
// 		Str("game", g.config.GameCode).
// 		Uint64("player", gameSit.PlayerId).
// 		Str("message", "GameSitMessage").
// 		Msg(fmt.Sprintf("Player %d took %d seat, buy-in: %f", gameSit.PlayerId, gameSit.SeatNo, gameSit.BuyIn))

// 	g.PlayersInSeats[gameSit.SeatNo-1] = gameSit.PlayerId

// 	if g.state.PlayersState == nil {
// 		g.state.PlayersState = make(map[uint64]*PlayerState)
// 	}
// 	g.state.PlayersState[gameSit.PlayerId] = &PlayerState{BuyIn: gameSit.BuyIn, CurrentBalance: gameSit.BuyIn, Status: PlayerStatus_PLAYING}

// 	// send player sat message to all
// 	playerSatMessage := GamePlayerSatMessage{SeatNo: gameSit.SeatNo, BuyIn: gameSit.BuyIn, PlayerId: gameSit.PlayerId}
// 	gameMessage := GameMessage{MessageType: PlayerSat, ClubId: message.ClubId, GameId: message.GameId}
// 	gameMessage.GameMessage = &GameMessage_PlayerSat{PlayerSat: &playerSatMessage}
// 	g.broadcastGameMessage(&gameMessage)
// 	return nil
// }

func (g *Game) onStatusChanged(message *GameMessage) error {
	gameStatusChanged := message.GetStatusChange()
	g.Status = gameStatusChanged.NewStatus

	if g.Status == GameStatus_ACTIVE {
		g.startGame()
	}

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

// TODO: Clean up this function if not needed.
//
// func (g *Game) onPlayerUpdate(message *GameMessage) error {
// 	playerUpdate := message.GetPlayerUpdate()
// 	channelGameLogger.Info().Msg(fmt.Sprintf("Player update: %v", playerUpdate))

// 	if playerUpdate.Status == PlayerStatus_KICKED_OUT ||
// 		playerUpdate.Status == PlayerStatus_LEFT {
// 		// the player is out of the game
// 		for i, player := range g.PlayersInSeats {
// 			if player.PlayerID == playerUpdate.PlayerId {
// 				// this is the seat no where player was sitting
// 				g.state.PlayersInSeats[i] = 0
// 			}
// 		}
// 	} else if playerUpdate.Status == PlayerStatus_IN_BREAK {
// 		g.state.PlayersState[playerUpdate.PlayerId].Status = playerUpdate.Status
// 	} else {
// 		// we can only update the players that are in seats
// 		if playerUpdate.SeatNo == 0 {
// 			channelGameLogger.Error().Msg(fmt.Sprintf("Player update: SeatNo cannot be empty. %+v", playerUpdate))
// 			return fmt.Errorf("SeatNo cannot be empty")
// 		}
// 		// check to see if the player switched seat
// 		for seatNo, playerID := range g.state.PlayersInSeats {
// 			if playerID == playerUpdate.PlayerId &&
// 				uint32(seatNo) != playerUpdate.SeatNo {
// 				// this player switch seat
// 				channelGameLogger.Error().Msgf("Player %d switched seat from %d to %d", playerID, seatNo, playerUpdate.SeatNo)
// 				g.state.PlayersInSeats[seatNo] = 0
// 				message.GetPlayerUpdate().OldSeat = uint32(seatNo)
// 				break
// 			}
// 		}

// 		// buyin/reload/sitting in the table
// 		g.state.PlayersInSeats[playerUpdate.SeatNo] = playerUpdate.PlayerId
// 		var tokenInt uint64
// 		if playerUpdate.GameToken != "" {
// 			// pad here 000000
// 			gameToken := fmt.Sprintf("000000%s", playerUpdate.GameToken)
// 			token, _ := hex.DecodeString(gameToken)
// 			tokenInt = binary.LittleEndian.Uint64(token)
// 			channelGameLogger.Info().
// 				Str("game", g.config.GameCode).
// 				Uint64("player", playerUpdate.PlayerId).
// 				Str("message", "GameSitMessage").
// 				Msgf("Player %d took %d seat, buy-in: %f, gameToken: %s tokenXorKey: %X",
// 					playerUpdate.PlayerId, playerUpdate.SeatNo, playerUpdate.BuyIn, playerUpdate.GameToken, tokenInt)
// 		} else {
// 			channelGameLogger.Info().
// 				Str("game", g.config.GameCode).
// 				Uint64("player", playerUpdate.PlayerId).
// 				Str("message", "GameSitMessage").
// 				Msgf("Player %d took %d seat, buy-in: %f",
// 					playerUpdate.PlayerId, playerUpdate.SeatNo, playerUpdate.BuyIn)
// 		}
// 		if playerUpdate.GameToken != "" {
// 			g.state.PlayersState[playerUpdate.PlayerId] = &PlayerState{BuyIn: playerUpdate.BuyIn,
// 				CurrentBalance: playerUpdate.Stack,
// 				Status:         playerUpdate.Status,
// 				GameToken:      playerUpdate.GameToken,
// 				GameTokenInt:   tokenInt}
// 		} else {
// 			var gameToken string
// 			var gameTokenInt uint64
// 			if playerState, ok := g.state.PlayersState[playerUpdate.PlayerId]; ok {
// 				gameToken = playerState.GameToken
// 				gameTokenInt = playerState.GameTokenInt
// 			}
// 			g.state.PlayersState[playerUpdate.PlayerId] = &PlayerState{BuyIn: playerUpdate.BuyIn,
// 				CurrentBalance: playerUpdate.Stack,
// 				Status:         playerUpdate.Status,
// 				GameToken:      gameToken,
// 				GameTokenInt:   gameTokenInt}
// 		}
// 	}

// 	// send player update message to all
// 	if *g.messageReceiver != nil {
// 		(*g.messageReceiver).BroadcastGameMessage(message)

// 		if message.GetPlayerUpdate().NewUpdate == NewUpdate_SWITCH_SEAT {
// 			// switch seat, wait for animation
// 			time.Sleep(1 * time.Second)
// 		}
// 	}

// 	channelGameLogger.Info().Msg(fmt.Sprintf("Player update: %v DONE", playerUpdate))

// 	if g.state.Status == GameStatus_ACTIVE && !g.running {
// 		g.startGame()
// 	}

// 	return nil
// }
