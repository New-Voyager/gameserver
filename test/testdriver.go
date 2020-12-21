package test

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/rs/zerolog/log"
	"gopkg.in/godo.v2/glob"
	yaml "gopkg.in/yaml.v2"
	"voyager.com/server/game"
)

var testDriverLogger = log.With().Str("logger_name", "test::testdriver").Logger()

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
	ScriptResult map[string]*ScriptTestResult
	ScriptFiles  []string
}

func NewTestDriver() *TestDriver {
	return &TestDriver{ScriptResult: make(map[string]*ScriptTestResult), ScriptFiles: make([]string, 0)}
}

func (t *TestDriver) RunGameScript(filename string) error {
	fmt.Printf("Running game script: %s\n", filename)
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

	var gameScript game.GameScript
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

	testGameScript := TestGameScript{
		gameScript: &gameScript,
		filename:   filename,
		result:     result,
	}

	e := testGameScript.run(t)
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

func RunGameScriptTests(dir string, testName string) {

	pattern := fmt.Sprintf("%s/**/*.yaml", dir)
	patterns := []string{pattern}
	files, _, err := glob.Glob(patterns)
	// runs game scripts and reports results
	if err != nil {
		fmt.Printf("Failed to get files from dir: %s\n", dir)
		os.Exit(1)
	}

	testDriver := NewTestDriver()
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if testName != "" {
			if !strings.Contains(file.Name(), testName) {
				continue
			}
		}
		fmt.Printf("----------------------------------------------\n")
		testDriver.RunGameScript(file.Path)
		fmt.Printf("----------------------------------------------\n")
	}

	passed := testDriver.ReportResult()
	if passed {
		fmt.Printf("All scripts passed\n")
		os.Exit(0)
	} else {
		fmt.Printf("One or more scripts failed\n")
	}
	// if one or more tests failed, the process will exit with an error code
	os.Exit(1)
}
