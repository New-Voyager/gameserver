package player

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"
	"voyager.com/botrunner/internal/game"
	"voyager.com/gamescript"
)

func (bp *BotPlayer) processGameMessage(message *game.GameMessage) {
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
		// SOMA: Don't update table view here
		// table view is updated for every hand
		//bp.game.table.playersBySeat[seatNo] = p
		if playerID == bp.PlayerID {
			// me
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
				fmt.Printf("%s\n", string(data))

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
			err := bp.LeaveGameImmediately()
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

func (bp *BotPlayer) processNonProtoGameMessage(message *NonProtoMessage) {
	fmt.Printf("[%s] HANDLING NON-PROTO GAME MESSAGE: %+v\n", bp.logPrefix, message)
	switch message.Type {
	case "PLAYER_SEAT_CHANGE_PROMPT":
		if message.PlayerID != bp.PlayerID {
			// This message is for some other player.
			return
		}
		openedSeatNo := message.OpenedSeat
		scriptHandConf := bp.config.Script.GetHand(bp.game.handNum)
		for _, sc := range scriptHandConf.Setup.SeatChange {
			if sc.Seat == bp.seatNo {
				if sc.Confirm {
					bp.logger.Info().Msgf("%s: CONFIRM seat change (per hand %d setup)", bp.logPrefix, bp.game.handNum)
					bp.gqlHelper.ConfirmSeatChange(bp.gameCode, openedSeatNo)
				} else {
					bp.logger.Info().Msgf("%s: DECLINE seat change (per hand %d setup)", bp.logPrefix, bp.game.handNum)
					bp.gqlHelper.DeclineSeatChange(bp.gameCode)
				}
			}
		}
		break
	case "PLAYER_SEAT_MOVE":
		oldSeatNo := message.OldSeatNo
		newSeatNo := message.NewSeatNo
		playerID := message.PlayerID
		if playerID == bp.PlayerID {

		}
		if bp.IsObserver() {
			bp.logger.Info().Msgf("Player [%s] changed seat %d -> %d", message.PlayerName, oldSeatNo, newSeatNo)
		}
		break
	case "PLAYER_SEAT_CHANGE_DONE":
		break
	}
}

func (bp *BotPlayer) onTableUpdate(message *game.GameMessage) {
	// based on the update, do different things
	tableUpdate := message.GetTableUpdate()
	if tableUpdate.Type == game.TableUpdateSeatChangeProcess {
		// data, _ := protojson.Marshal(message)
		// fmt.Printf("%s\n", string(data))
		// // open seat
		// // do i want to change seat??
		// if bp.requestedSeatChange && bp.confirmSeatChange {
		// 	bp.logger.Info().Msgf("%s: Confirming seat change to the open seat", bp.logPrefix)
		// 	// confirm seat change
		// 	bp.gqlHelper.ConfirmSeatChange(bp.gameCode)
		// }
	} else if tableUpdate.Type == game.TableUpdateWaitlistSeating {
		data, _ := protojson.Marshal(message)
		fmt.Printf("%s\n", string(data))

		bp.seatWaitList(message.GetTableUpdate())
	} else if tableUpdate.Type == game.TableUpdateHostSeatChangeMove ||
		tableUpdate.Type == game.TableUpdateHostSeatChangeInProcessStart ||
		tableUpdate.Type == game.TableUpdateHostSeatChangeInProcessEnd {
		data, _ := protojson.Marshal(message)

		if tableUpdate.Type == game.TableUpdateHostSeatChangeInProcessEnd {
			fmt.Printf("==========================\n")
			fmt.Printf("%s\n", string(data))
			fmt.Printf("==========================\n")
		} else {
			fmt.Printf("%s\n", string(data))
		}
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
					bp.logPrefix, bp.gameCode, bp.buyInAmount, bp.game.handNum)
				bp.event(BotEvent__SUCCEED_BUYIN)
			}
		}
	}
}

func (bp *BotPlayer) setupSeatChange() error {
	if int(bp.game.handNum) >= len(bp.config.Script.Hands) {
		return nil
	}

	currentHand := bp.config.Script.GetHand(bp.game.handNum)
	seatChanges := currentHand.Setup.SeatChange
	if seatChanges != nil {
		// using seat no, get the bot player and make seat change request
		for _, seatChangeRequest := range seatChanges {
			if seatChangeRequest.Seat == bp.seatNo {
				bp.logger.Info().Msgf("%s: Player [%s] requesting seat change.", bp.logPrefix, bp.config.Name)
				bp.gqlHelper.RequestSeatChange(bp.gameCode)
				bp.requestedSeatChange = true
				bp.confirmSeatChange = seatChangeRequest.Confirm
			}
		}
	}
	return nil
}

func (bp *BotPlayer) pauseGameIfNeeded() error {
	if int(bp.game.handNum) >= len(bp.config.Script.Hands) {
		return nil
	}

	currentHand := bp.config.Script.GetHand(bp.game.handNum)
	if currentHand.PauseGame {
		bp.logger.Info().Msgf("%s: Player [%s] requested to pause the game.", bp.logPrefix, bp.config.Name)
		bp.gqlHelper.PauseGame(bp.gameCode)
	}
	return nil
}

