package main

import (
	"os"
	"testing"
)

func TestScripts(t *testing.T) {
	os.Args = append(os.Args, "--script-tests")
	err := run()
	if err != nil {
		t.Fatalf(err.Error())
	}
}
