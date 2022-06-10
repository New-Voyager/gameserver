package player

import (
	"fmt"
	"time"

	"github.com/google/go-cmp/cmp"
	"voyager.com/botrunner/internal/game"
	"voyager.com/gamescript"
)

func (bp *BotPlayer) processPreActions(seatAction *game.NextSeatAction, preActions []gamescript.PreAction) {
	for _, pa := range preActions {
		errs := bp.verifyPreAction(seatAction, pa.Verify)
		if errs != nil {
			bp.logger.Error().Msgf("Pre action verification failed.")
			for _, e := range errs {
				bp.logger.Error().Err(e).Msg("Pre action verification")
			}
			panic("Pre action verification failed. Please check the logs.")
		}

		if pa.SetupServerCrash.CrashPoint != "" {
			crashPoint := pa.SetupServerCrash.CrashPoint
			bp.setupServerCrashWithRetry(crashPoint, bp.PlayerID, 30)
		}
	}
}

func (bp *BotPlayer) verifyPreAction(seatAction *game.NextSeatAction, verify gamescript.YourActionVerification) []error {
	var errs []error
	if verify.AvailableActions != nil {
		actionStrs := make([]string, 0)
		for _, a := range seatAction.AvailableActions {
			actionStrs = append(actionStrs, a.String())
		}
		if !cmp.Equal(actionStrs, verify.AvailableActions) {
			errs = append(errs, fmt.Errorf("Available actions: %v, Expected: %v", actionStrs, verify.AvailableActions))
		}
	}
	if verify.BetOptions != nil {
		if len(verify.BetOptions) != len(seatAction.BetOptions) {
			errs = append(errs, fmt.Errorf("Bet options length: %d, Expected: %d", len(seatAction.BetOptions), len(verify.BetOptions)))
		}
		for i := 0; i < len(verify.BetOptions); i++ {
			ex := verify.BetOptions[i]
			ac := seatAction.BetOptions[i]
			if ac.Text != ex.Text || ac.Amount != ex.Amount {
				errs = append(errs, fmt.Errorf("Bet options: %v, Expected: %v", seatAction.BetOptions, verify.BetOptions))
				break
			}
		}
	}
	if verify.StraddleAmount != nil {
		ex := *verify.StraddleAmount
		ac := seatAction.StraddleAmount
		if ac != ex {
			errs = append(errs, fmt.Errorf("StraddleAmount: %v, Expected: %v", ac, ex))
		}
	}
	if verify.CallAmount != nil {
		ex := *verify.CallAmount
		ac := seatAction.CallAmount
		if ac != ex {
			errs = append(errs, fmt.Errorf("CallAmount: %v, Expected: %v", ac, ex))
		}
	}
	if verify.RaiseAmount != nil {
		ex := *verify.RaiseAmount
		ac := seatAction.RaiseAmount
		if ac != ex {
			errs = append(errs, fmt.Errorf("RaiseAmount: %v, Expected: %v", ac, ex))
		}
	}
	if verify.MinBetAmount != nil {
		ex := *verify.MinBetAmount
		ac := seatAction.MinBetAmount
		if ac != ex {
			errs = append(errs, fmt.Errorf("MinBetAmount: %v, Expected: %v", ac, ex))
		}
	}
	if verify.MaxBetAmount != nil {
		ex := *verify.MaxBetAmount
		ac := seatAction.MaxBetAmount
		if ac != ex {
			errs = append(errs, fmt.Errorf("MaxBetAmount: %v, Expected: %v", ac, ex))
		}
	}
	if verify.MinRaiseAmount != nil {
		ex := *verify.MinRaiseAmount
		ac := seatAction.MinRaiseAmount
		if ac != ex {
			errs = append(errs, fmt.Errorf("MinRaiseAmount: %v, Expected: %v", ac, ex))
		}
	}
	if verify.MaxRaiseAmount != nil {
		ex := *verify.MaxRaiseAmount
		ac := seatAction.MaxRaiseAmount
		if ac != ex {
			errs = append(errs, fmt.Errorf("MaxRaiseAmount: %v, Expected: %v", ac, ex))
		}
	}
	if verify.AllInAmount != nil {
		ex := *verify.AllInAmount
		ac := seatAction.AllInAmount
		if ac != ex {
			errs = append(errs, fmt.Errorf("AllInAmount: %v, Expected: %v", ac, ex))
		}
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

func (bp *BotPlayer) processPreDealItems(preDealItems []gamescript.PreDealSetup) {
	for _, pd := range preDealItems {
		if pd.SetupServerCrash.CrashPoint == "" {
			continue
		}

		crashPoint := pd.SetupServerCrash.CrashPoint
		bp.setupServerCrashWithRetry(crashPoint, 0, 30)
	}
}

func (bp *BotPlayer) setupServerCrashWithRetry(crashPoint string, playerID uint64, maxRetries int) {
	err := bp.setupServerCrash(crashPoint, playerID)
	retries := 0
	for err != nil && retries < maxRetries {
		bp.logger.Warn().Msgf("Could not setup game server crash: %v", err)
		time.Sleep(2 * time.Second)
		err = bp.setupServerCrash(crashPoint, playerID)
		retries++
	}
	if err != nil {
		bp.logger.Fatal().Msgf("Unable to setup game server crash: %s", err)
	}
}

func (bp *BotPlayer) setupButtonPos(buttonPos uint32) error {
	// separate REST API to setup the button position
	return bp.restHelper.UpdateButtonPos(bp.gameCode, buttonPos)
}

func (bp *BotPlayer) SetupServerSettings(serverSettings *gamescript.ServerSettings) error {
	return bp.restHelper.SetServerSettings(serverSettings)
}

func (bp *BotPlayer) ResetServerSettings() error {
	return bp.restHelper.ResetServerSettings()
}

func (bp *BotPlayer) BuyAppCoins(amount int) error {
	return bp.restHelper.BuyAppCoins(bp.PlayerUUID, amount)
}
