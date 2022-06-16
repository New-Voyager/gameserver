package driver

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"voyager.com/botrunner/internal/player"
	"voyager.com/botrunner/internal/util"
	"voyager.com/gamescript"
	"voyager.com/logging"
)

type TournamentRunner struct {
	logger            *zerolog.Logger
	tournamentID      uint64
	botCount          uint32
	bots              []*player.BotPlayer
	observerBot       *player.BotPlayer
	botsByName        map[string]*player.BotPlayer
	tables            []*TournamentTable
	tournamentChannel string
}

type TournamentTable struct {
	logger *zerolog.Logger
	bots   []*player.BotPlayer
}

var TOURNAMENT_DEVICE_START_ID = "f0a675ef-0000-4963-%04x-75a7d1735665"

func NewTournamentRunner(tournamentID uint64, clubCode string, botCount int32) (*TournamentRunner, error) {
	return &TournamentRunner{
		logger:            logging.GetZeroLogger("TournamentRunner", nil),
		tournamentID:      tournamentID,
		botCount:          uint32(botCount),
		bots:              make([]*player.BotPlayer, 0),
		botsByName:        make(map[string]*player.BotPlayer),
		tables:            make([]*TournamentTable, 0),
		tournamentChannel: fmt.Sprintf("tournament-%d", tournamentID),
	}, nil
}

// register players for the tournament
// launch a go routine to listen for tournament messages
// when the bots join the tournament, we will start go routine for each bot to listen for hand/game messages

// RegisterBots registers the bots for the tournament
func (tr *TournamentRunner) CreateBots(botCount int32) error {
	fileName := "botrunner_scripts/players/tournament-players.csv"
	bytes, err := ioutil.ReadFile(fileName)
	if err != nil {
		return errors.Wrapf(err, "Error reading players configuration file [%s]", fileName)
	}
	botNames := strings.Split(string(bytes), "\n")
	minActionDelay := uint32(1000)
	maxActionMillis := uint32(1000)

	// Create the player bots based on the setup script.
	for i, botName := range botNames {
		deviceID := fmt.Sprintf(TOURNAMENT_DEVICE_START_ID, i)
		email := fmt.Sprintf("%s@bot.net", botName)
		password := "password"
		gps := gamescript.GpsLocation{Lat: 0, Long: 0}
		ip := "10.0.0.1"
		bot, err := player.NewBotPlayer(player.Config{
			Name:            botName,
			DeviceID:        deviceID,
			Email:           email,
			Password:        password,
			Gps:             &gps,
			IpAddress:       ip,
			IsHost:          false,
			IsHuman:         false,
			MinActionDelay:  minActionDelay,
			MaxActionDelay:  maxActionMillis,
			APIServerURL:    util.Env.GetAPIServerURL(),
			NatsURL:         util.Env.GetNatsURL(),
			GQLTimeoutSec:   util.Env.GetGQLTimeoutSec(),
			IsTournamentBot: true,
		}, os.Stdout)
		if err != nil {
			tr.logger.Info().Msgf("Unable to create bot %s", botName)

			continue
		}
		tr.bots = append(tr.bots, bot)
		tr.botsByName[botName] = bot
		if i == int(botCount) {
			break
		}
	}
	return nil
}

func (tr *TournamentRunner) BotsSignIn() error {
	// Register bots to the poker service.
	for _, b := range tr.bots {
		var err error
		signedIn := false
		maxAttempts := 5
		for attempts := 0; attempts < maxAttempts && !signedIn; attempts++ {
			if attempts > 0 {
				tr.logger.Info().Msgf("%s could not sign in (%d/%d)", b.GetName(), attempts, maxAttempts)
			}
			// Try logging in first. The bot player might've already signed up from some other game.
			err = b.Login()
			if err == nil {
				signedIn = true
				break
			}
			// This bot has never signed up. Go ahead and sign up.
			err = b.SignUp()
			if err == nil {
				signedIn = true
				break
			}
			time.Sleep(2 * time.Second)
		}
		if !signedIn {
			tr.logger.Error().Msgf("%s cannot sign in", b.GetName())
		}
	}

	return nil
}

func (tr *TournamentRunner) RegisterBots() error {
	var err error
	// register bots for the tournament
	for i, b := range tr.bots {
		if i >= int(tr.botCount) {
			// reached max number of bots
			break
		}

		err = b.RegisterTournament(tr.tournamentID)
		if err != nil {
			return errors.Wrapf(err, "%s cannot register for tournament", b.GetName())
		}
	}
	return nil
}

func (br *TournamentRunner) ResetBots() {
	for _, bot := range br.bots {
		bot.Reset()
	}
	//br.observerBot.Reset()
}

func (br *TournamentRunner) EndTournament() error {
	for _, bot := range br.bots {
		bot.EndTournament()
	}
	return nil
}

func (tr *TournamentRunner) JoinTournament() error {
	var err error
	// register bots for the tournament
	for i, b := range tr.bots {
		if i >= int(tr.botCount) {
			// reached max number of bots
			break
		}

		err = b.JoinTournament(tr.tournamentID)
		if err != nil {
			return errors.Wrapf(err, "%s cannot join tournament %d", b.GetName(), tr.tournamentID)
		}
	}
	return nil
}
