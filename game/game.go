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

type MessageDirection string

const (
	PlayerToPlayer MessageDirection = "P_2_P"
	PlayerToGame                    = "P_2_G"
	GameToPlayer                    = "G_2_P"
	GameToAll                       = "G_2_A"
)

type GameMessage struct {
	version      string
	clubID       uint32
	gameNum      uint32
	messageType  string
	playerID     uint32 // 0: send the message to all players
	messageProto []byte
	player       *Player
}

type HandMessage struct {
	version      string
	clubID       uint32
	gameNum      uint32
	messageType  string
	playerID     uint32 // 0: send the message to all players
	messageProto []byte
}

// Game messages
const (
	GameJoin       string = "JOIN"
	GameStarted    string = "GAME_STARTED"
	PlayerTookSeat string = "PLAYER_SAT"
)

// Hand messages
const (
	HandDeal       string = "DEAL"
	HandActed      string = "ACTED"
	HandNextAction string = "NEXT_ACTION"
	HandFlop       string = "FLOP"
	HandTurn       string = "TURN"
	HandRiver      string = "RIVER"
	HandShowDown   string = "SHOWDOWN"
	HandWinner     string = "WINNER"
	HandEnded      string = "END"
)

type Game struct {
	gameID         string
	clubID         uint32
	gameNum        uint32
	manager        *Manager
	gameType       GameType
	title          string
	end            chan bool
	running        bool
	chHand         chan HandMessage
	chGame         chan GameMessage
	activePlayers  map[uint32]*Player
	players        map[uint32]string
	minPlayers     int
	waitingPlayers []*Player
	lock           sync.Mutex
}

func NewPokerGame(gameManager *Manager, gameID string, gameType GameType,
	clubID uint32, gameNum uint32, minPlayers int, gameStatePersist PersistGameState,
	handStatePersist PersistHandState) *Game {
	title := fmt.Sprintf("%d:%d %s", clubID, gameNum, GameType_name[int32(gameType)])
	game := Game{manager: gameManager, gameID: gameID, gameType: gameType, title: title, clubID: clubID, gameNum: gameNum}
	game.activePlayers = make(map[uint32]*Player)
	game.chGame = make(chan GameMessage)
	game.chHand = make(chan HandMessage)
	game.end = make(chan bool)
	game.waitingPlayers = make([]*Player, 0)
	game.minPlayers = minPlayers
	game.players = make(map[uint32]string)
	return &game
}

func (game *Game) handleHandMessage(message HandMessage) {
	channelGameLogger.Debug().
		Uint32("club", game.clubID).
		Uint32("game", game.gameNum).
		Msg(fmt.Sprintf("Hand message: %s", message.messageType))
}

func (game *Game) handleGameMessage(message GameMessage) {
	channelGameLogger.Info().
		Uint32("club", game.clubID).
		Uint32("game", game.gameNum).
		Msg(fmt.Sprintf("Game message: %s. %v", message.messageType, message))

	defer game.lock.Unlock()
	game.lock.Lock()

	switch message.messageType {
	case PlayerTookSeat:
		game.activePlayers[message.playerID] = message.player
		game.players[message.playerID] = message.player.playerName
		if len(game.activePlayers) == 9 {
			break
		}
	}

	channelGameLogger.Info().
		Uint32("club", game.clubID).
		Uint32("game", game.gameNum).
		Msg(fmt.Sprintf("Game message: %s. RETURN", message.messageType))

}

