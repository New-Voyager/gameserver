package internal

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/pkg/errors"
)

// StandardTest is a basic test case that runs the game server and a bot runner with one game script.
type StandardTest struct {
	gameServerDir    string
	gameServerExec   string
	botRunnerDir     string
	botRunnerExec    string
	testName         string
	botRunnerScript  string
	timeOutDuration  time.Duration
	expectedMsgsFile string
	msgDumpFile      string
}

// NewStandardTest creates an instance of StandardTest.
func NewStandardTest(
	gameServerDir string,
	gameServerExec string,
	botRunnerDir string,
	botRunnerExec string,
	testName string,
	botRunnerScript string,
	timeOutDuration time.Duration,
	expectedMsgsFile string,
	msgDumpFile string,
) *StandardTest {
	return &StandardTest{
		gameServerDir:    gameServerDir,
		gameServerExec:   gameServerExec,
		botRunnerDir:     botRunnerDir,
		botRunnerExec:    botRunnerExec,
		testName:         testName,
		botRunnerScript:  botRunnerScript,
		timeOutDuration:  timeOutDuration,
		msgDumpFile:      msgDumpFile,
		expectedMsgsFile: expectedMsgsFile,
	}
}

// Run runs the test case.
func (t *StandardTest) Run() error {
	// Launch game server.
	fmt.Println("Launching game server")
	gameServerArgs := []string{"--server"}
	gsCmd := exec.Command(t.gameServerExec, gameServerArgs...)
	gsCmd.Dir = t.gameServerDir
	gsCmd.Stdout = os.Stdout
	gsCmd.Stderr = os.Stderr

	if err := gsCmd.Start(); err != nil {
		return errors.Wrap(err, "Error while starting game server")
	}

	defer func() {
		fmt.Println("Stopping game server.")
		gsCmd.Process.Kill()
	}()

	// TODO: Make http call to check if the game server is ready and listening.
	fmt.Println("Waiting for game server to be ready.")
	time.Sleep(5 * time.Second)

	// Launch botrunner with the test script.
	fmt.Println("Launching botrunner")
	botRunnerArgs := []string{"--config", t.botRunnerScript, "--expected-msgs", t.expectedMsgsFile, "--dump-msgs-to", t.msgDumpFile}
	brCmd := exec.Command(t.botRunnerExec, botRunnerArgs...)
	brCmd.Dir = t.botRunnerDir
	brCmd.Stdout = os.Stdout
	brCmd.Stderr = os.Stderr
	time.AfterFunc(t.timeOutDuration, func() {
		fmt.Printf("Test timed out (%s). Killing botrunner.\n", t.timeOutDuration)
		brCmd.Process.Kill()
	})
	if err := brCmd.Run(); err != nil {
		return errors.Wrap(err, "Error from botrunner")
	}

	return nil
}
