package game

import (
	"fmt"

	"github.com/golang/protobuf/proto"
)

type GameManager struct {
	gameCount        uint32
	gameStatePersist PersistGameState
	handStatePersist PersistHandState
	activeGames      map[string]*ChannelGame
}

func NewGameManager() *GameManager {
	var gamePersist = NewMemoryGameStateTracker()
	var handPersist = NewMemoryHandStateTracker()

	return &GameManager{
		gameStatePersist: gamePersist,
		handStatePersist: handPersist,
		activeGames:      make(map[string]*ChannelGame),
		gameCount:        0,
	}
}

func (gm *GameManager) StartGame(gameType GameType, title string, minPlayers int) uint32 {
	// club id is hard code for now
	clubID := uint32(1)
	gm.gameCount++
	gameID := fmt.Sprintf("%d:%d", clubID, gm.gameCount)
	game := NewPokerGame(gm, gameID, GameType_HOLDEM, clubID, gm.gameCount, minPlayers, gm.gameStatePersist, gm.handStatePersist)
	gm.activeGames[gameID] = game

	go runGame(game)
	return gm.gameCount
}

func (gm *GameManager) gameEnded(game *ChannelGame) {
	delete(gm.activeGames, game.gameID)
}

func (gm *GameManager) JoinGame(gameNum uint32, player *ChannelPlayer) error {
	clubID := uint32(1)
	gameID := fmt.Sprintf("%d:%d", clubID, gameNum)
	if _, ok := gm.activeGames[gameID]; !ok {
		// game not found
		return fmt.Errorf("Game %d is not found", gameNum)
	}

	game, _ := gm.activeGames[gameID]
	if len(game.activePlayers) >= 9 {
		return fmt.Errorf("Game has enough players on the table")
	}

	// add the user waiting list
	defer game.lock.Unlock()
	game.lock.Lock()
	joinMessage := GameJoinMessage{
		PlayerID: player.playerID,
		ClubID:   clubID,
		GameNum:  gameNum,
	}
	game.waitingPlayers = append(game.waitingPlayers, player)
	messageData, _ := proto.Marshal(&joinMessage)
	game.chManagement <- GameMessage{messageType: MessageJoin, playerID: player.playerID, messageProto: messageData}

	go player.playGame(game.chGame, game.chManagement)
	return nil
}
