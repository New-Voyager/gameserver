package main

import (
	"flag"
	"fmt"

	"github.com/rs/zerolog"
	"voyager.com/server/poker"
	"voyager.com/server/test"
)

func main() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	var runGameScript = flag.String("game-script", "test/game-scripts", "runs tests with game script files")
	if *runGameScript != "" {
		test.RunGameScriptTests(*runGameScript)
	}
}

func testStuff() {
	player1 := poker.CardsInAscii{"Kh", "Qd"}
	player2 := poker.CardsInAscii{"3s", "7s"}
	flop := poker.CardsInAscii{"Ac", "Ad", "2c"}
	turn := poker.NewCard("Td")
	river := poker.NewCard("3s")
	players := make([]poker.CardsInAscii, 2)
	players[0] = player1
	players[1] = player2
	deck := poker.DeckFromScript(players, flop, turn, river)
	//deck := poker.NewDeckNoShuffle()
	deckStr := deck.PrettyPrint()
	fmt.Printf("%s\n", deckStr)
}
