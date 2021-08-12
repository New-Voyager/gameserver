package main

import (
	"testing"
)

// TestRunMain is used to run the server within the test binary so
// that we can get the coverage while running the app normally.
// The command line arguments for the app should be passed in to the
// test binary before the test arguments.
//
// For example,
//
// go test -coverpkg=./... -c -o game-server.test
// ./server.test --script-tests -test.coverprofile=coverage.out -test.run TestRunMain voyager.com/server
//
// Here is a good resource on how to build and use the test binary.
// https://www.elastic.co/blog/code-coverage-for-your-golang-system-tests
func TestRunMain(t *testing.T) {
	main()
}
