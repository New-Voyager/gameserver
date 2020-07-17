package game

import (
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/proto"
	"voyager.com/server/poker"
)

/**
NOTE: Seat numbers are indexed from 1-9 like the real poker table.
**/

var channelGameLogger = log.With().Str("logger_name", "game::game").Logger()

type GameMessageReceiver interface {
	BroadcastGameMessage(message *GameMessage)
	BroadcastHandMessage(message *HandMessage)
	SendHandMessageToPlayer(message *HandMessage, playerID uint32)
	SendGameMessageToPlayer(message *GameMessage, playerID uint32)
}

type Game struct {
	gameID          string
	clubID          uint32
	gameNum         uint32
	manager         *Manager
	gameType        GameType
	title           string
	end             chan bool
	running         bool
	chHand          chan []byte
	chGame          chan []byte
	allPlayers      map[uint32]*Player   // players at the table and the players that are viewing
	messageReceiver *GameMessageReceiver // receives messages
	players         map[uint32]string
	waitingPlayers  []uint32
	minPlayers      int

	// test driver specific variables
	autoStart     bool
	autoDeal      bool
	testDeckToUse *poker.Deck
	testButtonPos int32

	lock sync.Mutex
}

func NewPokerGame(gameManager *Manager, messageReceiver *GameMessageReceiver, gameID string, gameType GameType,
	clubID uint32, gameNum uint32, minPlayers int, autoStart bool, autoDeal bool,
	gameStatePersist PersistGameState,
	handStatePersist PersistHandState) *Game {
	title := fmt.Sprintf("%d:%d %s", clubID, gameNum, GameType_name[int32(gameType)])
	game := Game{
		manager:         gameManager,
		messageReceiver: messageReceiver,
		gameID:          gameID,
		gameType:        gameType,
		title:           title,
		clubID:          clubID,
		gameNum:         gameNum,
		autoStart:       autoStart,
		autoDeal:        autoDeal,
		testButtonPos:   -1,
	}
	game.allPlayers = make(map[uint32]*Player)
	game.chGame = make(chan []byte)
	game.chHand = make(chan []byte)
	game.end = make(chan bool)
	game.waitingPlayers = make([]uint32, 0)
	game.minPlayers = minPlayers
	game.players = make(map[uint32]string)
	game.initialize()
	return &game
}

func (game *Game) playersInSeatsCount() int {
	state, err := game.loadState()
	if err != nil {
		// panic
		// TODO: FIX THIS CODE
		panic("Shouldn't be here")
	}
	playersInSeats := state.GetPlayersInSeats()
	countPlayersInSeats := 0
	for _, playerID := range playersInSeats {
		if playerID != 0 {
			countPlayersInSeats++
		}
	}
	return countPlayersInSeats
}

func (game *Game) runGame() {
	ended := false
	for !ended {
		if !game.running {
			started, err := game.startGame()
			if err != nil {
				channelGameLogger.Error().
					Uint32("club", game.clubID).
					Uint32("game", game.gameNum).
					Msg(fmt.Sprintf("Failed to start game: %v", err))
			} else {
				if started {
					game.running = true
				}
			}
		}
		select {
		case <-game.end:
			ended = true
		case message := <-game.chHand:
			var handMessage HandMessage
			err := proto.Unmarshal(message, &handMessage)
			if err == nil {
				game.handleHandMessage(&handMessage)
			}
		case message := <-game.chGame:
			var gameMessage GameMessage
			err := proto.Unmarshal(message, &gameMessage)
			if err == nil {
				game.handleGameMessage(&gameMessage)
			}
		default:
			if !game.running {
				playersInSeats := game.playersInSeatsCount()
				channelGameLogger.Trace().
					Uint32("club", game.clubID).
					Uint32("game", game.gameNum).
					Msg(fmt.Sprintf("Waiting for players to join. %d players in the table, and waiting for %d more players",
						playersInSeats, game.minPlayers-playersInSeats))
				time.Sleep(50 * time.Millisecond)
			}
		}
	}
	game.manager.gameEnded(game)
}

