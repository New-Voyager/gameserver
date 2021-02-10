package app

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"voyager.com/botrunner/internal/game"
	_player "voyager.com/botrunner/internal/player"
	"voyager.com/botrunner/internal/util"
)

var (
	logger = log.With().Str("logger_name", "app::tester").Logger()
)

// Tester is the object that drives the tester application.
type Tester struct {
	config   game.BotRunnerConfig
	gameCode string
	player   *_player.BotPlayer
}

// NewTester creates new instance of Tester.
func NewTester(config game.BotRunnerConfig, gameCode string) (*Tester, error) {
	t := Tester{
		config:   config,
		gameCode: gameCode,
	}

	return &t, nil
}

// Run joins the game and follows it to the end.
func (t *Tester) Run() error {
	logger.Debug().Msgf("Config: %+v", t.config)
	playerConf := t.getPlayerConfig()
	if playerConf == nil {
		return fmt.Errorf("No player found in the setup script")
	}

	playerLogger := log.With().Str("logger_name", "TesterPlayer").Logger()
	player, err := _player.NewBotPlayer(_player.Config{
		Name:          playerConf.Name,
		DeviceID:      playerConf.DeviceID,
		Email:         playerConf.Email,
		Password:      playerConf.Password,
		IsHuman:       !playerConf.Bot,
		APIServerURL:  util.Env.GetAPIServerURL(),
		NatsURL:       util.Env.GetNatsURL(),
		GQLTimeoutSec: 10,
		Script:        t.config,
	}, &playerLogger)
	if err != nil {
		return err
	}
	t.player = player

	err = t.player.Login(playerConf.DeviceID, playerConf.DeviceID)
	if err != nil {
		return errors.Wrap(err, "Unable to login")
	}

	err = t.player.JoinGame(t.gameCode)
	if err != nil {
		return errors.Wrap(err, "Unable to join game")
	}

	for !t.player.IsGameOver() {
		time.Sleep(200 * time.Millisecond)
	}

	return nil
}

// func (t *Tester) joinGame() error {
// 	gameInfo, err := t.player.GetGameInfo()
// 	seatNo := t.getSeatNo(playerConf.Name)
// 	if seatNo == 0 {
// 		return fmt.Errorf("Seat number cannot be 0")
// 	}
// 	err = t.player.JoinGame(t.gameCode, seatNo)
// 	if err != nil {
// 		return err
// 	}

// 	buyInAmount := t.getBuyInAmount(seatNo)
// 	if buyInAmount == 0 {
// 		return fmt.Errorf("Buy in amount cannot be 0")
// 	}

// 	err = t.player.BuyIn(t.gameCode, buyInAmount)
// 	if err != nil {
// 		return err
// 	}
// 	return nil
// }

func (t *Tester) getPlayerConfig() *game.PlayerConfig {
	for _, player := range t.config.Setup.Players {
		if !player.Bot {
			return &player
		}
	}
	return nil
}

// func (t *Tester) getSeatNo(playerName string) uint32 {
// 	for _, sitIn := range t.config.Setup.SitIn {
// 		if sitIn.PlayerName == playerName {
// 			return sitIn.SeatNo
// 		}
// 	}
// 	return 0
// }

// func (t *Tester) getBuyInAmount(seatNo uint32) float32 {
// 	for _, buyIn := range t.config.Setup.BuyIn {
// 		if buyIn.SeatNo == seatNo {
// 			return buyIn.BuyChips
// 		}
// 	}
// 	return 0
// }
