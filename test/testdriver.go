package test

import (
	"fmt"
	"io/ioutil"
	//"time"

	//jsoniter "github.com/json-iterator/go"
	"voyager.com/server/game"
	"github.com/rs/zerolog/log"
	yaml "gopkg.in/yaml.v2"
)

var testDriverLogger = log.With().Str("logger_name", "test::testdriver").Logger()


// runs game scripts and captures the results
// and output the results at the end
type TestDriver struct {

}

func NewTestDriver() *TestDriver {
	return &TestDriver {}
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

	gameScript.configure()

	return nil
}


// configures the table with the configuration
func (g *GameScript) configure() {
	gameType := game.GameType(game.GameType_value[g.GameConfig.GameType])
	g.testGame = NewTestGame(1, gameType, g.GameConfig.Title, g.GameConfig.AutoStart, g.Players)
	g.testGame.Start(g.AssignSeat.Seats)
	//time.Sleep(5000)
	select {}
}