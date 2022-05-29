package player

import (
	"encoding/json"
	"fmt"

	natsgo "github.com/nats-io/nats.go"
	"github.com/pkg/errors"
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

// enterGame enters a game without taking a seat as a player.
func (bp *BotPlayer) enterTournament() error {
	tournamentChannelName := fmt.Sprintf("tournament.%d", bp.tournamentID)
	bp.tournamentChannelName = tournamentChannelName
	bp.logger.Info().Msgf("%s: Entering tournament [%d]", bp.logPrefix, bp.tournamentID)
	if bp.tournamentMsgSubscription == nil || !bp.tournamentMsgSubscription.IsValid() {
		bp.logger.Info().Msgf("%s: Subscribing to %s to receive hand messages sent to tournament channel: %s", bp.logPrefix, tournamentChannelName, bp.config.Name)
		sub, err := bp.natsConn.Subscribe(tournamentChannelName, bp.handleTournamentMsg)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("%s: Unable to subscribe to the tournament channel subject [%s]", bp.logPrefix, tournamentChannelName))
		}
		bp.tournamentMsgSubscription = sub
		bp.logger.Info().Msgf("%s: Successfully subscribed to %s.", bp.logPrefix, tournamentChannelName)
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
}
