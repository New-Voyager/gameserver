package player

import (
	"voyager.com/gamescript"
)

func (bp *BotPlayer) processPreActions(preActions []gamescript.PreAction) {
	for _, pa := range preActions {
		if pa.SetupServerCrash.CrashPoint != "" {
			crashPoint := pa.SetupServerCrash.CrashPoint
			err := bp.setupServerCrash(crashPoint)
			if err != nil {
				bp.logger.Fatal().Msgf("%s: Unable to setup game server crash: %s", bp.logPrefix, err)
			}
		}
	}
}