func runGame(game *Game) {
	ended := false
	for !ended {
		if !game.running && len(game.activePlayers) >= game.minPlayers {
			game.lock.Lock()
			// start the game
			game.startGame()
			game.running = true
			game.dealNewHand()
			game.lock.Unlock()
		}
		select {
		case <-game.end:
			ended = true
		case message := <-game.chHand:
			game.handleHandMessage(message)
		case message := <-game.chGame:
			game.handleGameMessage(message)
		default:
			if !game.running {
				channelGameLogger.Info().
					Uint32("club", game.clubID).
					Uint32("game", game.gameNum).
					Msg(fmt.Sprintf("Waiting for players to join. %d players in the table, and waiting for %d more players",
						len(game.activePlayers), game.minPlayers-len(game.activePlayers)))
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
	game.manager.gameEnded(game)
}

func (game *Game) startGame() {
	channelGameLogger.Info().
		Uint32("club", game.clubID).
		Uint32("game", game.gameNum).
		Msg(fmt.Sprintf("Game started. Good luck every one. Players in the table: %d. Waiting list players: %d",
			len(game.activePlayers), len(game.waitingPlayers)))

	playersState := make(map[uint32]*PlayerState)
	playersInSeats := make([]uint32, 9)
	i := 0
	for _, player := range game.activePlayers {
		playersInSeats[i] = player.playerID
		playersState[player.playerID] = &PlayerState{BuyIn: 100, CurrentBalance: 100, Status: PlayerState_PLAYING}
		i++
	}

	// initialize game state
	gameState := GameState{
		ClubId:                game.clubID,
		GameNum:               game.gameNum,
		PlayersInSeats:        playersInSeats,
		PlayersState:          playersState,
		UtgStraddleAllowed:    false,
		ButtonStraddleAllowed: false,
		Status:                GameState_RUNNING,
		GameType:              GameType_HOLDEM,
		HandNum:               0,
		ButtonPos:             1,
		SmallBlind:            1.0,
		BigBlind:              2.0,
		MaxSeats:              9,
	}
	game.manager.gameStatePersist.Save(game.clubID, game.gameNum, &gameState)
	message := GameStartedMessage{ClubId: game.clubID, GameNum: game.gameNum}
	messageData, _ := proto.Marshal(&message)
	game.broadcastGameMessage(GameMessage{messageType: GameStarted, playerID: 0, messageProto: messageData})
}

func (game *Game) dealNewHand() {
	gameState, err := game.manager.gameStatePersist.Load(game.clubID, game.gameNum)
	if err != nil {
		channelGameLogger.Error().
			Uint32("club", game.clubID).
			Uint32("game", game.gameNum).
			Msg(fmt.Sprintf("Game %d is not found", game.gameNum))
	}

	gameState.HandNum++
	handState := HandState{
		ClubId:   gameState.GetClubId(),
		GameNum:  gameState.GetGameNum(),
		HandNum:  gameState.GetHandNum(),
		GameType: gameState.GetGameType(),
	}

	handState.initialize(gameState)

	game.manager.handStatePersist.Save(gameState.GetClubId(), gameState.GetGameNum(), handState.HandNum, &handState)

	gameState.ButtonPos = handState.GetButtonPos()
	// save the game
	game.manager.gameStatePersist.Save(gameState.GetClubId(), gameState.GetGameNum(), gameState)

	channelGameLogger.Info().
		Uint32("club", game.clubID).
		Uint32("game", game.gameNum).
		Uint32("hand", handState.HandNum).
		Msg(fmt.Sprintf("Table: %s", handState.PrintTable(game.players)))

	// send the cards to each player
	for _, player := range game.activePlayers {
		playerCards := handState.PlayersCards[player.playerID]
		message := HandDealCards{ClubId: game.clubID, GameNum: game.gameNum, HandNum: handState.HandNum}
		message.Cards = make([]uint32, len(playerCards))
		for i, card := range playerCards {
			message.Cards[i] = uint32(card)
		}
		message.CardsStr = poker.CardsToString(message.Cards)

		messageData, _ := proto.Marshal(&message)
		player.chHand <- HandMessage{messageType: HandDeal, playerID: player.playerID, messageProto: messageData}
	}
	time.Sleep(100 * time.Millisecond)
	// print next action
	channelGameLogger.Info().
		Uint32("club", game.clubID).
		Uint32("game", game.gameNum).
		Uint32("hand", handState.HandNum).
		Msg(fmt.Sprintf("Next action: %s", handState.NextSeatAction.PrettyPrint(&handState, gameState, game.players)))
}

func (game *Game) broadcastMessage(message HandMessage) {
	for _, player := range game.activePlayers {
		player.chHand <- message
	}
}

func (game *Game) broadcastGameMessage(message GameMessage) {
	for _, player := range game.activePlayers {
		player.chGame <- message
	}
}