func (bp *BotPlayer) processPostHandSteps() error {
	if int(bp.game.handNum) >= len(bp.config.Script.Hands) {
		return nil
	}
	bp.logger.Info().Msgf("%s: Running post hand steps.", bp.logPrefix)

	currentHand := bp.config.Script.GetHand(bp.game.handNum)
	if len(currentHand.PostHandSteps) == 0 {
		bp.logger.Info().Msgf("%s: No post hand steps.", bp.logPrefix)
		return nil
	}

	// we need to process post hand steps only after the game is paused
	var isGamePaused bool
	var err error
	for !isGamePaused {
		isGamePaused, err = bp.isGamePaused()
		if err != nil {
			bp.logger.Error().Msgf("Getting game info failed. Error: %s", err.Error())
			panic(fmt.Sprintf("Getting game info failed. Error: %s", err.Error()))
		}
		if !isGamePaused {
			bp.logger.Info().Msgf("Game Info: Game pause status: %v. Waiting for a second", isGamePaused)
			time.Sleep(1 * time.Second)
		} else {
			bp.logger.Info().Msgf("Game Info: Game pause status: %v", isGamePaused)
		}
	}

	for _, step := range currentHand.PostHandSteps {
		if step.Sleep != 0 {
			bp.logger.Info().Msgf("%s: Post hand step: Sleeping %d", bp.logPrefix, step.Sleep)
			time.Sleep(time.Duration(step.Sleep) * time.Second)
			bp.logger.Info().Msgf("%s: Post hand step: Sleeping %d done", bp.logPrefix, step.Sleep)
			continue
		}
		if step.ResumeGame {
			bp.logger.Info().Msgf("%s: Post hand step: Resume game %s", bp.logPrefix, bp.gameCode)
			// resume game
			err := bp.gqlHelper.ResumeGame(bp.gameCode)
			if err != nil {
				bp.logger.Error().Msgf("%s: Error while resuming game %s: %s", bp.logPrefix, bp.gameCode, err)
			}
			continue
		}

		if len(step.HostSeatChange.Changes) > 0 {
			// initiate host seat change process
			bp.hostSeatChange(&step.HostSeatChange)
			continue
		}
	}
	bp.logger.Info().Msgf("%s: Running post hand steps done", bp.logPrefix)
	return nil
}

func (bp *BotPlayer) hostSeatChange(hostSeatChange *gamescript.HostSeatChange) error {
	// initiate seat change process
	bp.logger.Error().Msgf("%s: Initiating host seat change process game %s: %s", bp.logPrefix, bp.gameCode)
	_, err := bp.gqlHelper.HostRequestSeatChange(bp.gameCode)
	if err != nil {
		bp.logger.Error().Msgf("%s: Error setting up host seat change process game %s: %s", bp.logPrefix, bp.gameCode, err)
		return err
	}
	time.Sleep(2 * time.Second)
	// make seat changes
	for _, change := range hostSeatChange.Changes {
		bp.logger.Error().Msgf("%s: game %s: Swapping seat: %d to seat2: %d", bp.logPrefix, bp.gameCode, change.Seat1, change.Seat2)
		_, err := bp.gqlHelper.HostRequestSeatChangeSwap(bp.gameCode, change.Seat1, change.Seat2)
		time.Sleep(1 * time.Second)
		if err != nil {
			bp.logger.Error().Msgf("%s: Error swapping seat1: %d to seat2: %d failed. game %s: %s", bp.logPrefix, change.Seat1, change.Seat2, bp.gameCode, err)
			return err
		}
	}

	time.Sleep(2 * time.Second)
	bp.logger.Error().Msgf("%s: game %s: Completing seat change updates", bp.logPrefix, bp.gameCode)
	// complete seat change process
	_, err = bp.gqlHelper.HostRequestSeatChangeComplete(bp.gameCode)
	if err != nil {
		bp.logger.Error().Msgf("%s: Error completing host seat change process game %s: %s", bp.logPrefix, bp.gameCode, err)
		return err
	}
	return nil
}

func (bp *BotPlayer) setupLeaveGame() error {
	if int(bp.game.handNum) >= len(bp.config.Script.Hands) {
		return nil
	}

	currentHand := bp.config.Script.GetHand(bp.game.handNum)
	leaveGame := currentHand.Setup.LeaveGame
	if leaveGame != nil {
		// using seat no, get the bot player and make seat change request
		for _, leaveGameRequest := range leaveGame {
			if leaveGameRequest.Seat == bp.seatNo {
				// will leave in next hand
				if bp.isSeated {
					var err error
					_, err = bp.gqlHelper.LeaveGame(bp.gameCode)
					if err != nil {
						return errors.Wrap(err, "Error while making a GQL request to leave game")
					}
					bp.hasSentLeaveGameRequest = true
				}
			}
		}
	}
	return nil
}

func (bp *BotPlayer) JoinWaitlist(observer *gamescript.Observer) error {
	_, err := bp.gqlHelper.JoinWaitList(bp.gameCode)
	if err == nil {
		bp.confirmWaitlist = observer.Confirm
		bp.buyInAmount = uint32(observer.BuyIn)
	}
	return err
}

func (bp *BotPlayer) setupWaitList() error {
	if int(bp.game.handNum) >= len(bp.config.Script.Hands) {
		return nil
	}

	currentHand := bp.config.Script.GetHand(bp.game.handNum)
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
