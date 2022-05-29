package app

import (
	"github.com/rs/zerolog"
	"voyager.com/botrunner/internal/driver"
	"voyager.com/logging"
)

// Tournament manages an instance of a tournament object in the bot runner side
type Tournament struct {
	logger *zerolog.Logger
	// players      *gamescript.Players
	// script       *gamescript.Script
	clubCode     string
	tournamentID uint64
	botCount     int32
	instance     *driver.TournamentRunner
	demoGame     bool
}

func NewTournament(clubCode string, tournamentID uint64, botCount int32) (*Tournament, error) {
	b := Tournament{
		logger:       logging.GetZeroLogger("Tournament", nil),
		clubCode:     clubCode,
		tournamentID: tournamentID,
		botCount:     botCount,
	}
	return &b, nil
}

func (t *Tournament) Launch() error {
	t.logger.Info().Msgf("Launching bot runners for tournament %d.", t.tournamentID)
	var err error
	t.instance, err = driver.NewTournamentRunner(t.tournamentID, t.clubCode, t.botCount)
	if err != nil {
		t.logger.Error().Msgf("Launching tournament runner %d failed.", t.tournamentID)
		return err
	}
	err = t.instance.RegisterBots()
	if err != nil {
		t.logger.Error().Msgf("Registering bots for tournament %d failed.", t.tournamentID)
		return err
	}
	err = t.instance.BotsSignIn()
	if err != nil {
		t.logger.Error().Msgf("Registering bots for tournament %d failed.", t.tournamentID)
		return err
	}
	// we are going to register all the known bots to the system
	// the  bots will signup for the tournament
	// bots will listen for the tournament messages
	return err
}
