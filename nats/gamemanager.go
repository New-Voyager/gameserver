package nats

import "fmt"

// This game manager is similar to game.GameManager.
// However, this game manager active NatsGame objects.
// This will cleanup a NatsGame object and removes when the game ends.
type GameManager struct {
	activeGames map[string]*NatsGame
}

var natsGameManager = &GameManager{
	activeGames: make(map[string]*NatsGame),
}

func initializeNatsGame(clubID uint32, gameNum uint32) (*NatsGame, error) {
	gameID := fmt.Sprintf("%d:%d", clubID, gameNum)
	game, err := NewGame(clubID, gameNum)
	if err != nil {
		return nil, err
	}
	natsGameManager.activeGames[gameID] = game
	return game, nil
}

func endNatsGame(clubID uint32, gameNum uint32) {
	gameID := fmt.Sprintf("%d:%d", clubID, gameNum)
	if game, ok := natsGameManager.activeGames[gameID]; ok {
		game.cleanup()
		delete(natsGameManager.activeGames, gameID)
	}
}
