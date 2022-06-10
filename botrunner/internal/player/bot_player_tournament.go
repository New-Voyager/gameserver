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

// enterTournament enters a game without taking a seat as a player.
func (bp *BotPlayer) enterTournament() error {
	var e error
	bp.tournamentInfo, e = bp.gqlHelper.GetTournamentInfo(bp.tournamentID)
	if e != nil {
		return errors.Wrapf(e, "Error getting tournament info for tournament [%d]", bp.tournamentID)
	}

	bp.logger.Info().Msgf("%s: Entering tournament [%d]", bp.logPrefix, bp.tournamentID)
	if bp.tournamentMsgSubscription == nil || !bp.tournamentMsgSubscription.IsValid() {
		bp.logger.Info().Msgf("%s: Subscribing to %s to receive hand messages sent to tournament channel: %s", bp.logPrefix, bp.tournamentInfo.TournamentChannel, bp.config.Name)
		sub, err := bp.natsConn.Subscribe(bp.tournamentInfo.TournamentChannel, bp.handleTournamentMsg)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("%s: Unable to subscribe to the tournament channel subject [%s]", bp.logPrefix, bp.tournamentInfo.TournamentChannel))
		}
		bp.tournamentMsgSubscription = sub
		bp.logger.Info().Msgf("%s: Successfully subscribed to %s.", bp.logPrefix, bp.tournamentInfo.TournamentChannel)
	}

	return nil
}

func (bp *BotPlayer) handleTournamentMsg(msg *natsgo.Msg) {
	if bp.printTournamentMsg {
		bp.logger.Info().Msgf("%s: Received game message %s", bp.logPrefix, string(msg.Data))
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

func (bp *BotPlayer) processTournamentMessage(message *TournamentMessageChannelItem) {
	if bp.IsErrorState() {
		bp.logger.Info().Msgf("%s: Bot is in error state. Ignoring hand message.", bp.logPrefix)
		return
	}

	if message.NonProtoMsg != nil {
		bp.processTournamentNonProtoMsg(message.NonProtoMsg)
	}
}

func (bp *BotPlayer) processTournamentNonProtoMsg(message *gamescript.NonProtoTournamentMsg) {
	if bp.IsErrorState() {
		bp.logger.Info().Msgf("%s: Bot is in error state. Ignoring hand message.", bp.logPrefix)
		return
	}
	fmt.Printf("[%s] HANDLING TOURNAMENT MESSAGE: %+v\n", bp.logPrefix, message.Type)
	// if util.Env.ShouldPrintTournamentMsg() {
	// 	fmt.Printf("[%s] HANDLING TOURNAMENT MESSAGE: %+v\n", bp.logPrefix, message.Type)
	// }

	switch message.Type {
	case "TOURNAMENT_STARTED":
		bp.tournamentStarted(message.TournamentId)
	case "TOURNAMENT_INITIAL_PLAYER_TABLE":
		bp.setTournamentPlayerSeat(message)
	case "TOURNAMENT_PLAYER_MOVED_TABLE":
		bp.tournamentPlayerMoved(message)
	}
}

func (bp *BotPlayer) tournamentStarted(tournamentID uint64) {
	bp.logger.Info().Msgf("%s: Tournament started. Tournament ID [%d]", bp.logPrefix, tournamentID)
	bp.tournamentID = tournamentID
	var e error
	bp.tournamentTableInfo, e = bp.gqlHelper.GetTournamentTableInfo(bp.tournamentID, bp.tournamentTableNo)
	if e != nil {
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

	playerChannelName := fmt.Sprintf("player.%d", bp.PlayerID)
	var err error
	err = bp.Subscribe(bp.tournamentTableInfo.GameToPlayerChannel,
		bp.tournamentTableInfo.HandToAllChannel, bp.tournamentTableInfo.HandToPlayerChannel,
		bp.tournamentTableInfo.HandToPlayerTextChannel, playerChannelName)
	if err != nil {
		// return errors.Wrap(err, fmt.Sprintf("%s: Unable to subscribe to game %s channels",
		// 	bp.logPrefix, bp.gameCode))
	}

	bp.meToHandSubjectName = bp.tournamentTableInfo.PlayerToHandChannel
	bp.clientAliveSubjectName = bp.tournamentTableInfo.ClientAliveChannel

	bp.logger.Info().Msgf("%s: Starting network check client", bp.logPrefix)
	bp.clientAliveCheck = networkcheck.NewClientAliveCheck(bp.logger, bp.logPrefix, bp.gameID, bp.gameCode, bp.sendAliveMsg)
	bp.clientAliveCheck.Run()
}

func (bp *BotPlayer) setTournamentPlayerSeat(message *gamescript.NonProtoTournamentMsg) {
	if message.PlayerID != bp.PlayerID {
		return
	}
	bp.tournamentTableNo = message.TableNo
	bp.tournamentSeatNo = message.SeatNo
	bp.seatNo = message.SeatNo
	bp.logger.Info().Msgf("%s: Tournament [%d] Player [%s] has taken seat %d on table %d.",
		bp.logPrefix, message.TournamentId, bp.GetName(), bp.tournamentSeatNo, bp.tournamentTableNo)
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
	bp.logger.Info().Msgf("%s: Tournament [%d] Player [%s] moved to table %d:%d from table %d.",
		bp.logPrefix, message.TournamentId, bp.GetName(),
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

	playerChannelName := fmt.Sprintf("player.%d", bp.PlayerID)
	err := bp.Subscribe(bp.tournamentTableInfo.GameToPlayerChannel,
		bp.tournamentTableInfo.HandToAllChannel, bp.tournamentTableInfo.HandToPlayerChannel,
		bp.tournamentTableInfo.HandToPlayerTextChannel, playerChannelName)
	if err != nil {
		bp.logger.Error().Msgf("%s: Unable to subscribe to game %s channels",
			bp.logPrefix, bp.gameCode)
	}

	bp.meToHandSubjectName = bp.tournamentTableInfo.PlayerToHandChannel
	bp.clientAliveSubjectName = bp.tournamentTableInfo.ClientAliveChannel

	bp.logger.Info().Msgf("%s: Starting network check client", bp.logPrefix)
	bp.clientAliveCheck = networkcheck.NewClientAliveCheck(bp.logger, bp.logPrefix, bp.gameID, bp.gameCode, bp.sendAliveMsg)
	bp.clientAliveCheck.Run()
}

func (bp *BotPlayer) refreshTournamentTableInfo() error {
	var err error
	bp.tournamentTableInfo, err = bp.gqlHelper.GetTournamentTableInfo(bp.tournamentID, bp.tournamentTableNo)
	if err != nil {
		bp.logger.Error().Err(err).Msgf("%s: Could not get tournament table info", bp.logPrefix)
		return err
	}
	bp.needsTournamentTableRefresh = false
	return nil
}