func (game *Game) initialize() error {
	playersState := make(map[uint32]*PlayerState)
	playersInSeats := make([]uint32, 9)

	// initialize game state
	gameState := GameState{
		ClubId:                game.clubID,
		GameNum:               game.gameNum,
		PlayersInSeats:        playersInSeats,
		PlayersState:          playersState,
		UtgStraddleAllowed:    false,
		ButtonStraddleAllowed: false,
		Status:                GameStatus_INITIALIZED,
		GameType:              game.gameType,
		MinPlayers:            uint32(game.minPlayers),
		HandNum:               0,
		ButtonPos:             0,
		SmallBlind:            1.0,
		BigBlind:              2.0,
		MaxSeats:              9,
	}
	err := game.saveState(&gameState)
	if err != nil {
		return err
	}
	return nil
}

func (game *Game) startGame() (bool, error) {
	gameState, err := game.loadState()
	if err != nil {
		return false, err
	}

	if !game.autoStart && gameState.Status != GameStatus_START_GAME_RECEIVED {
		return false, nil
	}

	playersInSeats := gameState.GetPlayersInSeats()
	countPlayersInSeats := 0
	for _, playerID := range playersInSeats {
		if playerID != 0 {
			countPlayersInSeats++
		}
	}
	if uint32(countPlayersInSeats) < gameState.GetMinPlayers() {
		// not enough players
		return false, nil
	}

	channelGameLogger.Info().
		Uint32("club", game.clubID).
		Uint32("game", game.gameNum).
		Msg(fmt.Sprintf("Game started. Good luck every one. Players in the table: %d. Waiting list players: %d",
			playersInSeats, len(game.waitingPlayers)))

	// assign the button pos to the first guy in the list
	playersInSeat := gameState.PlayersInSeats
	for seatNoIdx, playerID := range playersInSeat {
		if playerID != 0 {
			gameState.ButtonPos = uint32(seatNoIdx + 1)
			break
		}
	}
	gameState.Status = GameStatus_RUNNING
	err = game.saveState(gameState)
	if err != nil {
		return false, err
	}
	game.running = true

	gameMessage := GameMessage{MessageType: GameCurrentStatus, PlayerId: 0}
	gameMessage.GameMessage = &GameMessage_Status{Status: &GameStatusMessage{Status: gameState.Status}}
	game.broadcastGameMessage(&gameMessage)

	if game.autoDeal {
		game.dealNewHand()
	}

	return true, nil
}

func (game *Game) dealNewHand() error {
	gameState, err := game.loadState()
	if err != nil {
		return err
	}

	gameState.HandNum++
	handState := &HandState{
		ClubId:       gameState.GetClubId(),
		GameNum:      gameState.GetGameNum(),
		HandNum:      gameState.GetHandNum(),
		GameType:     gameState.GetGameType(),
		CurrentState: HandStatus_DEAL,
	}

	handState.initialize(gameState, game.testDeckToUse, game.testButtonPos)
	gameState.ButtonPos = handState.GetButtonPos()

	// save the game and hand
	game.saveState(gameState)
	game.saveHandState(gameState, handState)

	channelGameLogger.Trace().
		Uint32("club", game.clubID).
		Uint32("game", game.gameNum).
		Uint32("hand", handState.HandNum).
		Msg(fmt.Sprintf("Table: %s", handState.PrintTable(game.players)))

	// send a new hand message to all players
	newHand := NewHand{
		ButtonPos:      handState.ButtonPos,
		SbPos:          handState.SmallBlindPos,
		BbPos:          handState.BigBlindPos,
		NextActionSeat: handState.NextSeatAction.SeatNo,
	}
	handMessage := HandMessage{
		MessageType: HandNewHand,
		GameNum:     game.gameNum,
		ClubId:      game.clubID,
		HandNum:     handState.HandNum,
		HandStatus:  handState.CurrentState,
	}
	handMessage.HandMessage = &HandMessage_NewHand{NewHand: &newHand}
	game.broadcastHandMessage(&handMessage)

	// send the cards to each player
	for seatNo, playerID := range gameState.GetPlayersInSeats() {
		if playerID == 0 {
			// empty seat
			continue
		}

		// playerCards is a map
		// seatNo is the key, cards are value
		playerCards := handState.PlayersCards[uint32(seatNo+1)]
		message := HandDealCards{SeatNo: uint32(seatNo + 1)}
		message.Cards = make([]uint32, len(playerCards))
		for i, card := range playerCards {
			message.Cards[i] = uint32(card)
		}
		message.CardsStr = poker.CardsToString(message.Cards)

		//messageData, _ := proto.Marshal(&message)
		player := game.allPlayers[playerID]
		handMessage := HandMessage{MessageType: HandDeal, GameNum: game.gameNum, ClubId: game.clubID}
		handMessage.HandMessage = &HandMessage_DealCards{DealCards: &message}
		b, _ := proto.Marshal(&handMessage)
		player.chHand <- b
	}

	// print next action
	channelGameLogger.Trace().
		Uint32("club", game.clubID).
		Uint32("game", game.gameNum).
		Uint32("hand", handState.HandNum).
		Msg(fmt.Sprintf("Next action: %s", handState.NextSeatAction.PrettyPrint(handState, gameState, game.players)))

	// we dealt hands and setup for preflop, save handstate
	// if we crash between state: deal and preflop, we will deal the cards again
	game.saveHandState(gameState, handState)

	game.moveToNextAct(gameState, handState)
	return nil
}

