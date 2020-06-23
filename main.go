package main

import (
	"bufio"
	"fmt"
	proto "github.com/golang/protobuf/proto"
	"os"

	//"voyager.com/server/internal"
	"voyager.com/server/game"
	"voyager.com/server/poker"
)

func main() {
	reader := bufio.NewReader(os.Stdin)

	//fmt.Println("Hello New Beginning")
	//TestHoldem2()
	//TestHoldem()
	for {
		//TestOmahaHiLo()
		TestNewGame()
		fmt.Print("Press 'Enter' to get next hand...\n")
		reader.ReadBytes('\n')
	}
	/*
		err := internal.Run(os.Args)
		if err != nil {
			fmt.Printf("Error starting chat server: %v\n", err);
			os.Exit(1)
		}*/
}

func TestHoldem() {
	players := []poker.Player{
		{Name: "Bob", PlayerId: 1},
		{Name: "Dev", PlayerId: 2},
		{Name: "Kamal", PlayerId: 3},
		{Name: "Dave", PlayerId: 4},
		{Name: "Anna", PlayerId: 5},
	}
	pokerTable := poker.NewHoldemTable(players)
	hand := pokerTable.Deal(1)
	result := hand.EvaulateHoldem()
	//fmt.Printf("Result: %v\n", result)
	//playerResult := result.PlayersResult[0]
	fmt.Printf("%s\n", hand.PrettyPrintResult())
	fmt.Printf("Result: \n%s", result.PrettyPrintResult())
}

func TestOmaha() {
	players := []poker.Player{
		{Name: "Bob", PlayerId: 1},
		{Name: "Dev", PlayerId: 2},
		{Name: "Kamal", PlayerId: 3},
		{Name: "Dave", PlayerId: 4},
		{Name: "Anna", PlayerId: 5},
	}
	pokerTable := poker.NewOmahaTable(players)
	hand := pokerTable.Deal(1)
	result := hand.EvaulateOmaha()
	fmt.Printf("%s\n", hand.PrettyPrintResult())
	fmt.Printf("Result: \n%s", result.PrettyPrintResult())
}

func TestOmahaHiLo() {
	players := []poker.Player{
		{Name: "Bob", PlayerId: 1},
		{Name: "Dev", PlayerId: 2},
		{Name: "Kamal", PlayerId: 3},
		{Name: "Dave", PlayerId: 4},
		{Name: "Anna", PlayerId: 5},
	}
	pokerTable := poker.NewOmahaHiLoTable(players)
	hand := pokerTable.Deal(1)
	result := hand.EvaulateOmaha()
	fmt.Printf("%s\n", hand.PrettyPrintResult())
	fmt.Printf("Result: \n%s", result.PrettyPrintResult())
}

func TestHoldem2() {
	/*
		Player: 4  [ 4♦  3♦ ]
		Player: 5  [ 5♠  Q♣ ]
		Community: [ 2♣  Q♦  9♦  A♣  7♦ ]
	*/
	player1 := poker.PlayerHand{
		PlayerId: 1,
		Cards: []poker.Card{
			poker.NewCard("4d"), poker.NewCard("3d"),
		},
	}
	board := []poker.Card{
		poker.NewCard("2c"), poker.NewCard("Qd"),
		poker.NewCard("9d"), poker.NewCard("Ac"), poker.NewCard("7d"),
	}
	playerHands := []poker.PlayerHand{player1}
	hand := poker.NewHand(1, playerHands, board)
	result := hand.EvaulateHoldem()
	fmt.Printf("%s\n", hand.PrettyPrintResult())
	fmt.Printf("Result: \n%s", result.PrettyPrintResult())
}

func TestOmaha1() {
	/*
		Player: 4  [ 4♦  3♦ ]
		Player: 5  [ 5♠  Q♣ ]
		Community: [ 2♣  Q♦  9♦  A♣  7♦ ]
	*/
	player1 := poker.PlayerHand{
		PlayerId: 1,
		Cards: []poker.Card{
			poker.NewCard("4d"), poker.NewCard("3d"),
			poker.NewCard("2c"), poker.NewCard("Ad"),
		},
	}
	board := []poker.Card{
		poker.NewCard("2c"), poker.NewCard("Qd"),
		poker.NewCard("9d"), poker.NewCard("Ac"), poker.NewCard("7d"),
	}
	omahaResult := poker.EvaluateOmaha(player1.Cards, board)
	fmt.Printf("Score: %d, cards: %s, rank: %s",
		omahaResult.HiRank, poker.PrintCards(omahaResult.HiCards), poker.RankString(omahaResult.HiRank))

	//playerHands := []poker.PlayerHand{player1,}
	//hand := poker.NewHand(1, playerHands, board)
	//result := hand.EvaulateHoldem()
	//fmt.Printf("%s\n", hand.PrettyPrintResult())
	//fmt.Printf("Result: \n%s", result.PrettyPrintResult())
}

var gameObject, gameId = game.NewGame()

func TestNewGame() {
	gameObject, err := game.LoadGame(gameId)
	if err != nil {
		fmt.Printf("Game ID: %d is not found. Error: %v", gameId, err)
	}

	_, handID := gameObject.DealNextHand()
	handState := game.LoadHandState(handID)

	handStateProto, err := proto.Marshal(handState)
	//gameStateProto, err := proto.Marshal(handState)

	fmt.Printf("Handstate protobuf Size: %d HandState: %s\n", len(handStateProto), handState.PrettyPrint())
}
