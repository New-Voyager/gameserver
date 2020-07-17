package game

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	"voyager.com/server/poker"
)

func (game *Game) handleGameMessage(message *GameMessage) {
	channelGameLogger.Trace().
		Uint32("club", game.clubID).
		Uint32("game", game.gameNum).
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
	}
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
			Uint32("game", game.gameNum).
			Str("message", "GameSitMessage").
			Msg(fmt.Sprintf("Invalid seat no: %d Allowed values from 1-%d", gameSit.SeatNo, gameState.MaxSeats))
		return fmt.Errorf("Invalid seat no: %d Allowed values from 1-%d", gameSit.SeatNo, gameState.MaxSeats)
	}

	playersInSeat := gameState.PlayersInSeats
	if playersInSeat[gameSit.SeatNo-1] != 0 {
		// there is already a player in the seat
		channelGameLogger.Error().
			Uint32("club", game.clubID).
			Uint32("game", game.gameNum).
			Str("message", "GameSitMessage").
			Msg(fmt.Sprintf("A player is already sitting in the seat: %d", gameSit.SeatNo))
		return fmt.Errorf("A player is already sitting in the seat: %d", gameSit.SeatNo)
	}

	channelGameLogger.Info().
		Uint32("club", game.clubID).
		Uint32("game", game.gameNum).
		Uint32("player", gameSit.PlayerId).
		Str("message", "GameSitMessage").
		Msg(fmt.Sprintf("Player %d took %d seat, buy-in: %f", gameSit.PlayerId, gameSit.SeatNo, gameSit.BuyIn))

	gameState.PlayersInSeats[gameSit.SeatNo-1] = gameSit.PlayerId
	// TODO: Need to work on the buy-in and sitting
	// This is a bigger work item. A multiple players will be auto-seated
	// If the buy-in needs to approved by the club manager, we need to wait for the approval
	// we need a state tracking for seat as well
	// seat state: open, waiting for buyin approval, sitting, occupied, break, hold for certain time limit

	if gameState.PlayersState == nil {
		gameState.PlayersState = make(map[uint32]*PlayerState)
	}
	gameState.PlayersState[gameSit.PlayerId] = &PlayerState{BuyIn: gameSit.BuyIn, CurrentBalance: gameSit.BuyIn, Status: PlayerStatus_PLAYING}

	// save game state
	err = game.saveState(gameState)
	if err != nil {
		return err
	}

	// send player sat message to all
	playerSatMessage := GamePlayerSatMessage{SeatNo: gameSit.SeatNo, BuyIn: gameSit.BuyIn, PlayerId: gameSit.PlayerId}
	gameMessage := GameMessage{MessageType: PlayerSat, ClubId: message.ClubId, GameNum: message.GameNum}
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

	if gameState.Status == GameStatus_START_GAME_RECEIVED {
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

	gameTableState := &GameTableStateMessage{PlayersState: playersAtTable}
	var gameMessage GameMessage
	gameMessage.ClubId = game.clubID
	gameMessage.GameNum = game.gameNum
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

func (game *Game) onJoinGame(message *GameMessage) error {
	joinMessage := message.GetJoinGame()
	game.players[joinMessage.PlayerId] = joinMessage.Name
	return nil
}
