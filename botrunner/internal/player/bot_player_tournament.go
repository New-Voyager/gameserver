package player

import (
	"encoding/json"
	"fmt"

	natsgo "github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"voyager.com/botrunner/internal/game"
	"voyager.com/botrunner/internal/networkcheck"
	"voyager.com/gamescript"
)

func (bp *BotPlayer) RegisterTournament(tournamentID uint64) error {
	err := bp.gqlHelper.RegisterTournament(tournamentID)
	bp.tournamentID = tournamentID

	if err == nil {
		// subscribe for tournament messages
		bp.enterTournament()
	}
	return err
}

func (bp *BotPlayer) JoinTournament(tournamentID uint64) error {
	bp.logger.Info().Msgf("Joining tournament %d", bp.tournamentID)
	tournamentGameInfo, err := bp.gqlHelper.JoinTournament(tournamentID)
	if err == nil {
		bp.tournamentTableInfo = tournamentGameInfo.TournamentTableInfo
		bp.logger.Info().Msgf("Joining tournament %d is successful. Playing: %v", bp.tournamentID, bp.tournamentTableInfo.Playing)
		bp.tournamentTableNo = uint32(bp.tournamentTableInfo.TableNo)
		// subscribe to game channels
	} else {
		bp.logger.Info().Msgf("Joining tournament %d is failed", bp.tournamentID)
	}
	return err
}

// enterTournament enters a game without taking a seat as a player.
func (bp *BotPlayer) enterTournament() error {
	var e error
	bp.tournamentInfo, e = bp.gqlHelper.GetTournamentInfo(bp.tournamentID)
	if e != nil {
		return errors.Wrapf(e, "Error getting tournament info for tournament [%d]", bp.tournamentID)
	}

	bp.logger.Info().Msgf("Entering tournament [%d]", bp.tournamentID)
	if bp.tournamentMsgSubscription == nil || !bp.tournamentMsgSubscription.IsValid() {
		bp.logger.Info().Msgf("Subscribing to %s to receive hand messages sent to tournament channel: %s", bp.tournamentInfo.TournamentChannel, bp.config.Name)
		sub, err := bp.natsConn.Subscribe(bp.tournamentInfo.TournamentChannel, bp.handleTournamentMsg)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("Unable to subscribe to the tournament channel subject [%s]", bp.tournamentInfo.TournamentChannel))
		}
		bp.tournamentMsgSubscription = sub
		bp.logger.Info().Msgf("Successfully subscribed to %s.", bp.tournamentInfo.TournamentChannel)
	}
	if bp.tournamentPlayerMsgSubscription == nil || !bp.tournamentPlayerMsgSubscription.IsValid() {
		bp.logger.Info().Msgf("Subscribing to %s to receive hand messages sent to tournament player channel: %s", bp.tournamentInfo.PrivateChannel, bp.config.Name)
		sub, err := bp.natsConn.Subscribe(bp.tournamentInfo.PrivateChannel, bp.handleTournamentPrivateMsg)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("Unable to subscribe to the tournament player channel subject [%s]", bp.tournamentInfo.PrivateChannel))
		}
		bp.tournamentPlayerMsgSubscription = sub
		bp.logger.Info().Msgf("Successfully subscribed to tournament player channel %s.", bp.tournamentInfo.PrivateChannel)
	}

	return nil
}

func (bp *BotPlayer) handleTournamentMsg(msg *natsgo.Msg) {
	if bp.printTournamentMsg {
		bp.logger.Info().Msgf("Received game message %s", string(msg.Data))
	}
	var jsonMessage *gamescript.NonProtoTournamentMsg
	err := json.Unmarshal(msg.Data, &jsonMessage)
	if err == nil {
		bp.chTournament <- &TournamentMessageChannelItem{
			NonProtoMsg: jsonMessage,
		}
	} else {
		// handle proto message here
	}
}

func (bp *BotPlayer) handleTournamentPrivateMsg(msg *natsgo.Msg) {
	// if bp.printTournamentMsg {
	// 	bp.logger.Info().Msgf("Received private tournament message %s", string(msg.Data))
	// }
	bp.logger.Info().Msgf("##### Received private tournament message %s", string(msg.Data))
}

func (bp *BotPlayer) processTournamentMessage(message *TournamentMessageChannelItem) {
	if bp.IsErrorState() {
		bp.logger.Info().Msgf("Bot is in error state. Ignoring hand message.")
		return
	}

	if message.NonProtoMsg != nil {
		bp.processTournamentNonProtoMsg(message.NonProtoMsg)
	}
}

func (bp *BotPlayer) processTournamentNonProtoMsg(message *gamescript.NonProtoTournamentMsg) {
	if bp.IsErrorState() {
		bp.logger.Info().Msgf("Bot is in error state. Ignoring hand message.")
		return
	}
	//fmt.Printf("HANDLING TOURNAMENT MESSAGE: %+v\n", message.Type)
	// if util.Env.ShouldPrintTournamentMsg() {
	// 	fmt.Printf("HANDLING TOURNAMENT MESSAGE: %+v\n", message.Type)
	// }

	switch message.Type {
	// case "TOURNAMENT_STARTED":
	// 	bp.tournamentStarted(message.TournamentId)
	case "TOURNAMENT_ABOUT_TO_START":
		bp.tournamentAboutToStart(message.TournamentId)
	case "TOURNAMENT_INITIAL_PLAYER_TABLE":
		bp.setTournamentPlayerSeat(message)
	case "TOURNAMENT_PLAYER_MOVED_TABLE":
		bp.tournamentPlayerMoved(message)
	case "TOURNAMENT_ENDED":
		bp.tournamentEnded(message)
	}
}

