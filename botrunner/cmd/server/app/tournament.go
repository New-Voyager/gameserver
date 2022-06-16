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

func (t *Tournament) Launch(botCount int32) error {
	t.logger.Info().Msgf("Launching bot runners for tournament %d.", t.tournamentID)
	var err error
	t.instance, err = driver.NewTournamentRunner(t.tournamentID, t.clubCode, t.botCount)
	if err != nil {
		t.logger.Error().Msgf("Launching tournament runner %d failed.", t.tournamentID)
		return err
	}
	err = t.instance.CreateBots(botCount)
	if err != nil {
		t.logger.Error().Msgf("Registering bots for tournament %d failed.", t.tournamentID)
		return err
	}
	// we are going to signup all the known bots to the system
	err = t.instance.BotsSignIn()
	if err != nil {
		t.logger.Error().Msgf("Registering bots for tournament %d failed.", t.tournamentID)
		return err
	}

	// register bots to the tournament
	err = t.instance.RegisterBots()
	if err != nil {
		t.logger.Error().Msgf("Registering bots for tournament %d failed.", t.tournamentID)
		return err
	}

	// launch bots message loop
	t.instance.ResetBots()

	// the  bots will signup for the tournament
	// bots will listen for the tournament messages
	return err
}

func (t *Tournament) JoinTournament() error {
	// register bots to the tournament
	err := t.instance.JoinTournament()
	if err != nil {
		t.logger.Error().Msgf("Bots joining tournament %d failed.", t.tournamentID)
		return err
	}
	return nil
}

func (t *Tournament) EndTournament() error {
	// bots leave the tournament
	err := t.instance.EndTournament()
	if err != nil {
		t.logger.Error().Msgf("Ending tournament %d failed.", t.tournamentID)
		return err
	}
	return nil
}
