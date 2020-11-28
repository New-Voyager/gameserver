package game

import (
	"encoding/binary"
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
	SendHandMessageToPlayer(message *HandMessage, playerID uint64)
	SendGameMessageToPlayer(message *GameMessage, playerID uint64)
}

type Game struct {
	gameCode        string
	clubID          uint32
	gameID          uint64
	manager         *Manager
	gameType        GameType
	title           string
	end             chan bool
	running         bool
	chHand          chan []byte
	chGame          chan []byte
	chPlayTimedOut  chan timerMsg
	chResetTimer    chan timerMsg
	chPauseTimer    chan bool
	allPlayers      map[uint64]*Player   // players at the table and the players that are viewing
	messageReceiver *GameMessageReceiver // receives messages
	players         map[uint64]string
	waitingPlayers  []uint64
	minPlayers      int
	actionTime      uint32
	actionSeat      actionSeat

	// test driver specific variables
	autoStart     bool
	autoDeal      bool
	testDeckToUse *poker.Deck
	testButtonPos int32

	lock sync.Mutex
}

type timerMsg struct {
	seatNo      uint32
	playerID    uint64
	canCheck    bool
	allowedTime time.Duration
}

type actionSeat struct {
	seatNo              uint32
	remainingActionTime uint32
}

func NewPokerGame(gameManager *Manager, messageReceiver *GameMessageReceiver, gameCode string, gameType GameType,
	clubID uint32, gameID uint64, minPlayers int, autoStart bool, autoDeal bool, actionTime uint32,
	gameStatePersist PersistGameState,
	handStatePersist PersistHandState) *Game {
	title := fmt.Sprintf("%d:%d %s", clubID, gameID, GameType_name[int32(gameType)])
	game := Game{
		manager:         gameManager,
		messageReceiver: messageReceiver,
		//gameCode:        gameCode,
		gameType:      gameType,
		title:         title,
		clubID:        clubID,
		gameID:        gameID,
		autoStart:     autoStart,
		autoDeal:      autoDeal,
		testButtonPos: -1,
	}
	game.allPlayers = make(map[uint64]*Player)
	game.chGame = make(chan []byte)
	game.chHand = make(chan []byte, 1)
	game.chPlayTimedOut = make(chan timerMsg)
	game.chResetTimer = make(chan timerMsg)
	game.chPauseTimer = make(chan bool)
	game.end = make(chan bool)
	game.waitingPlayers = make([]uint64, 0)
	game.minPlayers = minPlayers
	game.players = make(map[uint64]string)
	game.actionTime = actionTime
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

func (game *Game) timerLoop(stop <-chan bool, pause <-chan bool) {
	var currentTimerMsg timerMsg
	var expirationTime time.Time
	paused := true
	for {
		select {
		case <-stop:
			return
		case <-pause:
			paused = true
		case msg := <-game.chResetTimer:
			// Start the new timer.
			currentTimerMsg = msg
			expirationTime = time.Now().Add(msg.allowedTime)
			paused = false
		default:
			if !paused {
				remainingTime := expirationTime.Sub(time.Now()).Seconds()
				if remainingTime < 0 {
					remainingTime = 0
				}
				game.actionSeat = actionSeat{
					seatNo:              currentTimerMsg.seatNo,
					remainingActionTime: uint32(remainingTime),
				}

				if remainingTime <= 0 {
					// The player timed out.
					game.chPlayTimedOut <- currentTimerMsg
					expirationTime = time.Time{}
					paused = true
				}
			} else {
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
}

func (game *Game) resetTimer(seatNo uint32, playerID uint64, canCheck bool) {
	channelGameLogger.Info().Msgf("Resetting timer. Current timer seat: %d timer: %d", seatNo, game.actionTime)
	game.chResetTimer <- timerMsg{
		seatNo:      seatNo,
		playerID:    playerID,
		allowedTime: time.Duration(game.actionTime) * time.Second,
		canCheck:    canCheck,
	}
}

func (game *Game) runGame() {
	stopTimerLoop := make(chan bool)
	defer func() {
		stopTimerLoop <- true
	}()
	go game.timerLoop(stopTimerLoop, game.chPauseTimer)

	ended := false
	for !ended {
		if !game.running {
			started, err := game.startGame()
			if err != nil {
				channelGameLogger.Error().
					Uint32("club", game.clubID).
					Uint64("game", game.gameID).
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
		case timeoutMsg := <-game.chPlayTimedOut:
			err := game.handlePlayTimeout(timeoutMsg)
			if err != nil {
				channelGameLogger.Error().Msgf("Error while handling player timeout %+v", err)
			}
		default:
			if !game.running {
				playersInSeats := game.playersInSeatsCount()
				channelGameLogger.Trace().
					Uint32("club", game.clubID).
					Uint64("game", game.gameID).
					Msg(fmt.Sprintf("Waiting for players to join. %d players in the table, and waiting for %d more players",
						playersInSeats, game.minPlayers-playersInSeats))
				time.Sleep(50 * time.Millisecond)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	game.manager.gameEnded(game)
}

func (game *Game) pausePlayTimer() {
	game.chPauseTimer <- true
}

func (game *Game) handlePlayTimeout(timeoutMsg timerMsg) error {
	gameState, err := game.loadState()
	if err != nil {
		return err
	}
	handState, err := game.loadHandState(gameState)
	if err != nil {
		return err
	}

	// Force a default action for the timed-out player.
	// TODO: What should be the correct default action?
	handAction := HandAction{
		SeatNo:   timeoutMsg.seatNo,
		Action:   ACTION_FOLD,
		Amount:   0.0,
		TimedOut: true,
	}
	if timeoutMsg.canCheck {
		handAction.Action = ACTION_CHECK
	}

	handMessage := HandMessage{
		MessageType: HandPlayerActed,
		GameId:      game.gameID,
		ClubId:      game.clubID,
		HandNum:     handState.HandNum,
		HandStatus:  handState.CurrentState,
		HandMessage: &HandMessage_PlayerActed{PlayerActed: &handAction},
	}
	game.SendHandMessage(&handMessage)
	return nil
}

func (game *Game) initialize() error {
	playersState := make(map[uint64]*PlayerState)
	playersInSeats := make([]uint64, 9)

	// initialize game state
	gameState := GameState{
		ClubId:                game.clubID,
		GameId:                game.gameID,
		PlayersInSeats:        playersInSeats,
		PlayersState:          playersState,
		UtgStraddleAllowed:    false,
		ButtonStraddleAllowed: false,
		Status:                GameStatus_CONFIGURED,
		GameType:              game.gameType,
		MinPlayers:            uint32(game.minPlayers),
		HandNum:               0,
		ButtonPos:             0,
		SmallBlind:            1.0,
		BigBlind:              2.0,
		MaxSeats:              9,
		TableStatus:           TableStatus_TABLE_STATUS_WAITING_TO_BE_STARTED,
		ActionTime:            game.actionTime,
	}
	err := game.saveState(&gameState)
	if err != nil {
		panic("Could not store game state")
		//return err
	}
	return nil
}

func (game *Game) startGame() (bool, error) {
	gameState, err := game.loadState()
	if err != nil {
		return false, err
	}

	if !game.autoStart && gameState.Status != GameStatus_ACTIVE {
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
		lastTableState := gameState.TableStatus
		// not enough players
		// set table status as not enough players
		gameState.TableStatus = TableStatus_TABLE_STATUS_NOT_ENOUGH_PLAYERS
		game.saveState(gameState)

		// TODO:
		// broadcast this message to the players
		// update this message in API server
		if lastTableState != gameState.TableStatus {
			game.broadcastTableState()
		}
		return false, nil
	}
	gameState.TableStatus = TableStatus_TABLE_STATUS_GAME_RUNNING

	channelGameLogger.Info().
		Uint32("club", game.clubID).
		Uint64("game", game.gameID).
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
	gameState.Status = GameStatus_ACTIVE
	err = game.saveState(gameState)
	if err != nil {
		return false, err
	}
	game.running = true

	gameMessage := GameMessage{MessageType: GameCurrentStatus, GameId: game.gameID, PlayerId: 0}
	gameMessage.GameMessage = &GameMessage_Status{Status: &GameStatusMessage{Status: gameState.Status, TableStatus: gameState.TableStatus}}
	game.broadcastGameMessage(&gameMessage)

	if game.autoDeal {
		game.dealNewHand()
	}

	return true, nil
}

func (game *Game) maskCards(playerCards []byte, gameToken uint64) ([]uint32, uint64) {
	// playerCards is a map
	card64 := make([]byte, 8)
	cards := make([]uint32, len(playerCards))
	for i, card := range playerCards {
		cards[i] = uint32(card)
		card64[i] = card
	}

	// convert cards to uint64
	cardsUint64 := binary.LittleEndian.Uint64(card64)
	mask := gameToken

	// TODO: mask it.
	mask = 0
	maskCards := uint64(cardsUint64)
	if mask != 0 {
		maskCards = uint64(cardsUint64 ^ mask)
	}
	maskedCards := uint64(maskCards) & uint64(0x000000FFFFFFFFFF)
	return cards, maskedCards
}

func (game *Game) dealNewHand() error {
	gameState, err := game.loadState()
	if err != nil {
		return err
	}

	gameState.HandNum++
	handState := &HandState{
		ClubId:        gameState.GetClubId(),
		GameId:        gameState.GetGameId(),
		HandNum:       gameState.GetHandNum(),
		GameType:      gameState.GetGameType(),
		CurrentState:  HandStatus_DEAL,
		HandStartedAt: uint64(time.Now().Unix()),
	}

	handState.initialize(gameState, game.testDeckToUse, game.testButtonPos)
	gameState.ButtonPos = handState.GetButtonPos()

	// save the game and hand
	game.saveState(gameState)
	game.saveHandState(gameState, handState)

	channelGameLogger.Trace().
		Uint32("club", game.clubID).
		Uint64("game", game.gameID).
		Uint32("hand", handState.HandNum).
		Msg(fmt.Sprintf("Table: %s", handState.PrintTable(game.players)))

	handMessage := HandMessage{
		MessageType: HandNewHand,
		GameId:      game.gameID,
		ClubId:      game.clubID,
		HandNum:     handState.HandNum,
		HandStatus:  handState.CurrentState,
	}
	playersCards := make(map[uint32]string)

	// send the cards to each player
	for seatNo, playerID := range gameState.GetPlayersInSeats() {
		if playerID == 0 {
			// empty seat
			continue
		}

		// if the player is in break
		playerState := handState.PlayersState[playerID]
		if playerState.Status == HandPlayerState_SAT_OUT {
			continue
		}

		// seatNo is the key, cards are value
		playerCards := handState.PlayersCards[uint32(seatNo+1)]
		message := HandDealCards{SeatNo: uint32(seatNo + 1)}

		cards, maskedCards := game.maskCards(playerCards, gameState.PlayersState[playerID].GameTokenInt)
		playersCards[uint32(seatNo+1)] = fmt.Sprintf("%d", maskedCards)
		message.Cards = cards
		message.CardsStr = poker.CardsToString(cards)

		/*
			//messageData, _ := proto.Marshal(&message)
			player := game.allPlayers[playerID]
			handMessage := HandMessage{MessageType: HandDeal, GameId: game.gameID, ClubId: game.clubID, PlayerId: playerID}
			handMessage.HandMessage = &HandMessage_DealCards{DealCards: &message}
			b, _ := proto.Marshal(&handMessage)

			if *game.messageReceiver != nil {
				(*game.messageReceiver).SendHandMessageToPlayer(&handMessage, playerID)

			} else {
				player.chHand <- b
			}
		*/
	}

	// send a new hand message to all players
	newHand := NewHand{
		ButtonPos:      handState.ButtonPos,
		SbPos:          handState.SmallBlindPos,
		BbPos:          handState.BigBlindPos,
		NextActionSeat: handState.NextSeatAction.SeatNo,
		PlayerCards:    playersCards,
	}
	// we dealt hands and setup for preflop, save handstate
	// if we crash between state: deal and preflop, we will deal the cards again
	game.saveHandState(gameState, handState)

	//newHand.PlayerCards = playersCards
	handMessage.HandMessage = &HandMessage_NewHand{NewHand: &newHand}
	game.broadcastHandMessage(&handMessage)

	// print next action
	channelGameLogger.Trace().
		Uint32("club", game.clubID).
		Uint64("game", game.gameID).
		Uint32("hand", handState.HandNum).
		Msg(fmt.Sprintf("Next action: %s", handState.NextSeatAction.PrettyPrint(handState, gameState, game.players)))

	game.moveToNextAct(gameState, handState)
	return nil
}

func (game *Game) loadState() (*GameState, error) {
	gameState, err := game.manager.gameStatePersist.Load(game.clubID, game.gameID)
	if err != nil {
		channelGameLogger.Error().
			Uint32("club", game.clubID).
			Uint64("game", game.gameID).
			Msg(fmt.Sprintf("Error loading game state.  Error: %v", err))
		return nil, err
	}

	return gameState, err
}

func (game *Game) saveState(gameState *GameState) error {
	err := game.manager.gameStatePersist.Save(game.clubID, game.gameID, gameState)
	return err
}

func (game *Game) saveHandState(gameState *GameState, handState *HandState) error {
	err := game.manager.handStatePersist.Save(gameState.GetClubId(),
		gameState.GetGameId(),
		handState.HandNum,
		handState)
	return err
}

func (game *Game) loadHandState(gameState *GameState) (*HandState, error) {
	handState, err := game.manager.handStatePersist.Load(gameState.GetClubId(),
		gameState.GetGameId(),
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

func (game *Game) SendHandMessage(message *HandMessage) {
	b, _ := proto.Marshal(message)
	game.chHand <- b
}

func (game *Game) sendHandMessageToPlayer(message *HandMessage, playerID uint64) {
	if *game.messageReceiver != nil {
		(*game.messageReceiver).SendHandMessageToPlayer(message, playerID)
	} else {
		player := game.allPlayers[playerID]
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
