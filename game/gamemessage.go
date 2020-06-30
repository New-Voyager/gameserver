package game

import (
	"fmt"
	"google.golang.org/protobuf/proto"
)

func (game *Game) handleGameMessage(message GameMessage) {
	channelGameLogger.Info().
		Uint32("club", game.clubID).
		Uint32("game", game.gameNum).
		Msg(fmt.Sprintf("Game message: %s. %v", message.messageType, message))

	defer game.lock.Unlock()
	game.lock.Lock()

	switch message.messageType {
	case PlayerTookSeat:
		game.onPlayerTookSeat(message)
		if len(game.activePlayers) == 9 {
			break
		}
	}

	channelGameLogger.Info().
		Uint32("club", game.clubID).
		Uint32("game", game.gameNum).
		Msg(fmt.Sprintf("Game message: %s. RETURN", message.messageType))
}

func (game *Game) onPlayerTookSeat(message GameMessage) error {
	gameState, err := game.loadState()
	if err != nil {
		return err
	}

	// unmarshal GameSitMessage
	var gameSit GameSitMessage
	err = proto.Unmarshal(message.messageProto, &gameSit)
	if err != nil {
		channelGameLogger.Error().
			Uint32("club", game.clubID).
			Uint32("game", game.gameNum).
			Str("message", "GameSitMessage").
			Msg(fmt.Sprintf("Error unmarshaling game sit message (GameSitMessage).  Error: %v", err))
		return err
	}

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

	gameState.PlayersInSeats[gameSit.SeatNo-1] = gameSit.PlayerId
	// TODO: Need to work on the buy-in and sitting
	// This is a bigger work item. A multiple players will be auto-seated
	// If the buy-in needs to approved by the club manager, we need to wait for the approval
	// we need a state tracking for seat as well
	// seat: open, waiting for buyin approval, occupied, break, hold for certain time limit

	if gameState.PlayersState == nil {
		gameState.PlayersState = make(map[uint32]*PlayerState)
	}
	gameState.PlayersState[gameSit.PlayerId] = &PlayerState{BuyIn: 100, CurrentBalance: 100, Status: PlayerState_PLAYING}

	// save game state
	err = game.saveState(gameState)
	if err != nil {
		return err
	}
	game.activePlayers[message.playerID] = message.player
	game.players[message.playerID] = message.player.playerName
	return nil
}
