package test

import (
	"testing"
)

func TestTwoPots(t *testing.T) {
	testDriver := NewTestDriver()
	testDriver.RunGameScript("game-scripts/two-pots.yaml")
}

func TestSimpleHand(t *testing.T) {
	testDriver := NewTestDriver()
	testDriver.RunGameScript("game-scripts/simple-hand.yaml")
}

func TestEveryOneAllIn(t *testing.T) {
	testDriver := NewTestDriver()
	testDriver.RunGameScript("game-scripts/everyone-allin.yaml")
}

func TestFlopAction(t *testing.T) {
	testDriver := NewTestDriver()
	testDriver.RunGameScript("game-scripts/flop-action.yaml")
}
