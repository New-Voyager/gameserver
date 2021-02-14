package internal

import "time"

type CrashTest struct {
	gameServerDir   string
	gameServerExec  string
	botRunnerDir    string
	botRunnerExec   string
	testName        string
	botRunnerScript string
	timeOutDuration time.Duration
}

func (t *CrashTest) Run() error {

	return nil
}
