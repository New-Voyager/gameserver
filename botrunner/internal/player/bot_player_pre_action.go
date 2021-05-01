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
		err := bp.setupServerCrash(crashPoint)
		attempts := 1
		for err != nil && attempts < 30 {
			time.Sleep(2 * time.Second)
			attempts++
			err = bp.setupServerCrash(crashPoint)
		}
		if err != nil {
			bp.logger.Fatal().Msgf("%s: Unable to setup game server crash: %s", bp.logPrefix, err)
		}
	}
}
