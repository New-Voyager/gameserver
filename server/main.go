package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"
	natsgo "github.com/nats-io/nats.go"
	"github.com/pkg/errors"

	"voyager.com/server/crashtest"
	"voyager.com/server/game"
	rpc "voyager.com/server/grpc"
	"voyager.com/server/nats"
	"voyager.com/server/rest"
	"voyager.com/server/util"
	"voyager.com/server/util/random"
	"voyager.com/server/util/simulation"

	"github.com/rs/zerolog"
	"voyager.com/logging"
	"voyager.com/server/poker"
	"voyager.com/server/test"
)

var runServer *bool
var runGameScriptTests *bool
var gameScriptsFileOrDir *string
var delayConfigFile *string
var testName *string
var testDeal *bool
var numDeals *uint
var exit bool
var mainLogger = logging.GetZeroLogger("main::main", nil)
var rpcPort = 9000

func init() {
	runServer = flag.Bool("server", true, "runs game server")
	runGameScriptTests = flag.Bool("script-tests", false, "runs script tests")
	gameScriptsFileOrDir = flag.String("game-script", "test/game-scripts", "runs tests with game script files")
	delayConfigFile = flag.String("delays", "delays.yaml", "YAML file containing pause times")
	testName = flag.String("testname", "", "runs a specific test")
	testDeal = flag.Bool("test-deal", false, "deals and counts ranks")
	numDeals = flag.Uint("num-deals", 100000, "number of test deals when -test-deal is set")
}

func main() {
	// Global random seed that is used by all games.
	rand.Seed(random.NewSeed())

	err := run()
	if err != nil {
		mainLogger.Error().Msg(err.Error())
		os.Exit(1)
	}
}

func run() error {
	logLevel := util.Env.GetZeroLogLogLevel()
	fmt.Printf("Setting log level to %s\n", logLevel)
	zerolog.SetGlobalLevel(logLevel)
	flag.Parse()
	if *testDeal {
		return simulation.Run(int(*numDeals))
	}

	delays, err := game.ParseDelayConfig(*delayConfigFile)
	if err != nil {
		return errors.Wrap(err, "Error while parsing delay config")
	}

	if !*runGameScriptTests {
		apiServerURL := util.Env.GetApiServerInternalURL()
		waitForAPIServer(apiServerURL)
	}

	// create game manager
	gameManager, err := game.CreateGameManager(*runGameScriptTests, delays)
	if err != nil {
		return errors.Wrap(err, "Error while creating game manager")
	}

	if *runGameScriptTests {
		return testScripts()
	}

	runWithNats(gameManager)
	return nil
}

func waitForAPIServer(apiServerURL string) {
	readyURL := fmt.Sprintf("%s/internal/ready", apiServerURL)
	client := http.Client{Timeout: 2 * time.Second}
	for {
		mainLogger.Info().Msgf("Checking API server ready (%s)", readyURL)
		resp, err := client.Get(readyURL)
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			break
		}
		if err == nil {
			mainLogger.Error().Msgf("%s returend %d", readyURL, resp.StatusCode)
			resp.Body.Close()
		}
		time.Sleep(1 * time.Second)
	}
}

func runWithNats(gameManager *game.Manager) {
	if util.Env.IsSystemTest() {
		mainLogger.Warn().Msg("Running in system test mode.")
	}

	mainLogger.Info().Msg("Running the server with NATS")
	natsURL := util.Env.GetNatsURL()
	mainLogger.Info().Msgf("NATS URL: %s", natsURL)

	nc, err := natsgo.Connect(natsURL)
	if err != nil {
		mainLogger.Error().Msgf("Error connecting to NATS server, error: %v", err)
		return
	}
	natsGameManager, err := nats.NewGameManager(nc)
	// initialize nats game manager
	if err != nil {
		mainLogger.Error().Msgf("Error creating NATS game manager, error: %v", err)
		return
	}

	gameManager.SetCrashHandler(natsGameManager.CrashCleanup)

	listener, err := nats.NewNatsDriverBotListener(nc, natsGameManager)
	if err != nil {
		mainLogger.Error().Msgf("Error when creating NatsDriverBotListener, error: %v", err)
		return
	}
	_ = listener

	apiServerURL := util.Env.GetApiServerInternalURL()

	// subscribe to api server events
	nats.RegisterGameServer(apiServerURL, natsGameManager)

	crashtest.SetExitFunc(setupExit)

	// run rest server
	go rest.RunRestServer(natsGameManager, setupExit)

	// run grpc server
	go rpc.Start(rpcPort)

	// restart games
	time.Sleep(3 * time.Second)
	mainLogger.Info().Msg("Requesting to restart the active games.")
	err = nats.RequestRestartGames(apiServerURL)
	if err != nil {
		mainLogger.Error().Msg("Error while requesting to restart active games")
	}

	if util.Env.IsSystemTest() {
		// System test needs a way to return from main to collect the code coverage.
		// This is only for testing. We shouldn't be exiting the server in production.
		for !exit {
			time.Sleep(500 * time.Millisecond)
		}
		// Give another 0.5 sec to make sure the rest api call that triggered the
		// exit has sent back the response before exiting.
		time.Sleep(500 * time.Millisecond)
	} else {
		select {}
	}
}

