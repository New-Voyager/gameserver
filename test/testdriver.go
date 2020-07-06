package test

import (
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/rs/zerolog/log"
	yaml "gopkg.in/yaml.v2"
	"voyager.com/server/game"
)

var testDriverLogger = log.With().Str("logger_name", "test::testdriver").Logger()

var gameManager = game.NewGameManager()

type ScriptTestResult struct {
	Filename string
	Passed   bool
	Failures []error
	Disabled bool
}

func (s *ScriptTestResult) addError(e error) {
	s.Failures = append(s.Failures, e)
}

// runs game scripts and captures the results
// and output the results at the end
type TestDriver struct {
	Observer     *TestPlayer
	ScriptResult map[string]*ScriptTestResult
	ScriptFiles  []string
}

func NewTestDriver() *TestDriver {
	return &TestDriver{ScriptResult: make(map[string]*ScriptTestResult), ScriptFiles: make([]string, 0)}
}

func (t *TestDriver) RunGameScript(filename string) error {

	result := &ScriptTestResult{Filename: filename, Failures: make([]error, 0)}
	t.ScriptResult[filename] = result
	t.ScriptFiles = append(t.ScriptFiles, filename)

	// load game script
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		// failed to load game script file
		fmt.Printf("Failed to load file: %s\n", filename)
		return err
	}

	var gameScript GameScript
	err = yaml.Unmarshal(data, &gameScript)
	if err != nil {
		// failed to load game script file
		fmt.Printf("Loading json failed: %s, err: %v\n", filename, err)
		result.addError(err)
		return err
	}
	if gameScript.Disabled {
		result.Disabled = true
		return nil
	}

	gameScript.filename = filename
	gameScript.result = result

	e := gameScript.run(t)
	if e != nil {
		return e
	}
	return nil
}

func (t *TestDriver) ReportResult() bool {
	passed := true
	for _, scriptFile := range t.ScriptFiles {
		result := t.ScriptResult[scriptFile]
		if result.Disabled {
			fmt.Printf("Script %s is disabled\n", result.Filename)
			continue
		}

		if len(result.Failures) != 0 {
			passed = false
			// failed and report errors
			fmt.Printf("Script %s failed\n", scriptFile)
			fmt.Printf("===========================\n")
			for _, e := range result.Failures {
				fmt.Printf("%s\n", e.Error())
			}
			fmt.Printf("===========================\n")
		}
	}
	return passed
}

func RunGameScriptTests(dir string) {
	// runs game scripts and reports results
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		fmt.Printf("Failed to get files from dir: %s\n", dir)
		os.Exit(1)
	}

	testDriver := NewTestDriver()
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		testDriver.RunGameScript(fmt.Sprintf("%s/%s", dir, file.Name()))
	}

	passed := testDriver.ReportResult()
	if passed {
		fmt.Printf("All scripts passed\n")
		os.Exit(0)
	} else {
		fmt.Printf("One or more scripts failed\n")
	}
	time.Sleep(1 * time.Second)
	// if one or more tests failed, the process will exit with an error code
	os.Exit(1)
}
