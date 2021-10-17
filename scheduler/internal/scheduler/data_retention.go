package scheduler

import (
	"time"

	"voyager.com/logging"
	"voyager.com/scheduler/internal/util"
)

var (
	dataRetentionLogger = logging.GetZeroLogger("scheduler::data_retention", nil)
)

func DataRetention() {
	interval := time.Duration(util.Env.GetDataRetentionIntervalMin()) * time.Minute
	for range time.NewTicker(interval).C {
		dataRetentionLogger.Debug().Msg("Requesting data retention")
		start := time.Now()
		resp, err := requestDataRetention()
		duration := time.Since(start)
		dataRetentionLogger.Debug().Msgf("Request for data retention took %.3f seconds", duration.Seconds())

		if err != nil {
			dataRetentionLogger.Error().Msgf("Error while requesting data retention: %s", err)
		} else {
			dataRetentionLogger.Info().Msgf("Data retention result: %+v", resp)
		}
	}
}