func (game *Game) loadState() (*GameState, error) {
	gameState, err := game.manager.gameStatePersist.Load(game.clubID, game.gameNum)
	if err != nil {
		channelGameLogger.Error().
			Uint32("club", game.clubID).
			Uint32("game", game.gameNum).
			Msg(fmt.Sprintf("Error loading game state.  Error: %v", err))
		return nil, err
	}

	return gameState, err
}

func (game *Game) saveState(gameState *GameState) error {
	err := game.manager.gameStatePersist.Save(game.clubID, game.gameNum, gameState)
	return err
}

func (game *Game) saveHandState(gameState *GameState, handState *HandState) error {
	err := game.manager.handStatePersist.Save(gameState.GetClubId(),
		gameState.GetGameNum(),
		handState.HandNum,
		handState)
	return err
}

func (game *Game) loadHandState(gameState *GameState) (*HandState, error) {
	handState, err := game.manager.handStatePersist.Load(gameState.GetClubId(),
		gameState.GetGameNum(),
		gameState.GetHandNum())
	return handState, err
}

func (game *Game) broadcastHandMessage(message *HandMessage) {
	if *game.messageReceiver != nil {
		(*game.messageReceiver).BroadcastHandMessage(message)
	} else {
		b, _ := proto.Marshal(message)
		for _, player := range game.allPlayers {
			player.chHand <- b
		}
	}
}

func (game *Game) broadcastGameMessage(message *GameMessage) {
	if *game.messageReceiver != nil {
		(*game.messageReceiver).BroadcastGameMessage(message)
	} else {
		b, _ := proto.Marshal(message)
		for _, player := range game.allPlayers {
			player.chGame <- b
		}
	}
}

func (game *Game) SendGameMessage(message *GameMessage) {
	b, _ := proto.Marshal(message)
	game.chGame <- b
}

func (game *Game) sendHandMessageToPlayer(message *HandMessage, player *Player) {
	if *game.messageReceiver != nil {
		(*game.messageReceiver).SendHandMessageToPlayer(message, player.PlayerID)
	} else {
		b, _ := proto.Marshal(message)
		player.chHand <- b
	}
}

func (game *Game) addPlayer(player *Player) error {
	game.lock.Lock()
	defer game.lock.Unlock()
	game.allPlayers[player.PlayerID] = player
	return nil
}

func (game *Game) getPlayersAtTable() ([]*PlayerAtTableState, error) {
	gameState, err := game.loadState()
	if err != nil {
		return nil, err
	}
	ret := make([]*PlayerAtTableState, 0)
	playersInSeats := gameState.GetPlayersInSeats()
	for seatNo, playerID := range playersInSeats {
		if playerID != 0 {
			playerState := gameState.PlayersState[playerID]
			playerAtTable := &PlayerAtTableState{
				PlayerId:       playerID,
				SeatNo:         uint32(seatNo + 1),
				BuyIn:          playerState.BuyIn,
				CurrentBalance: playerState.CurrentBalance,
				Status:         playerState.Status,
			}
			ret = append(ret, playerAtTable)
		}
	}

	return ret, nil
}
