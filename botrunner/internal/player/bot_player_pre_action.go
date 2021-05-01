package player

import (
	"time"

	"voyager.com/gamescript"
)

func (bp *BotPlayer) processPreActions(preActions []gamescript.PreAction) {
	for _, pa := range preActions {
		if pa.SetupServerCrash.CrashPoint == "" {
			continue
		}

		crashPoint := pa.SetupServerCrash.CrashPoint
		bp.setupServerCrashWithRetry(crashPoint, bp.PlayerID, 30)
	}
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
		bp.logger.Error().Msgf("%s: Error while setting up game server crash: %v", bp.logPrefix, err)
		time.Sleep(2 * time.Second)
		err = bp.setupServerCrash(crashPoint, playerID)
		retries++
	}
	if err != nil {
		bp.logger.Fatal().Msgf("%s: Unable to setup game server crash: %s", bp.logPrefix, err)
	}
}
