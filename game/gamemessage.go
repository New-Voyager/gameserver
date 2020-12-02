package game

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/http"

	"google.golang.org/protobuf/proto"
	"voyager.com/server/poker"
)

func (game *Game) handleGameMessage(message *GameMessage) {
	channelGameLogger.Trace().
		Uint32("club", game.clubID).
		Uint64("game", game.gameID).
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
	}
}

func processPendingUpdates(apiServerUrl string, gameID uint64) {
	// call api server processPendingUpdates
	channelGameLogger.Info().Msgf("Processing pending updates for the game %d", gameID)
	url := fmt.Sprintf("%s/internal/process-pending-updates/gameId/%d", apiServerUrl, gameID)
	resp, _ := http.Post(url, "application/json", nil)
	if resp.StatusCode != 200 {
		channelGameLogger.Fatal().Uint64("game", gameID).Msg(fmt.Sprintf("Failed to process pending updates. Error: %d", resp.StatusCode))
	}
}

func (game *Game) onMoveToNextHand(message *GameMessage) error {

	// if this game is used by script test, don't look for pending updates
	if game.scriptTest {
		return nil
	}

	// before we move to next hand, query API server whether we have any pending updates
	// if there are no pending updates, deal next hand

	// check any pending updates
	pendingUpdates, _ := anyPendingUpdates(game.apiServerUrl, game.gameID)
	if pendingUpdates {
		go processPendingUpdates(game.apiServerUrl, game.gameID)
	} else {
		gameMessage := &GameMessage{
			GameId:      game.gameID,
			MessageType: GameDealHand,
		}
		game.SendGameMessage(gameMessage)
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
			Uint32("club", game.clubID).
			Uint64("game", game.gameID).
			Str("message", "GameSitMessage").
			Msg(fmt.Sprintf("Invalid seat no: %d Allowed values from 1-%d", gameSit.SeatNo, gameState.MaxSeats))
		return fmt.Errorf("Invalid seat no: %d Allowed values from 1-%d", gameSit.SeatNo, gameState.MaxSeats)
	}

	playersInSeat := gameState.PlayersInSeats
	if playersInSeat[gameSit.SeatNo-1] != 0 {
		// there is already a player in the seat
		channelGameLogger.Error().
			Uint32("club", game.clubID).
			Uint64("game", game.gameID).
			Str("message", "GameSitMessage").
			Msg(fmt.Sprintf("A player is already sitting in the seat: %d", gameSit.SeatNo))
		return fmt.Errorf("A player is already sitting in the seat: %d", gameSit.SeatNo)
	}

	channelGameLogger.Info().
		Uint32("club", game.clubID).
		Uint64("game", game.gameID).
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
	gameMessage.ClubId = game.clubID
	gameMessage.GameId = game.gameID
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
	gameMessage.ClubId = game.clubID
	gameMessage.GameId = game.gameID
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
	channelGameLogger.Debug().Msg(fmt.Sprintf("Player update: %v", playerUpdate))
	gameState, err := g.loadState()
	if err != nil {
		return err
	}

	if gameState.Status == GameStatus_ACTIVE &&
		gameState.TableStatus == TableStatus_TABLE_STATUS_GAME_RUNNING {
		// table is running
		// add the update to pending updates
	} else {

		// NOTE: we should be here only when a player takes a seat
		// or buys in
		// or added to queue
		if playerUpdate.SeatNo > 0 {

			gameState.PlayersInSeats[playerUpdate.SeatNo-1] = playerUpdate.PlayerId
			if gameState.PlayersState == nil {
				gameState.PlayersState = make(map[uint64]*PlayerState)
			}
			var tokenInt uint64
			if playerUpdate.GameToken != "" {
				// pad here 000000
				gameToken := fmt.Sprintf("000000%s", playerUpdate.GameToken)
				token, _ := hex.DecodeString(gameToken)
				tokenInt = binary.LittleEndian.Uint64(token)
				channelGameLogger.Info().
					Uint64("game", g.gameID).
					Uint64("player", playerUpdate.PlayerId).
					Str("message", "GameSitMessage").
					Msgf("Player %d took %d seat, buy-in: %f, gameToken: %s tokenXorKey: %X",
						playerUpdate.PlayerId, playerUpdate.SeatNo, playerUpdate.BuyIn, playerUpdate.GameToken, tokenInt)
			} else {
				channelGameLogger.Info().
					Uint64("game", g.gameID).
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
		} else if playerUpdate.Status == PlayerStatus_LEFT {
			// player left
			panic("Player left should be handled up pending updates")
		} else if playerUpdate.Status == PlayerStatus_IN_BREAK {
			// player in break
			panic("Player in break should be handled up pending updates")
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
	}

	if gameState.Status == GameStatus_ACTIVE {
		g.startGame()
	}

	return nil
}
