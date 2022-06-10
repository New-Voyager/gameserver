package player

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"voyager.com/botrunner/internal/game"
	"voyager.com/botrunner/internal/util"
	"voyager.com/gamescript"
)

func (bp *BotPlayer) processNonProtoGameMessage(message *gamescript.NonProtoMessage) {
	if util.Env.ShouldPrintGameMsg() {
		fmt.Printf("HANDLING NON-PROTO GAME MESSAGE: %+v\n", message)
	}
	bp.GameMessages = append(bp.GameMessages, message)
	switch message.Type {
	case "GAME_STATUS":
		gs := game.GameStatus(game.GameStatus_value[message.GameStatus])
		ts := game.TableStatus(game.TableStatus_value[message.TableStatus])
		bp.game.status = gs
		bp.game.tableStatus = ts
		bp.logger.Info().Msgf("Received game status message. Game Status: %s Table Status: %s", gs, ts)
		if ts == game.TableStatus_GAME_RUNNING {
			err := bp.queryCurrentHandState()
			if err != nil {
				bp.logger.Error().Msgf("Error while querying current hand state. Error: %v", err)
			}
		}
		if gs == game.GameStatus_ENDED {
			// The game just ended. Player should leave the game.
			err := bp.LeaveGameImmediately()
			if err != nil {
				bp.logger.Error().Msgf("Error while leaving game: %s", err)
			}
		}
		if ts == game.TableStatus_NOT_ENOUGH_PLAYERS {
			if bp.IsHost() {
				err := bp.processNotEnoughPlayers()
				if err != nil {
					errMsg := fmt.Sprintf("Error while processing not-enough-players: %s", err)
					bp.logger.Error().Msg(errMsg)
					bp.errorStateMsg = errMsg
					bp.sm.SetState(BotState__ERROR)
					return
				}
			}
		}

	case "PLAYER_UPDATE":
		if message == nil {
			return
		}
		playerID := message.PlayerID
		playerStatus := game.PlayerStatus(game.PlayerStatus_value[message.Status])

		// SOMA: Don't update table view here
		// table view is updated for every hand
		//bp.game.table.playersBySeat[seatNo] = p
		if playerID == bp.PlayerID {
			// me
			bp.updateSeatNo(message.SeatNo)
			bp.balance = message.Stack
			newUpdate := message.NewUpdate
			if newUpdate == "WAIT_FOR_BUYIN" {
				bp.logger.Info().Msgf("Bot [%s] ran out of stack. Player status: %s", message.PlayerName, newUpdate)
				if bp.game.tableStatus == game.TableStatus_NOT_ENOUGH_PLAYERS {
					// The hand can't continue unless someone buys in now.
					err := bp.autoReloadBalance()
					if err != nil {
						errMsg := fmt.Sprintf("Could not reload chips when status is WAIT_FOR_BUYIN. Current hand num: %d. Error: %v", bp.game.handNum, err)
						bp.logger.Error().Msg(errMsg)
						bp.errorStateMsg = errMsg
						bp.sm.SetState(BotState__ERROR)
						return
					}
				} else {
					// Don't need to buy in now.
					// If we buy in now, we sometimes get into the very next hand
					// without skipping a hand. This behavior makes the testing hard
					// since it all depends on the timing.
					// Just wait for the next hand to start and then buy in, so that
					// we always skip one hand.
				}
			}
			if playerStatus == game.PlayerStatus_PLAYING &&
				message.Stack > 0.0 {
				bp.observing = false
			}
		}
		bp.logger.Info().Msgf("PlayerUpdate: ID: %d Seat No: %d Stack: %v Status: %s",
			playerID, message.SeatNo, message.Stack, message.Status)
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
					bp.logger.Info().Msgf("CONFIRM seat change (per hand %d setup)", bp.game.handNum)
					bp.gqlHelper.ConfirmSeatChange(bp.gameCode, openedSeatNo)
				} else {
					bp.logger.Info().Msgf("DECLINE seat change (per hand %d setup)", bp.game.handNum)
					bp.gqlHelper.DeclineSeatChange(bp.gameCode)
				}
			}
		}
	case "PLAYER_SEAT_MOVE":
		oldSeatNo := message.OldSeatNo
		newSeatNo := message.NewSeatNo
		playerID := message.PlayerID
		if playerID == bp.PlayerID {

		}
		if bp.IsObserver() {
			bp.logger.Info().Msgf("Player [%s] changed seat %d -> %d", message.PlayerName, oldSeatNo, newSeatNo)
		}
	case "PLAYER_SEAT_CHANGE_DONE":
		break
	case "NEW_HIGHHAND_WINNER":
		break
	case "TABLE_UPDATE":
		bp.onTableUpdate(message)
	case "WAITLIST_SEATING":
		bp.seatWaitList(message)
	case "GAME_ENDING":
		bp.logger.Info().Msgf("Received game ending notification")
	}
}

