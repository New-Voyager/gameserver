package timer

func notifyTimeout(t *Timer) {
	timerLogger.Info().Msgf("Notifying timeout %d|%d|%s", t.gameID, t.playerID, t.purpose)
}
