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

func (gm *Manager) InitializeGame(clubID uint32, gameType GameType,
	title string, minPlayers int, autoStart bool, autoDeal bool) uint32 {
	gm.gameCount++
	gameID := fmt.Sprintf("%d:%d", clubID, gm.gameCount)
	game := NewPokerGame(gm, gameID,
		GameType_HOLDEM,
		clubID, gm.gameCount,
		minPlayers,
		autoStart,
		autoDeal,
		gm.gameStatePersist,
		gm.handStatePersist)
	gm.activeGames[gameID] = game

	go game.runGame()
	return gm.gameCount
}

func (gm *Manager) gameEnded(game *Game) {
	delete(gm.activeGames, game.gameID)
}

func (gm *Manager) SitAtTable(clubID uint32, gameNum uint32, player *Player, seatNo uint32, buyIn float32) error {
	//clubID := uint32(1)
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
		PlayerId: player.PlayerID,
		SeatNo:   seatNo,
		BuyIn:    buyIn,
	}

	game.allPlayers[player.PlayerID] = player
	game.players[player.PlayerID] = player.PlayerName

	// it looks like circular references are not a problem in golang
	// https://www.reddit.com/r/golang/comments/8jaqyw/circular_references/
	player.game = game
	var gameMessage GameMessage
	gameMessage.ClubId = clubID
	gameMessage.GameNum = gameNum
	gameMessage.PlayerId = player.PlayerID
	gameMessage.MessageType = PlayerTakeSeat
	gameMessage.GameMessage = &GameMessage_TakeSeat{TakeSeat: &takeSeatMessage}
	messageData, _ := proto.Marshal(&gameMessage)

	go player.playGame()

	game.chGame <- messageData
	return nil
}

func (gm *Manager) StartGame(clubID uint32, gameNum uint32) error {
	gameID := fmt.Sprintf("%d:%d", clubID, gm.gameCount)
	if _, ok := gm.activeGames[gameID]; !ok {
		// game not found
		return fmt.Errorf("Game %d is not found", gameNum)
	}
	game, _ := gm.activeGames[gameID]

	var gameMessage GameMessage

	statusChangeMessage := &GameStatusChangeMessage{
		NewStatus: GameStatus_START_GAME_RECEIVED,
	}

	gameMessage.ClubId = clubID
	gameMessage.GameNum = gameNum
	gameMessage.MessageType = GameStatusChanged
	gameMessage.GameMessage = &GameMessage_StatusChange{StatusChange: statusChangeMessage}
	messageData, _ := proto.Marshal(&gameMessage)

	game.chGame <- messageData
	return nil
}


func (gm *Manager) JoinGame(clubID uint32, gameNum uint32, player *Player) error  {
	gameID := fmt.Sprintf("%d:%d", clubID, gm.gameCount)
	if _, ok := gm.activeGames[gameID]; !ok {
		// game not found
		return fmt.Errorf("Game %d is not found", gameNum)
	}
	game, _ := gm.activeGames[gameID]

	game.addPlayer(player)
	
	return nil
}

func (gm *Manager) DealHand(clubID uint32, gameNum uint32) error  {
	gameID := fmt.Sprintf("%d:%d", clubID, gm.gameCount)
	if _, ok := gm.activeGames[gameID]; !ok {
		// game not found
		return fmt.Errorf("Game %d is not found", gameNum)
	}
	game, _ := gm.activeGames[gameID]

	var gameMessage GameMessage

	dealHandMessage := &GameDealHandMessage{
	}

	gameMessage.ClubId = clubID
	gameMessage.GameNum = gameNum
	gameMessage.MessageType = GameDealHand
	gameMessage.GameMessage = &GameMessage_DealHand{DealHand: dealHandMessage}
	messageData, _ := proto.Marshal(&gameMessage)

	game.chGame <- messageData
	return nil
}

