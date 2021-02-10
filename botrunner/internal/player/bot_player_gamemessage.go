package player

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"
	"voyager.com/botrunner/internal/game"
)

func (bp *BotPlayer) handleGameMessage(message *game.GameMessage) {
	bp.lastGameMessage = message

	switch message.MessageType {
	case game.PlayerUpdate:
		playerUpdateMsg := message.GetPlayerUpdate()
		if playerUpdateMsg == nil {
			return
		}
		seatNo := playerUpdateMsg.GetSeatNo()
		playerID := playerUpdateMsg.GetPlayerId()
		playerStatus := playerUpdateMsg.GetStatus()
		buyIn := playerUpdateMsg.GetBuyIn()
		stack := playerUpdateMsg.GetStack()
		p := &player{
			seatNo:   seatNo,
			playerID: playerID,
			status:   playerStatus,
			buyIn:    buyIn,
			stack:    stack,
		}
		bp.game.table.playersBySeat[seatNo] = p
		if playerID == bp.PlayerID {
			// me
			bp.game.table.me = p
			bp.seatNo = p.seatNo

			if playerUpdateMsg.GetStatus() == game.PlayerStatus_PLAYING &&
				playerUpdateMsg.GetStack() > 0.0 {
				bp.observing = false
			}
		}
		bp.logger.Info().Msgf("%s: PlayerUpdate: ID: %d Seat No: %d Stack: %f Status: %d",
			bp.logPrefix, playerID, playerUpdateMsg.GetSeatNo(), playerUpdateMsg.GetStack(), playerUpdateMsg.GetStatus())

		if playerUpdateMsg.GetNewUpdate() == game.NewUpdate_SWITCH_SEAT {
			if playerID == bp.PlayerID {
				data, _ := json.Marshal(message)
				fmt.Printf("%s", string(data))

				bp.game.table.me = p
				bp.seatNo = p.seatNo
				bp.updateLogPrefix()
			}
			bp.logger.Info().Msgf("%s: Player: %d switched to a new seat. Seat No: %d from Seat: %d",
				bp.logPrefix, playerID, p.seatNo, playerUpdateMsg.OldSeat)
			// a player switched seat, his old seat is empty
			bp.game.table.playersBySeat[playerUpdateMsg.OldSeat] = nil
		}

	case game.GameCurrentStatus:
		gameStatus := message.GetStatus()
		if gameStatus == nil {
			return
		}

		gs := gameStatus.GetStatus()
		ts := gameStatus.GetTableStatus()
		bp.game.status = gs
		bp.game.tableStatus = ts
		bp.logger.Info().Msgf("%s: Received game status message. Game Status: %s Table Status: %s", bp.logPrefix, gs, ts)
		if ts == game.TableStatus_GAME_RUNNING {
			err := bp.queryCurrentHandState()
			if err != nil {
				bp.logger.Error().Msgf("%s: Error while querying current hand state. Error: %v", bp.logPrefix, err)
			}
		}
		if gs == game.GameStatus_ENDED {
			// The game just ended. Player should leave the game.
			err := bp.LeaveGame()
			if err != nil {
				bp.logger.Error().Msgf("%s: Error while leaving game: %s", bp.logPrefix, err)
			}
		}

	case game.GameTableUpdate:
		tableUpdateMsg := message.GetTableUpdate()
		if tableUpdateMsg == nil {
			return
		}
		bp.logger.Info().Msgf("%s: Received table update message. Type: %s", bp.logPrefix, tableUpdateMsg.Type)
		bp.onTableUpdate(message)
	}
}

func (bp *BotPlayer) onTableUpdate(message *game.GameMessage) {
	// based on the update, do different things
	tableUpdate := message.GetTableUpdate()
	if tableUpdate.Type == game.TableUpdateSeatChangeProcess {
		data, _ := protojson.Marshal(message)
		fmt.Printf("%s", string(data))
		// open seat
		// do i want to change seat??
		if bp.requestedSeatChange && bp.confirmSeatChange {
			bp.logger.Info().Msgf("%s: Confirming seat change to the open seat", bp.logPrefix)
			// confirm seat change
			bp.gqlHelper.ConfirmSeatChange(bp.gameCode)
		}
	} else if tableUpdate.Type == game.TableUpdateWaitlistSeating {
		data, _ := protojson.Marshal(message)
		fmt.Printf("%s", string(data))

		bp.seatWaitList(message.GetTableUpdate())
	}
}