func (bp *BotPlayer) tournamentAboutToStart(tournamentID uint64) {
	bp.logger.Info().Msgf("Tournament is about to start. Tournament ID [%d]", tournamentID)
	bp.tournamentID = tournamentID
	// var e error
	// bp.tournamentTableInfo, e = bp.gqlHelper.GetTournamentTableInfo(bp.tournamentID, bp.tournamentTableNo)
	// if e != nil {
	// 	return
	// }

	var err error
	err = bp.JoinTournament(tournamentID)
	if err != nil {
		bp.logger.Error().Err(err).Msgf("Could not join tournament %d", bp.tournamentID)
		return
	}

	bp.game = &gameView{
		table: &tableView{
			playersBySeat: make(map[uint32]*player),
			actionTracker: game.NewHandActionTracker(),
			playersActed:  make(map[uint32]*game.PlayerActRound),
		},
		handNum: 1,
	}

	bp.gameCode = bp.tournamentTableInfo.GameCode
	bp.gameID = bp.tournamentTableInfo.GameID
	bp.UpdateLogger()

	playerChannelName := fmt.Sprintf("player.%d", bp.PlayerID)
	err = bp.Subscribe(bp.tournamentTableInfo.GameToPlayerChannel,
		bp.tournamentTableInfo.HandToAllChannel, bp.tournamentTableInfo.HandToPlayerChannel,
		bp.tournamentTableInfo.HandToPlayerTextChannel, playerChannelName)
	if err != nil {
		// return errors.Wrap(err, fmt.Sprintf("Unable to subscribe to game %s channels",
		// 	bp.gameCode))
	}

	bp.meToHandSubjectName = bp.tournamentTableInfo.PlayerToHandChannel
	bp.clientAliveSubjectName = bp.tournamentTableInfo.ClientAliveChannel

	bp.logger.Info().Msgf("Starting network check client")
	bp.clientAliveCheck = networkcheck.NewClientAliveCheck(bp.logger, bp.gameID, bp.gameCode, bp.sendAliveMsg)
	bp.clientAliveCheck.Run()
}

func (bp *BotPlayer) setTournamentPlayerSeat(message *gamescript.NonProtoTournamentMsg) {
	if message.PlayerID != bp.PlayerID {
		return
	}
	bp.tournamentTableNo = message.TableNo
	bp.tournamentSeatNo = message.SeatNo
	bp.seatNo = message.SeatNo
	bp.logger.Info().Msgf("Tournament [%d] Player [%s] has taken seat %d on table %d.",
		message.TournamentId, bp.GetName(), bp.tournamentSeatNo, bp.tournamentTableNo)
}

func (bp *BotPlayer) tournamentPlayerMoved(message *gamescript.NonProtoTournamentMsg) {
	if message.TableNo == bp.tournamentTableNo {
		bp.needsTournamentTableRefresh = true
	} else if message.NewTableNo == bp.tournamentTableNo {
		bp.needsTournamentTableRefresh = true
	}

	if message.PlayerID != bp.PlayerID {
		return
	}

	bp.tournamentTableNo = message.NewTableNo
	bp.tournamentSeatNo = message.SeatNo
	bp.seatNo = message.SeatNo
	bp.logger.Info().Msgf("Tournament [%d] Player [%s] moved to table %d:%d from table %d.",
		message.TournamentId, bp.GetName(),
		bp.tournamentSeatNo, bp.tournamentTableNo,
		message.CurrentTableNo)
	bp.refreshTournamentTableInfo()
	bp.game = &gameView{
		table: &tableView{
			playersBySeat: make(map[uint32]*player),
			actionTracker: game.NewHandActionTracker(),
			playersActed:  make(map[uint32]*game.PlayerActRound),
		},
		handNum: 1,
	}
	// re-establish connection with new table
	bp.unsubscribe()
	bp.gameCode = bp.tournamentTableInfo.GameCode
	bp.gameID = bp.tournamentTableInfo.GameID
	bp.UpdateLogger()

	playerChannelName := fmt.Sprintf("player.%d", bp.PlayerID)
	err := bp.Subscribe(bp.tournamentTableInfo.GameToPlayerChannel,
		bp.tournamentTableInfo.HandToAllChannel, bp.tournamentTableInfo.HandToPlayerChannel,
		bp.tournamentTableInfo.HandToPlayerTextChannel, playerChannelName)
	if err != nil {
		bp.logger.Error().Msgf("Unable to subscribe to game %s channels", bp.gameCode)
	}

	bp.meToHandSubjectName = bp.tournamentTableInfo.PlayerToHandChannel
	bp.clientAliveSubjectName = bp.tournamentTableInfo.ClientAliveChannel

	bp.logger.Info().Msgf("Starting network check client")
	bp.clientAliveCheck = networkcheck.NewClientAliveCheck(bp.logger, bp.gameID, bp.gameCode, bp.sendAliveMsg)
	bp.clientAliveCheck.Run()
}

func (bp *BotPlayer) tournamentEnded(message *gamescript.NonProtoTournamentMsg) {
	// re-establish connection with new table
	bp.unsubscribe()
	bp.gameCode = bp.tournamentTableInfo.GameCode
	bp.gameID = bp.tournamentTableInfo.GameID
	bp.UpdateLogger()
}

func (bp *BotPlayer) EndTournament() {
	bp.LeaveGameImmediately()
}

func (bp *BotPlayer) refreshTournamentTableInfo() error {
	var err error
	bp.tournamentTableInfo, err = bp.gqlHelper.GetTournamentTableInfo(bp.tournamentID, bp.tournamentTableNo)
	if err != nil {
		bp.logger.Error().Err(err).Msgf("Could not get tournament table info")
		return err
	}
	bp.needsTournamentTableRefresh = false
	return nil
}