func (bp *BotPlayer) onTableUpdate(message *gamescript.NonProtoMessage) {
	if message.SubType == "HostSeatChangeMove" {
		data, _ := json.Marshal(message)
		fmt.Printf("%s", string(data))
	}
}

func (bp *BotPlayer) processNotEnoughPlayers() error {
	scriptHand := bp.config.Script.GetHand(bp.game.handNum)
	if scriptHand.WhenNotEnoughPlayers.RequestEndGame {
		bp.logger.Info().Msgf("Requesting to end the game [%s] due to not enough players", bp.gameCode)
		return bp.RequestEndGame(bp.gameCode)
	}
	if len(scriptHand.WhenNotEnoughPlayers.AddPlayers) > 0 {

	}
	return nil
}

func (bp *BotPlayer) seatWaitList(message *gamescript.NonProtoMessage) {
	bp.logger.Info().Msgf("Waitlist seating message received In Waitlist: %v", bp.inWaitList)
	if !bp.inWaitList {
		return
	}
	// waitlist seating
	if bp.PlayerID != message.WaitlistPlayerId {
		// not my turn
		return
	}
	bp.logger.Info().Msgf("My turn to take a seat: Confirm Waitlist: %v", bp.confirmWaitlist)

	if !bp.confirmWaitlist {
		// decline wait list
		bp.logger.Info().Msgf("declining to take the open seat.")
		confirmed, err := bp.gqlHelper.DeclineWaitListSeat(bp.gameCode)
		if err != nil {
			panic("Error while declining waitlist seat")
		}
		if !confirmed {
			panic("Response from DeclineWaitListSeat has confirmed = false")
		}
		return
	}

	bp.logger.Info().Msgf("Accepting to take the open seat.")
	// get open seats
	gi, err := bp.GetGameInfo(bp.gameCode)
	if err != nil {
		bp.logger.Error().Msgf("Unable to get game info %s", bp.gameCode)
	}
	openSeat := uint32(0)
	for _, seatNo := range gi.SeatInfo.AvailableSeats {
		openSeat = seatNo
		break
	}
	if openSeat == 0 {
		bp.logger.Error().Msgf("No open seat available %s", bp.gameCode)
		return
	}
	bp.event(BotEvent__REQUEST_SIT)
	// confirm join game
	err = bp.SitIn(bp.gameCode, openSeat, nil)
	if err != nil {
		panic(fmt.Sprintf("Player could not take seat %d: %s", openSeat, err))
	} else {
		// buyin
		if bp.buyInAmount != 0 {
			err := bp.BuyIn(bp.gameCode, float64(bp.buyInAmount))
			if err != nil {
				bp.logger.Error().Msgf("Unable to buy in %d chips while sitting from waitlist: %s", bp.buyInAmount, err.Error())
			} else {
				bp.logger.Info().Msgf("Player bought in for: %d. Current hand num: %d",
					bp.buyInAmount, bp.game.handNum)
				bp.event(BotEvent__SUCCEED_BUYIN)
			}
		}
	}
}

func (bp *BotPlayer) setupSeatChange() error {
	if bp.tournament {
		return nil
	}

	if int(bp.game.handNum) > len(bp.config.Script.Hands) {
		return nil
	}

	currentHand := bp.config.Script.GetHand(bp.game.handNum)
	seatChanges := currentHand.Setup.SeatChange
	if seatChanges == nil {
		return nil
	}

	// using seat no, get the bot player and make seat change request
	for _, seatChangeRequest := range seatChanges {
		if seatChangeRequest.Seat == bp.seatNo {
			bp.logger.Info().Msgf("Requesting seat change.")
			bp.gqlHelper.RequestSeatChange(bp.gameCode)
		}
	}
	return nil
}

func (bp *BotPlayer) setupTakeBreak() error {
	if int(bp.game.handNum) > len(bp.config.Script.Hands) {
		return nil
	}

	currentHand := bp.config.Script.GetHand(bp.game.handNum)
	breakConfigs := currentHand.Setup.TakeBreak
	if breakConfigs == nil {
		return nil
	}

	// using seat no, get the bot player and make seat change request
	for _, breakConfig := range breakConfigs {
		if breakConfig.Seat == bp.seatNo {
			bp.logger.Info().Msgf("Requesting to take a break.")
			bp.gqlHelper.RequestTakeBreak(bp.gameCode)
		}
	}
	return nil
}

