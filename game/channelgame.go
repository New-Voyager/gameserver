package game

import (
	"fmt"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/rs/zerolog/log"
)

/**
NOTE: Seat numbers are indexed from 1-9 like the real poker table.
**/

var channelGameLogger = log.With().Str("logger_name", "game::channelgame").Logger()

type GameMessage struct {
	version      string
	clubID       uint32
	gameNum      uint32
	messageType  string
	playerID     uint32 // 0 to all players
	messageProto []byte
}

const (
	MessageJoin        string = "JOIN"
	MessageGameStarted string = "GAME_STARTED"

	MessageDeal string = "DEAL"

	MESSAGE_ACTION      string = "ACTION"
	MESSAGE_NEXT_ACTION string = "NEXT_ACTION"
	MESSAGE_FLOP        string = "FLOP"
	MESSAGE_TURN        string = "TURN"
	MESSAGE_RIVER       string = "RIVER"
	MESSAGE_SHOWDOWN    string = "SHOWDOWN"
	MESSAGE_WINNER      string = "WINNER"
	MESSAGE_PLAYERSAT   string = "PLAYER_SAT"
)

type ChannelGame struct {
	gameID         string
	clubID         uint32
	gameNum        uint32
	gameManager    *GameManager
	gameType       GameType
	title          string
	end            chan bool
	running        bool
	chGame         chan GameMessage
	chManagement   chan GameMessage
	activePlayers  map[uint32]*ChannelPlayer
	players        map[uint32]string
	minPlayers     int
	waitingPlayers []*ChannelPlayer
	lock           sync.Mutex
}

/**
This function starts a go routine and listens in Game channel for incoming player messages. Only one message is allowed any given time.
This is primarily used for running unit tests to catch regressions.
*/
func NewPokerGame(gameManager *GameManager, gameID string, gameType GameType, clubID uint32, gameNum uint32, minPlayers int, gameStatePersist PersistGameState, handStatePersist PersistHandState) *ChannelGame {
	title := fmt.Sprintf("%d:%d %s", clubID, gameNum, GameType_name[int32(gameType)])
	game := ChannelGame{gameManager: gameManager, gameID: gameID, gameType: gameType, title: title, clubID: clubID, gameNum: gameNum}
	game.activePlayers = make(map[uint32]*ChannelPlayer)
	game.chGame = make(chan GameMessage)
	game.end = make(chan bool)
	game.chManagement = make(chan GameMessage)
	game.waitingPlayers = make([]*ChannelPlayer, 0)
	game.minPlayers = minPlayers
	game.players = make(map[uint32]string)
	return &game
}

func (game *ChannelGame) handleGameMessage(message GameMessage) {
	channelGameLogger.Debug().
		Uint32("club", game.clubID).
		Uint32("game", game.gameNum).
		Msg(fmt.Sprintf("Game message: %s", message.messageType))
}

func (game *ChannelGame) handleManagementMessage(message GameMessage) {
	channelGameLogger.Debug().
		Uint32("club", game.clubID).
		Uint32("game", game.gameNum).
		Msg(fmt.Sprintf("Game message: %s", message.messageType))
	defer game.lock.Unlock()
	game.lock.Lock()
	if !game.running {
		var player *ChannelPlayer
		for len(game.waitingPlayers) > 0 {
			player, game.waitingPlayers = game.waitingPlayers[0], game.waitingPlayers[1:]
			game.activePlayers[player.playerID] = player
			game.players[player.playerID] = player.playerName
			if len(game.activePlayers) == 9 {
				break
			}
		}
	}
}

func runGame(game *ChannelGame) {
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
		case message := <-game.chGame:
			game.handleGameMessage(message)
		case message := <-game.chManagement:
			game.handleManagementMessage(message)
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
	game.gameManager.gameEnded(game)
}

func (game *ChannelGame) startGame() {
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
	game.gameManager.gameStatePersist.Save(game.clubID, game.gameNum, &gameState)
	message := GameStartedMessage{ClubId: game.clubID, GameNum: game.gameNum}
	messageData, _ := proto.Marshal(&message)
	game.chManagement <- GameMessage{messageType: MessageJoin, playerID: 0, messageProto: messageData}
}

func (game *ChannelGame) dealNewHand() {
	gameState, err := game.gameManager.gameStatePersist.Load(game.clubID, game.gameNum)
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

	game.gameManager.handStatePersist.Save(gameState.GetClubId(), gameState.GetGameNum(), handState.HandNum, &handState)

	gameState.ButtonPos = handState.GetButtonPos()
	// save the game
	game.gameManager.gameStatePersist.Save(gameState.GetClubId(), gameState.GetGameNum(), gameState)

	channelGameLogger.Info().
		Uint32("club", game.clubID).
		Uint32("game", game.gameNum).
		Uint32("hand", handState.HandNum).
		Msg(fmt.Sprintf("Table: %s", handState.PrintTable(game.players)))

	// send the cards to each player
	for _, player := range game.activePlayers {
		playerCards := handState.PlayersCards[player.playerID]
		message := HandDealCards{ClubId: game.clubID, GameNum: game.gameNum, HandNum: handState.HandNum, Cards: playerCards}
		messageData, _ := proto.Marshal(&message)
		player.ch <- GameMessage{messageType: MessageDeal, playerID: player.playerID, messageProto: messageData}
	}
	time.Sleep(100 * time.Millisecond)
	// print next action
	channelGameLogger.Info().
		Uint32("club", game.clubID).
		Uint32("game", game.gameNum).
		Uint32("hand", handState.HandNum).
		Msg(fmt.Sprintf("Next action: %s", handState.NextSeatAction.PrettyPrint(&handState, gameState, game.players)))

}

func (game *ChannelGame) broadcastMessage(message GameMessage) {
	for _, player := range game.activePlayers {
		player.ch <- message
	}
}
