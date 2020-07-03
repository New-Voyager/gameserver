package game

import (
	"fmt"

	"github.com/golang/protobuf/proto"
)

type Manager struct {
	gameCount        uint32
	gameStatePersist PersistGameState
	handStatePersist PersistHandState
	activeGames      map[string]*Game
}

func NewGameManager() *Manager {
	var gamePersist = NewMemoryGameStateTracker()
	var handPersist = NewMemoryHandStateTracker()

	return &Manager{
		gameStatePersist: gamePersist,
		handStatePersist: handPersist,
		activeGames:      make(map[string]*Game),
		gameCount:        0,
	}
}

func (gm *Manager) StartGame(gameType GameType, title string, minPlayers int) uint32 {
	// club id is hard code for now
	clubID := uint32(1)
	gm.gameCount++
	gameID := fmt.Sprintf("%d:%d", clubID, gm.gameCount)
	game := NewPokerGame(gm, gameID, GameType_HOLDEM, clubID, gm.gameCount, minPlayers, gm.gameStatePersist, gm.handStatePersist)
	gm.activeGames[gameID] = game

	go game.runGame()
	return gm.gameCount
}

func (gm *Manager) gameEnded(game *Game) {
	delete(gm.activeGames, game.gameID)
}

func (gm *Manager) SitAtTable(gameNum uint32, player *Player, seatNo uint32, buyIn float32) error {
	clubID := uint32(1)
	gameID := fmt.Sprintf("%d:%d", clubID, gameNum)
	if _, ok := gm.activeGames[gameID]; !ok {
		// game not found
		return fmt.Errorf("Game %d is not found", gameNum)
	}

	game, _ := gm.activeGames[gameID]
	if game.playersInSeatsCount() >= 9 {
		return fmt.Errorf("Game has enough players on the table")
	}
	game.lock.Lock()
	defer game.lock.Unlock()

	// send a SIT message
	takeSeatMessage := GameSitMessage{
		PlayerId: player.playerID,
		SeatNo:   seatNo,
		BuyIn:    buyIn,
	}

	game.allPlayers[player.playerID] = player
	game.players[player.playerID] = player.playerName

	// it looks like circular references are not a problem in golang
	// https://www.reddit.com/r/golang/comments/8jaqyw/circular_references/
	player.game = game
	var gameMessage GameMessage
	gameMessage.ClubId = clubID
	gameMessage.GameNum = gameNum
	gameMessage.PlayerId = player.playerID
	gameMessage.MessageType = PlayerTakeSeat
	gameMessage.GameMessage = &GameMessage_TakeSeat{TakeSeat: &takeSeatMessage}
	messageData, _ := proto.Marshal(&gameMessage)

	go player.playGame()

	game.chGame <- messageData
	return nil
}
