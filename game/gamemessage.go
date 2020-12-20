package game

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"google.golang.org/protobuf/proto"
	"voyager.com/server/poker"
)

func (game *Game) handleGameMessage(message *GameMessage) {
	channelGameLogger.Trace().
		Uint32("club", game.config.ClubId).
		Str("game", game.config.GameCode).
		Msg(fmt.Sprintf("Game message: %s. %v", message.MessageType, message))

	switch message.MessageType {
	case PlayerTakeSeat:
		game.onPlayerTakeSeat(message)
		if game.playersInSeatsCount() == 9 {
			break
		}

	case GameStatusChanged:
		game.onStatusChanged(message)

	case GameSetupNextHand:
		game.onNextHandSetup(message)

	case GameDealHand:
		game.onDealHand(message)

	case GameQueryTableState:
		game.onQueryTableState(message)

	case GameJoin:
		game.onJoinGame(message)

	case PlayerUpdate:
		game.onPlayerUpdate(message)

	case GameMoveToNextHand:
		game.onMoveToNextHand(message)

	case GamePendingUpdatesDone:
		game.onPendingUpdatesDone(message)
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

func (game *Game) onPendingUpdatesDone(message *GameMessage) error {
	game.inProcessPendingUpdates = false
	// move to next hand
	gameState, err := game.loadState()
	if err != nil {
		return err
	}

	if gameState.Status == GameStatus_ACTIVE && gameState.TableStatus == TableStatus_TABLE_STATUS_GAME_RUNNING {
		// deal next hand
		gameMessage := &GameMessage{
			GameId:      game.config.GameId,
			MessageType: GameDealHand,
		}
		go game.SendGameMessage(gameMessage)
	}
	return nil
}

func (game *Game) onMoveToNextHand(message *GameMessage) error {

	time.Sleep(3 * time.Second)

	// if this game is used by script test, don't look for pending updates
	if game.scriptTest {
		return nil
	}

	if game.inProcessPendingUpdates {
		channelGameLogger.Info().Msgf("******* Processing pending updates. How did we get here?")
		return nil
	}

	// before we move to next hand, query API server whether we have any pending updates
	// if there are no pending updates, deal next hand

	// check any pending updates
	pendingUpdates, _ := anyPendingUpdates(game.apiServerUrl, game.config.GameId)
	if pendingUpdates {
		game.inProcessPendingUpdates = true
		go processPendingUpdates(game.apiServerUrl, game.config.GameId)
	} else {
		gameMessage := &GameMessage{
			GameId:      game.config.GameId,
			MessageType: GameDealHand,
		}
		go game.SendGameMessage(gameMessage)
	}

	return nil
}

func (game *Game) onPlayerTakeSeat(message *GameMessage) error {
	gameState, err := game.loadState()
	if err != nil {
		return err
	}
	gameSit := message.GetTakeSeat()

	if gameSit.SeatNo <= 0 || gameSit.SeatNo > gameState.MaxSeats {
		channelGameLogger.Error().
			Uint32("club", game.config.ClubId).
			Str("game", game.config.GameCode).
			Str("message", "GameSitMessage").
			Msg(fmt.Sprintf("Invalid seat no: %d Allowed values from 1-%d", gameSit.SeatNo, gameState.MaxSeats))
		return fmt.Errorf("Invalid seat no: %d Allowed values from 1-%d", gameSit.SeatNo, gameState.MaxSeats)
	}

	playersInSeat := gameState.PlayersInSeats
	if playersInSeat[gameSit.SeatNo-1] != 0 {
		// there is already a player in the seat
		channelGameLogger.Error().
			Uint32("club", game.config.ClubId).
			Str("game", game.config.GameCode).
			Str("message", "GameSitMessage").
			Msg(fmt.Sprintf("A player is already sitting in the seat: %d", gameSit.SeatNo))
		return fmt.Errorf("A player is already sitting in the seat: %d", gameSit.SeatNo)
	}

	channelGameLogger.Info().
		Uint32("club", game.config.ClubId).
		Str("game", game.config.GameCode).
		Uint64("player", gameSit.PlayerId).
		Str("message", "GameSitMessage").
		Msg(fmt.Sprintf("Player %d took %d seat, buy-in: %f", gameSit.PlayerId, gameSit.SeatNo, gameSit.BuyIn))

	gameState.PlayersInSeats[gameSit.SeatNo-1] = gameSit.PlayerId
	// TODO: Need to work on the buy-in and sitting
	// This is a bigger work item. A multiple players will be auto-seated
	// If the buy-in needs to approved by the club manager, we need to wait for the approval
	// we need a state tracking for seat as well
	// seat state: open, waiting for buyin approval, sitting, occupied, break, hold for certain time limit

	if gameState.PlayersState == nil {
		gameState.PlayersState = make(map[uint64]*PlayerState)
	}
	gameState.PlayersState[gameSit.PlayerId] = &PlayerState{BuyIn: gameSit.BuyIn, CurrentBalance: gameSit.BuyIn, Status: PlayerStatus_PLAYING}

	// save game state
	err = game.saveState(gameState)
	if err != nil {
		return err
	}

	// send player sat message to all
	playerSatMessage := GamePlayerSatMessage{SeatNo: gameSit.SeatNo, BuyIn: gameSit.BuyIn, PlayerId: gameSit.PlayerId}
	gameMessage := GameMessage{MessageType: PlayerSat, ClubId: message.ClubId, GameId: message.GameId}
	gameMessage.GameMessage = &GameMessage_PlayerSat{PlayerSat: &playerSatMessage}
	game.broadcastGameMessage(&gameMessage)
	return nil
}

func (g *Game) onStatusChanged(message *GameMessage) error {
	gameStatusChanged := message.GetStatusChange()
	gameState, err := g.loadState()
	if err != nil {
		return err
	}
	gameState.Status = gameStatusChanged.NewStatus
	err = g.saveState(gameState)
	if err != nil {
		return err
	}

	if gameState.Status == GameStatus_ACTIVE {
		g.startGame()
	}

	return nil
}

func (game *Game) onNextHandSetup(message *GameMessage) error {
	setupNextHand := message.GetNextHand()

	gameState, err := game.loadState()
	if err != nil {
		return err
	}
	gameState.ButtonPos = setupNextHand.ButtonPos
	game.saveState(gameState)

	game.testButtonPos = int32(setupNextHand.ButtonPos)
	game.testDeckToUse = poker.DeckFromBytes(setupNextHand.Deck)
	return nil
}

func (game *Game) onDealHand(message *GameMessage) error {
	err := game.dealNewHand()
	return err
}

// GetTableState returns the table returned to a specific player requested the state
func (game *Game) onQueryTableState(message *GameMessage) error {
	// get active players on the table
	playersAtTable, err := game.getPlayersAtTable()
	if err != nil {
		return err
	}
	gameState, err := game.loadState()
	if err != nil {
		return err
	}

	gameTableState := &GameTableStateMessage{PlayersState: playersAtTable, Status: gameState.Status, TableStatus: gameState.TableStatus}
	var gameMessage GameMessage
	gameMessage.ClubId = game.config.ClubId
	gameMessage.GameId = game.config.GameId
	gameMessage.MessageType = GameTableState
	gameMessage.PlayerId = message.GetQueryTableState().PlayerId
	gameMessage.GameMessage = &GameMessage_TableState{TableState: gameTableState}

	if *game.messageReceiver != nil {
		(*game.messageReceiver).SendGameMessageToPlayer(&gameMessage, message.GetQueryTableState().PlayerId)
	} else {
		messageData, _ := proto.Marshal(&gameMessage)
		game.allPlayers[message.PlayerId].chGame <- messageData
	}
	return nil
}

func (game *Game) broadcastTableState() error {
	// get active players on the table
	playersAtTable, err := game.getPlayersAtTable()
	if err != nil {
		return err
	}
	gameState, err := game.loadState()
	if err != nil {
		return err
	}

	gameTableState := &GameTableStateMessage{PlayersState: playersAtTable, Status: gameState.Status, TableStatus: gameState.TableStatus}
	var gameMessage GameMessage
	gameMessage.ClubId = game.config.ClubId
	gameMessage.GameId = game.config.GameId
	gameMessage.MessageType = GameTableState
	gameMessage.GameMessage = &GameMessage_TableState{TableState: gameTableState}

	if *game.messageReceiver != nil {
		(*game.messageReceiver).BroadcastGameMessage(&gameMessage)
	}
	return nil
}

func (game *Game) onJoinGame(message *GameMessage) error {
	joinMessage := message.GetJoinGame()
	game.players[joinMessage.PlayerId] = joinMessage.Name
	return nil
}

func (g *Game) onPlayerUpdate(message *GameMessage) error {
	playerUpdate := message.GetPlayerUpdate()
	channelGameLogger.Info().Msg(fmt.Sprintf("Player update: %v", playerUpdate))
	gameState, err := g.loadState()
	if err != nil {
		return err
	}
	if gameState.PlayersState == nil {
		gameState.PlayersState = make(map[uint64]*PlayerState)
	}

	if playerUpdate.Status == PlayerStatus_KICKED_OUT ||
		playerUpdate.Status == PlayerStatus_LEFT {
		// the player is out of the game
		for i, playerId := range gameState.PlayersInSeats {
			if playerId == playerUpdate.PlayerId {
				// this is the seat no where player was sitting
				gameState.PlayersInSeats[i] = 0
			}
		}
	} else if playerUpdate.Status == PlayerStatus_IN_BREAK {
		gameState.PlayersState[playerUpdate.PlayerId].Status = playerUpdate.Status
	} else {
		// we can only update the players that are in seats
		if playerUpdate.SeatNo == 0 {
			channelGameLogger.Error().Msg(fmt.Sprintf("Player update: SeatNo cannot be empty. %+v", playerUpdate))
			return fmt.Errorf("SeatNo cannot be empty")
		}

		// check to see if the player switched seat
		for seatNoIdx, playerID := range gameState.PlayersInSeats {
			seatNo := seatNoIdx + 1
			if playerID == playerUpdate.PlayerId &&
				uint32(seatNo) != playerUpdate.SeatNo {
				// this player switch seat
				channelGameLogger.Error().Msgf("Player %d switched seat from %d to %d", playerID, seatNo, playerUpdate.SeatNo)
				gameState.PlayersInSeats[seatNoIdx] = 0
				break
			}
		}

		// buyin/reload/sitting in the table
		gameState.PlayersInSeats[playerUpdate.SeatNo-1] = playerUpdate.PlayerId
		var tokenInt uint64
		if playerUpdate.GameToken != "" {
			// pad here 000000
			gameToken := fmt.Sprintf("000000%s", playerUpdate.GameToken)
			token, _ := hex.DecodeString(gameToken)
			tokenInt = binary.LittleEndian.Uint64(token)
			channelGameLogger.Info().
				Str("game", g.config.GameCode).
				Uint64("player", playerUpdate.PlayerId).
				Str("message", "GameSitMessage").
				Msgf("Player %d took %d seat, buy-in: %f, gameToken: %s tokenXorKey: %X",
					playerUpdate.PlayerId, playerUpdate.SeatNo, playerUpdate.BuyIn, playerUpdate.GameToken, tokenInt)
		} else {
			channelGameLogger.Info().
				Str("game", g.config.GameCode).
				Uint64("player", playerUpdate.PlayerId).
				Str("message", "GameSitMessage").
				Msgf("Player %d took %d seat, buy-in: %f",
					playerUpdate.PlayerId, playerUpdate.SeatNo, playerUpdate.BuyIn)
		}
		if playerUpdate.GameToken != "" {
			gameState.PlayersState[playerUpdate.PlayerId] = &PlayerState{BuyIn: playerUpdate.BuyIn,
				CurrentBalance: playerUpdate.Stack,
				Status:         playerUpdate.Status,
				GameToken:      playerUpdate.GameToken,
				GameTokenInt:   tokenInt}
		} else {
			var gameToken string
			var gameTokenInt uint64
			if playerState, ok := gameState.PlayersState[playerUpdate.PlayerId]; ok {
				gameToken = playerState.GameToken
				gameTokenInt = playerState.GameTokenInt
			}
			gameState.PlayersState[playerUpdate.PlayerId] = &PlayerState{BuyIn: playerUpdate.BuyIn,
				CurrentBalance: playerUpdate.Stack,
				Status:         playerUpdate.Status,
				GameToken:      gameToken,
				GameTokenInt:   gameTokenInt}
		}
	}

	// save game state
	err = g.saveState(gameState)
	if err != nil {
		return err
	}

	// send player update message to all
	if *g.messageReceiver != nil {
		(*g.messageReceiver).BroadcastGameMessage(message)
	}

	channelGameLogger.Info().Msg(fmt.Sprintf("Player update: %v DONE", playerUpdate))

	if gameState.Status == GameStatus_ACTIVE && !g.running {
		g.startGame()
	}

	return nil
}