func (bp *BotPlayer) seatWaitList(tableUpdate *game.TableUpdate) {
	// waitlist seating
	if bp.inWaitList {
		if bp.PlayerID != tableUpdate.WaitlistPlayerId {
			// not my turn
			return
		}

		if !bp.confirmWaitlist {
			// decline wait list
			bp.logger.Info().Msgf("%s: declined to take the open seat.", bp.logPrefix)
			bp.gqlHelper.DeclineWaitListSeat(bp.gameCode)
			return
		}

		bp.logger.Info().Msgf("%s: Confirm to take the open seat.", bp.logPrefix)
		// get open seats
		gi, err := bp.GetGameInfo(bp.gameCode)
		if err != nil {
			bp.logger.Error().Msgf("%s: Unable to get game info %s", bp.logPrefix, bp.gameCode)
		}
		openSeat := uint32(0)
		for _, seatNo := range gi.SeatInfo.AvailableSeats {
			openSeat = seatNo
			break
		}
		if openSeat == 0 {
			bp.logger.Error().Msgf("%s: No open seat available %s", bp.logPrefix, bp.gameCode)
			return
		}
		bp.event(BotEvent__REQUEST_SIT)
		// confirm join game
		err = bp.SitIn(bp.gameCode, openSeat)
		if err != nil {
			bp.logger.Error().Msgf("%s: [%s] Player could not take seat: %d", bp.logPrefix, bp.gameCode, openSeat)
		} else {
			bp.observing = false
			bp.logger.Info().Msgf("%s: [%s] Player took seat: %d", bp.logPrefix, bp.gameCode, openSeat)
			bp.isSeated = true
			bp.seatNo = openSeat
			bp.updateLogPrefix()
			// buyin
			if bp.buyInAmount != 0 {
				bp.BuyIn(bp.gameCode, float32(bp.buyInAmount))
				bp.logger.Info().Msgf("%s: [%s] Player bought in for: %d. Current hand num: %d",
					bp.logPrefix, bp.gameCode, bp.buyInAmount, bp.game.table.handNum)
				bp.event(BotEvent__SUCCEED_BUYIN)
			}
		}
	}
}

func (bp *BotPlayer) setupSeatChange() error {
	if int(bp.handNum) >= len(bp.config.Script.Hands) {
		return nil
	}

	currentHand := bp.config.Script.Hands[bp.handNum-1]
	seatChanges := currentHand.Setup.SeatChange
	if seatChanges != nil {
		// using seat no, get the bot player and make seat change request
		for _, seatChangeRequest := range seatChanges {
			if seatChangeRequest.SeatNo == bp.seatNo {
				bp.logger.Info().Msgf("%s: Player [%s] requested seat change.", bp.logPrefix, bp.config.Name)
				bp.gqlHelper.RequestSeatChange(bp.gameCode)
				bp.requestedSeatChange = true
				bp.confirmSeatChange = seatChangeRequest.Confirm
			}
		}
	}
	return nil
}

func (bp *BotPlayer) setupLeaveGame() error {
	if int(bp.handNum) >= len(bp.config.Script.Hands) {
		return nil
	}

	currentHand := bp.config.Script.Hands[bp.handNum-1]
	leaveGame := currentHand.Setup.LeaveGame
	if leaveGame != nil {
		// using seat no, get the bot player and make seat change request
		for _, leaveGameRequest := range leaveGame {
			if leaveGameRequest.SeatNo == bp.seatNo {
				// will leave in next hand
				if bp.isSeated {
					var err error
					_, err = bp.gqlHelper.LeaveGame(bp.gameCode)
					if err != nil {
						return errors.Wrap(err, "Error while making a GQL request to leave game")
					}
				}
			}
		}
	}
	return nil
}

func (bp *BotPlayer) setupWaitList() error {
	if int(bp.handNum) >= len(bp.config.Script.Hands) {
		return nil
	}

	currentHand := bp.config.Script.Hands[bp.handNum-1]
	waitLists := currentHand.Setup.WaitLists
	if waitLists != nil {
		for _, waitlistPlayer := range waitLists {
			if waitlistPlayer.Player == bp.GetName() {
				bp.logger.Info().Msgf("%s: Player [%s] requested to add to wait list.", bp.logPrefix, bp.config.Name)
				bp.gqlHelper.JoinWaitList(bp.gameCode)
				bp.inWaitList = true
				bp.confirmWaitlist = false
				bp.buyInAmount = uint32(waitlistPlayer.BuyIn)
				if waitlistPlayer.Confirm {
					bp.confirmWaitlist = true
				}
			}
		}
	}
	return nil
}