func (bp *BotPlayer) setupSitBack() error {
	if int(bp.game.handNum) > len(bp.config.Script.Hands) {
		return nil
	}

	currentHand := bp.config.Script.GetHand(bp.game.handNum)
	sitbackConfigs := currentHand.Setup.SitBack
	if sitbackConfigs == nil {
		return nil
	}

	// using seat no, get the bot player and make seat change request
	for _, sitbackConfig := range sitbackConfigs {
		if sitbackConfig.Seat == bp.seatNo {
			bp.logger.Info().Msgf("Player [%s] sitting back.", bp.config.Name)
			bp.gqlHelper.RequestSitBack(bp.gameCode, bp.config.Gps)
		}
	}
	return nil
}

func (bp *BotPlayer) setupRunItTwice() error {
	if bp.tournament {
		return nil
	}

	if int(bp.game.handNum) > len(bp.config.Script.Hands) {
		return nil
	}

	currentHand := bp.config.Script.GetHand(bp.game.handNum)
	runItTwiceConfigs := currentHand.Setup.RunItTwice
	if runItTwiceConfigs == nil {
		return nil
	}

	for _, playerRITSetup := range runItTwiceConfigs {
		if playerRITSetup.Seat == bp.seatNo {
			bp.UpdateGamePlayerSettings(bp.gameCode, nil, nil, nil, nil, &playerRITSetup.AllowPrompt, nil)
		}
	}
	return nil
}

func (bp *BotPlayer) getRunItTwiceConfig() *gamescript.RunItTwiceSetup {
	if int(bp.game.handNum) > len(bp.config.Script.Hands) {
		return nil
	}

	currentHand := bp.config.Script.GetHand(bp.game.handNum)
	runItTwiceConfigs := currentHand.Setup.RunItTwice
	if runItTwiceConfigs == nil {
		return nil
	}

	for _, playerRITSetup := range runItTwiceConfigs {
		if playerRITSetup.Seat == bp.seatNo {
			return &playerRITSetup
		}
	}

	return nil
}

func (bp *BotPlayer) pauseGameIfNeeded() error {
	if int(bp.game.handNum) > len(bp.config.Script.Hands) {
		return nil
	}

	currentHand := bp.config.Script.GetHand(bp.game.handNum)
	if currentHand.PauseGame {
		bp.logger.Info().Msgf("Player [%s] requested to pause the game.", bp.config.Name)
		bp.gqlHelper.PauseGame(bp.gameCode)
	}
	return nil
}

func (bp *BotPlayer) processPostHandSteps() error {
	if int(bp.game.handNum) > len(bp.config.Script.Hands) {
		return nil
	}
	bp.logger.Info().Msgf("Running post hand steps.")

	currentHand := bp.config.Script.GetHand(bp.game.handNum)
	if len(currentHand.PostHandSteps) == 0 {
		bp.logger.Info().Msgf("No post hand steps.")
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
			bp.logger.Info().Msgf("Post hand step: Sleeping %d", step.Sleep)
			time.Sleep(time.Duration(step.Sleep) * time.Second)
			bp.logger.Info().Msgf("Post hand step: Sleeping %d done", step.Sleep)
			continue
		}
		if step.ResumeGame {
			bp.logger.Info().Msgf("Post hand step: Resume game %s", bp.gameCode)
			// resume game
			attempts := 1
			err := bp.gqlHelper.ResumeGame(bp.gameCode)
			for err != nil && attempts < bp.maxRetry {
				attempts++
				bp.logger.Error().Msgf("Error while resuming game %s: %s. Retrying... (%d)", bp.gameCode, err, attempts)
				time.Sleep(2 * time.Second)
				err = bp.gqlHelper.ResumeGame(bp.gameCode)
			}
			continue
		}

		if len(step.HostSeatChange.Changes) > 0 {
			// initiate host seat change process
			bp.hostSeatChange(&step.HostSeatChange)
			continue
		}

		if step.BuyCoins > 0 {
			err := bp.restHelper.BuyAppCoins(bp.PlayerUUID, step.BuyCoins)
			if err != nil {
				bp.logger.Error().Msgf("Error when buying app coins %s: %s", bp.gameCode, err)
			}
			continue
		}
	}
	bp.logger.Info().Msgf("Running post hand steps done")
	return nil
}