func setupExit() {
	exit = true
}

func testScripts() error {
	if *gameScriptsFileOrDir != "" {
		err := test.RunGameScriptTests(*gameScriptsFileOrDir, *testName)
		if err != nil {
			return err
		}
	}
	return nil
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
	deck := poker.DeckFromScript(players, flop, turn, river, true /* burn card */)
	//deck := poker.NewDeckNoShuffle()
	deckStr := deck.PrettyPrint()
	fmt.Printf("%s\n", deckStr)
}

func TestOmaha() {
	players := []poker.Player{
		{Name: "Bob", PlayerId: 1},
		{Name: "Dev", PlayerId: 2},
		{Name: "Kamal", PlayerId: 3},
		{Name: "Dave", PlayerId: 4},
		{Name: "Anna", PlayerId: 5},
		{Name: "Aditya", PlayerId: 6},
		{Name: "Ajay", PlayerId: 7},
		{Name: "Aaron", PlayerId: 8},
		//{Name: "Kapil", PlayerId: 9},
	}
	rank := []string{
		"Royal Flush",
		"Straight Flush",
		"Four of a Kind",
		"Full House",
		"Flush",
		"Straight",
		"Three of a Kind",
		"Two Pair",
		"Pair",
		"High Card",
	}
	noOfDecks := []int{2}
	noOfHands := 200
	fmt.Printf("\n")
	for _, deckCount := range noOfDecks {
		pokerTable := poker.NewOmahaTable(players, 5)
		winnerRank := map[string]uint64{
			"Straight Flush":  0,
			"Four of a Kind":  0,
			"Full House":      0,
			"Flush":           0,
			"Straight":        0,
			"Three of a Kind": 0,
			"Two Pair":        0,
			"Pair":            0,
			"High Card":       0,
			//"N/A":             0,
		}

		playerRank := map[string]uint64{
			"Royal Flush":     0,
			"Straight Flush":  0,
			"Four of a Kind":  0,
			"Full House":      0,
			"Flush":           0,
			"Straight":        0,
			"Three of a Kind": 0,
			"Two Pair":        0,
			"Pair":            0,
			"High Card":       0,
			//"N/A":             0,
		}
		straightFlushes := make([]string, 0)
		fmt.Printf("Number of decks: %d, Number of players in the table: %d, Game hands: %d\n",
			deckCount, len(players), noOfHands)
		noShowDowns := 0
		for i := 0; i < noOfHands; i++ {
			showDown := rand.Uint32() % 3
			hand := pokerTable.Deal(1)
			result := hand.EvaulateOmaha()
			if showDown == 0 {
				winnerRank[result.HiRankStr()]++
				noShowDowns++
				for _, playerResult := range result.PlayersResult {
					rankStr := poker.RankString(playerResult.Rank)
					if rankStr == "Straight Flush" {
						straightFlushes = append(straightFlushes, poker.PrintCards(playerResult.BestCards))
						if playerResult.Rank == 1 {
							playerRank["Royal Flush"]++
						} else {
							playerRank[rankStr]++
						}
					} else {
						playerRank[rankStr]++
					}
				}
			}
		}
		odds := map[string]float64{
			"Royal Flush":     0,
			"Straight Flush":  0,
			"Four of a Kind":  0,
			"Full House":      0,
			"Flush":           0,
			"Straight":        0,
			"Three of a Kind": 0,
			"Two Pair":        0,
			"Pair":            0,
			"High Card":       0,
			//"N/A":             0,
		}
		winnerOdds := map[string]float64{
			"Royal Flush":     0,
			"Straight Flush":  0,
			"Four of a Kind":  0,
			"Full House":      0,
			"Flush":           0,
			"Straight":        0,
			"Three of a Kind": 0,
			"Two Pair":        0,
			"Pair":            0,
		}

		for key := range playerRank {
			odds[key] = float64(playerRank[key]) / (float64(len(players)) * float64(noOfHands))
		}

		for key := range winnerRank {
			winnerOdds[key] = float64(winnerRank[key]) / float64(noOfHands)
		}
		fmt.Printf("Number of player hands: %d , number of showdowns: %d hands in the showdowns: %d\n",
			noOfHands*len(players), noShowDowns, noShowDowns*len(players))
		for _, key := range rank {
			fmt.Printf("|%-15s|%6d|%6.6f\n", key, playerRank[key], odds[key])
		}

		if len(straightFlushes) > 0 {
			fmt.Printf("\n\nStraight flushes:\n")
			for _, flush := range straightFlushes {
				fmt.Printf("%s\n", flush)
			}
		}
	}

	/*
			1: "Straight Flush",
		2: "Four of a Kind",
		3: "Full House",
		4: "Flush",
		5: "Straight",
		6: "Three of a Kind",
		7: "Two Pair",
		8: "Pair",
		9: "High Card",*/

}
