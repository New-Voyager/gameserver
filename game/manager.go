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

func (gm *Manager) JoinGame(clubID uint32, gameNum uint32, player *Player) error {
	gameID := fmt.Sprintf("%d:%d", clubID, gm.gameCount)
	if _, ok := gm.activeGames[gameID]; !ok {
		// game not found
		return fmt.Errorf("Game %d is not found", gameNum)
	}
	game, _ := gm.activeGames[gameID]

	game.addPlayer(player)

	// start listenting for game/hand events
	go player.playGame()

	return nil
}

func (gm *Manager) DealHand(clubID uint32, gameNum uint32) error {
	gameID := fmt.Sprintf("%d:%d", clubID, gm.gameCount)
	if _, ok := gm.activeGames[gameID]; !ok {
		// game not found
		return fmt.Errorf("Game %d is not found", gameNum)
	}
	game, _ := gm.activeGames[gameID]

	var gameMessage GameMessage

	dealHandMessage := &GameDealHandMessage{}

	gameMessage.ClubId = clubID
	gameMessage.GameNum = gameNum
	gameMessage.MessageType = GameDealHand
	gameMessage.GameMessage = &GameMessage_DealHand{DealHand: dealHandMessage}
	messageData, _ := proto.Marshal(&gameMessage)

	game.chGame <- messageData
	return nil
}

// GetTableState returns the table returned to a specific player requested the state
func (gm *Manager) GetTableState(clubID uint32, gameNum uint32, playerID uint32) error {
	gameID := fmt.Sprintf("%d:%d", clubID, gameNum)
	if _, ok := gm.activeGames[gameID]; !ok {
		// game not found
		return fmt.Errorf("Game %d is not found", gameNum)
	}
	game, _ := gm.activeGames[gameID]
	// get active players on the table
	playersAtTable, err := game.getPlayersAtTable()
	if err != nil {
		return err
	}
	gameTableState := &GameTableStateMessage{PlayersState: playersAtTable}
	var gameMessage GameMessage
	gameMessage.ClubId = clubID
	gameMessage.GameNum = gameNum
	gameMessage.MessageType = GameTableState
	gameMessage.GameMessage = &GameMessage_TableState{TableState: gameTableState}
	messageData, _ := proto.Marshal(&gameMessage)

	game.allPlayers[playerID].chGame <- messageData
	return nil
}

// SetupNextHand method can be called only from the test driver
// and this is available only in test mode.
// We will never allow hands to be set any scripts. It is morally wrong.
func (gm *Manager) SetupNextHand(clubID uint32, gameNum uint32, deck []byte, buttonPos uint32) error {
	gameID := fmt.Sprintf("%d:%d", clubID, gm.gameCount)
	if _, ok := gm.activeGames[gameID]; !ok {
		// game not found
		return fmt.Errorf("Game %d is not found", gameNum)
	}
	game, _ := gm.activeGames[gameID]

	var gameMessage GameMessage

	nextHand := &GameSetupNextHandMessage{
		Deck:      deck,
		ButtonPos: buttonPos,
	}

	gameMessage.ClubId = clubID
	gameMessage.GameNum = gameNum
	gameMessage.MessageType = GameSetupNextHand
	gameMessage.GameMessage = &GameMessage_NextHand{NextHand: nextHand}
	messageData, _ := proto.Marshal(&gameMessage)

	game.chGame <- messageData
	return nil
}