func (bp *BotPlayer) hostSeatChange(hostSeatChange *gamescript.HostSeatChange) error {
	// initiate seat change process
	bp.logger.Error().Msgf("Initiating host seat change process game %s: %s", bp.gameCode)
	_, err := bp.gqlHelper.HostRequestSeatChange(bp.gameCode)
	if err != nil {
		bp.logger.Error().Msgf("Error setting up host seat change process game %s: %s", bp.gameCode, err)
		return err
	}
	// make seat changes
	for _, change := range hostSeatChange.Changes {
		bp.logger.Error().Msgf("game %s: Swapping seat: %d to seat2: %d", bp.gameCode, change.Seat1, change.Seat2)
		_, err := bp.gqlHelper.HostRequestSeatChangeSwap(bp.gameCode, change.Seat1, change.Seat2)
		if err != nil {
			bp.logger.Error().Msgf("Error swapping seat1: %d to seat2: %d failed. game %s: %s", change.Seat1, change.Seat2, bp.gameCode, err)
			return err
		}
	}

	bp.logger.Error().Msgf("game %s: Completing seat change updates", bp.gameCode)
	// complete seat change process
	_, err = bp.gqlHelper.HostRequestSeatChangeComplete(bp.gameCode)
	if err != nil {
		bp.logger.Error().Msgf("Error completing host seat change process game %s: %s", bp.gameCode, err)
		return err
	}
	return nil
}

func (bp *BotPlayer) setupLeaveGame() error {
	if int(bp.game.handNum) > len(bp.config.Script.Hands) {
		return nil
	}

	currentHand := bp.config.Script.GetHand(bp.game.handNum)
	leaveGame := currentHand.Setup.LeaveGame
	if leaveGame != nil {
		// using seat no, get the bot player and make seat change request
		for _, leaveGameRequest := range leaveGame {
			if leaveGameRequest.Seat == bp.seatNo {
				// will leave in next hand
				if bp.IsSeated() {
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

func (bp *BotPlayer) setupSwitchSeats() error {
	if int(bp.game.handNum) > len(bp.config.Script.Hands) {
		return nil
	}
	currentHand := bp.config.Script.GetHand(bp.game.handNum)
	switchSeats := currentHand.Setup.SwitchSeats
	if switchSeats != nil {
		// using seat no, get the bot player and make seat change request
		for _, request := range switchSeats {
			if request.FromSeat == bp.seatNo {
				// will leave in next hand
				if bp.IsSeated() {
					var err error
					_, err = bp.gqlHelper.SwitchSeat(bp.gameCode, int(request.ToSeat))
					if err != nil {
						return errors.Wrap(err, "Error while making a GQL request to leave game")
					}
				}
			}
		}
	}
	return nil
}

func (bp *BotPlayer) setupReloadChips() error {
	if int(bp.game.handNum) > len(bp.config.Script.Hands) {
		return nil
	}
	currentHand := bp.config.Script.GetHand(bp.game.handNum)
	reloadChips := currentHand.Setup.ReloadChips
	if reloadChips != nil {
		// using seat no, get the bot player and make seat change request
		for _, request := range reloadChips {
			if request.SeatNo == bp.seatNo {
				// will leave in next hand
				var err error
				_, err = bp.gqlHelper.ReloadChips(bp.gameCode, request.Amount)
				if err != nil {
					return errors.Wrap(err, "Error while making a GQL request to leave game")
				}
			}
		}
	}
	return nil
}

func (bp *BotPlayer) JoinWaitlist(gameCode string, observer *gamescript.Observer, confirmWaitlist bool) error {
	if bp.gameCode != "" {
		gameCode = bp.gameCode
	}
	_, err := bp.gqlHelper.JoinWaitList(gameCode)
	if err == nil {
		bp.inWaitList = true
		if observer != nil {
			bp.confirmWaitlist = observer.Confirm
			bp.buyInAmount = uint32(observer.BuyIn)
		} else {
			bp.confirmWaitlist = confirmWaitlist
		}
	}
	return err
}

func (bp *BotPlayer) updatePlayersConfig() error {
	// tournament is auto play
	if bp.tournament {
		return nil
	}

	if int(bp.game.handNum) > len(bp.config.Script.Hands) {
		return nil
	}

	currentHand := bp.config.Script.GetHand(bp.game.handNum)
	playersConfig := currentHand.Setup.PlayersConfig
	// using seat no, get the bot player and make seat change request
	for _, playerConfig := range playersConfig {
		if playerConfig.Player == bp.config.Name {
			// update player config
			// ip address, location, run-it-twice, muck-losing-hand
			if playerConfig.IpAddress != nil {
				bp.SetIPAddress(*playerConfig.IpAddress)
				bp.gqlHelper.UpdateIpAddress(*playerConfig.IpAddress)
			}

			if playerConfig.Gps != nil {
				bp.SetGpsLocation(playerConfig.Gps)
				bp.gqlHelper.UpdateGpsLocation(playerConfig.Gps)
			}
		}
	}
	return nil
}
