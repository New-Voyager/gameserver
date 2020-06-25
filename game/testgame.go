package game

func RunTestGame() {
	gameManager := NewGameManager()
	gameNum := gameManager.StartGame(GameType_HOLDEM, "First game", 5)
	player1 := NewPlayer("rob", 1)
	player2 := NewPlayer("steve", 2)
	player3 := NewPlayer("larry", 3)
	player4 := NewPlayer("pike", 4)
	player5 := NewPlayer("fish", 5)
	gameManager.JoinGame(gameNum, player1)
	gameManager.JoinGame(gameNum, player2)
	gameManager.JoinGame(gameNum, player3)
	gameManager.JoinGame(gameNum, player4)
	gameManager.JoinGame(gameNum, player5)
	select {}
}
