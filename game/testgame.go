package game

//import "time"

func RunTestGame() {
	gameManager := NewGameManager()
	gameNum := gameManager.StartGame(GameType_HOLDEM, "First game", 5)
	player1 := NewPlayer("rob", 1)
	player2 := NewPlayer("steve", 2)
	gameManager.JoinGame(gameNum, player1, 1)
	//time.Sleep(100*time.Millisecond)
	gameManager.JoinGame(gameNum, player2, 2)
	player3 := NewPlayer("larry", 3)
	gameManager.JoinGame(gameNum, player3, 3)
	player4 := NewPlayer("pike", 4)
	gameManager.JoinGame(gameNum, player4, 4)
	player5 := NewPlayer("fish", 5)
	gameManager.JoinGame(gameNum, player5, 5)
	select {}
}
