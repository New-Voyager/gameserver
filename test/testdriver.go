package test

import (
	"fmt"
	"io/ioutil"
	"time"

	"github.com/rs/zerolog/log"
	yaml "gopkg.in/yaml.v2"
	"voyager.com/server/game"
)

var testDriverLogger = log.With().Str("logger_name", "test::testdriver").Logger()

var gameManager = game.NewGameManager()

// runs game scripts and captures the results
// and output the results at the end
type TestDriver struct {
	Observer *TestPlayer
}

func NewTestDriver() *TestDriver {
	return &TestDriver{}
}

func (t *TestDriver) RunGameScript(filename string) error {
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
		return err
	}
	fmt.Printf("Script: %v\n", gameScript)
	gameScript.configure(t)

	return nil
}

// configures the table with the configuration
func (g *GameScript) configure(t *TestDriver) error {
	gameType := game.GameType(game.GameType_value[g.GameConfig.GameType])
	g.testGame = NewTestGame(t, 1, gameType, g.GameConfig.Title, g.GameConfig.AutoStart, g.Players)
	g.testGame.Start(g.AssignSeat.Seats)

	// get current game status
	gameManager.GetTableState(g.testGame.clubID, g.testGame.gameNum, t.Observer.player.PlayerID)
	time.Sleep(100 * time.Millisecond)

	// validate the player stack here to ensure sit-in command worked
	return nil
}
