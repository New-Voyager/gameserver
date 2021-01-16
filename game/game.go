package game

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
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

var RunningTests bool

type GameMessageReceiver interface {
	BroadcastGameMessage(message *GameMessage)
	BroadcastHandMessage(message *HandMessage)
	SendHandMessageToPlayer(message *HandMessage, playerID uint64)
	SendGameMessageToPlayer(message *GameMessage, playerID uint64)
}

type Game struct {
	manager             *Manager
	end                 chan bool
	running             bool
	chHand              chan []byte
	chGame              chan []byte
	chPlayTimedOut      chan timerMsg
	chResetTimer        chan timerMsg
	chPauseTimer        chan bool
	allPlayers          map[uint64]*Player   // players at the table and the players that are viewing
	messageReceiver     *GameMessageReceiver // receives messages
	actionTimeStart     time.Time
	players             map[uint64]string
	waitingPlayers      []uint64
	remainingActionTime uint32
	apiServerUrl        string
	// test driver specific variables
	autoDeal                bool
	testDeckToUse           *poker.Deck
	testButtonPos           int32
	scriptTest              bool
	inProcessPendingUpdates bool
	config                  *GameConfig
	delays                  Delays
	lock                    sync.Mutex
	timerSeatNo             uint32
}

type timerMsg struct {
	seatNo      uint32
	playerID    uint64
	canCheck    bool
	allowedTime time.Duration
}

func NewPokerGame(gameManager *Manager, messageReceiver *GameMessageReceiver, config *GameConfig, delays Delays, autoDeal bool,
	gameStatePersist PersistGameState,
	handStatePersist PersistHandState,
	apiServerUrl string) (*Game, error) {

	if config.SmallBlind == 0.0 || config.BigBlind == 0.0 {
		channelGameLogger.Error().Msgf("Game cannot be configured with small blind and big blind")
		return nil, fmt.Errorf("Blinds must be set. SmallBlind: %f BigBlind: %f", config.SmallBlind, config.BigBlind)
	}
	g := Game{
		manager:         gameManager,
		messageReceiver: messageReceiver,
		config:          config,
		delays:          delays,
		autoDeal:        autoDeal,
		testButtonPos:   -1,
		apiServerUrl:    apiServerUrl,
	}
	g.allPlayers = make(map[uint64]*Player)
	g.chGame = make(chan []byte)
	g.chHand = make(chan []byte, 1)
	g.chPlayTimedOut = make(chan timerMsg)
	g.chResetTimer = make(chan timerMsg)
	g.chPauseTimer = make(chan bool)
	g.end = make(chan bool)
	g.waitingPlayers = make([]uint64, 0)
	g.players = make(map[uint64]string)
	g.initialize()
	return &g, nil
}

func (g *Game) SetScriptTest(scriptTest bool) {
	g.scriptTest = scriptTest
}

func (g *Game) playersInSeatsCount() int {
	state, err := g.loadState()
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

func (g *Game) timerLoop(stop <-chan bool, pause <-chan bool) {
	var currentTimerMsg timerMsg
	var expirationTime time.Time
	paused := true
	for {
		select {
		case <-stop:
			return
		case <-pause:
			paused = true
		case msg := <-g.chResetTimer:
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
				// track remainingActionTime to show the new observer how much time the current player has to act
				g.remainingActionTime = uint32(remainingTime)

				if remainingTime <= 0 {
					// The player timed out.
					g.chPlayTimedOut <- currentTimerMsg
					expirationTime = time.Time{}
					paused = true
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (g *Game) resetTimer(seatNo uint32, playerID uint64, canCheck bool) {
	channelGameLogger.Info().Msgf("Resetting timer. Current timer seat: %d timer: %d", seatNo, g.config.ActionTime)
	fmt.Printf("Resetting timer. Current timer seat: %d timer: %d\n", seatNo, g.config.ActionTime)
	g.timerSeatNo = seatNo
	g.actionTimeStart = time.Now()
	g.chResetTimer <- timerMsg{
		seatNo:      seatNo,
		playerID:    playerID,
		allowedTime: time.Duration(g.config.ActionTime) * time.Second,
		canCheck:    canCheck,
	}
}

func (g *Game) runGame() {
	stopTimerLoop := make(chan bool)
	defer func() {
		stopTimerLoop <- true
	}()
	go g.timerLoop(stopTimerLoop, g.chPauseTimer)

	ended := false
	for !ended {
		if !g.running {
			started, err := g.startGame()
			if err != nil {
				channelGameLogger.Error().
					Uint32("club", g.config.ClubId).
					Str("game", g.config.GameCode).
					Msg(fmt.Sprintf("Failed to start game: %v", err))
			} else {
				if started {
					g.running = true
				}
			}
		}
		select {
		case <-g.end:
			ended = true
		case message := <-g.chHand:
			var handMessage HandMessage
			err := proto.Unmarshal(message, &handMessage)
			if err == nil {
				g.handleHandMessage(&handMessage)
			}
		case message := <-g.chGame:
			var gameMessage GameMessage
			err := proto.Unmarshal(message, &gameMessage)
			if err == nil {
				g.handleGameMessage(&gameMessage)
			}
		case timeoutMsg := <-g.chPlayTimedOut:
			err := g.handlePlayTimeout(timeoutMsg)
			if err != nil {
				channelGameLogger.Error().Msgf("Error while handling player timeout %+v", err)
			}
		default:
			if !g.running {
				playersInSeats := g.playersInSeatsCount()
				channelGameLogger.Trace().
					Uint32("club", g.config.ClubId).
					Str("game", g.config.GameCode).
					Msg(fmt.Sprintf("Waiting for players to join. %d players in the table, and waiting for %d more players",
						playersInSeats, g.config.MinPlayers-playersInSeats))
				time.Sleep(50 * time.Millisecond)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	g.manager.gameEnded(g)
}

func (g *Game) pausePlayTimer(seatNo uint32) {
	actionResponseTime := time.Now().Sub(g.actionTimeStart)

	fmt.Printf("Pausing timer. Seat responded seat: %d Responded in: %fs \n", seatNo, actionResponseTime.Seconds())
	g.chPauseTimer <- true
}

func (g *Game) handlePlayTimeout(timeoutMsg timerMsg) error {
	gameState, err := g.loadState()
	if err != nil {
		return err
	}
	handState, err := g.loadHandState(gameState)
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
		GameId:      g.config.GameId,
		ClubId:      g.config.ClubId,
		HandNum:     handState.HandNum,
		HandStatus:  handState.CurrentState,
		HandMessage: &HandMessage_PlayerActed{PlayerActed: &handAction},
	}
	g.SendHandMessage(&handMessage)
	return nil
}

func (g *Game) initialize() error {
	playersState := make(map[uint64]*PlayerState)
	playersInSeats := make([]uint64, g.config.MaxPlayers+1) // seat 0: dealer

	var rewardTrackingIds []uint64
	if g.config.RewardTrackingIds != nil && len(g.config.RewardTrackingIds) > 0 {
		rewardTrackingIds = make([]uint64, len(g.config.RewardTrackingIds))
		for i, id := range g.config.RewardTrackingIds {
			rewardTrackingIds[i] = uint64(id)
		}
	}

	// initialize game state
	gameState := GameState{
		ClubId:                g.config.ClubId,
		GameId:                g.config.GameId,
		PlayersInSeats:        playersInSeats,
		PlayersState:          playersState,
		UtgStraddleAllowed:    false,
		ButtonStraddleAllowed: false,
		Status:                GameStatus_CONFIGURED,
		GameType:              g.config.GameType,
		MinPlayers:            uint32(g.config.MinPlayers),
		HandNum:               0,
		ButtonPos:             0,
		SmallBlind:            float32(g.config.SmallBlind),
		BigBlind:              float32(g.config.BigBlind),
		MaxSeats:              uint32(g.config.MaxPlayers),
		TableStatus:           TableStatus_WAITING_TO_BE_STARTED,
		ActionTime:            uint32(g.config.ActionTime),
		RakePercentage:        float32(g.config.RakePercentage),
		RakeCap:               float32(g.config.RakeCap),
		RewardTrackingIds:     rewardTrackingIds,
		BringIn:               float32(g.config.BringIn),
	}
	err := g.saveState(&gameState)
	if err != nil {
		panic("Could not store game state")
		//return err
	}
	return nil
}

func (g *Game) startGame() (bool, error) {
	gameState, err := g.loadState()
	if err != nil {
		return false, err
	}

	if !g.config.AutoStart && gameState.Status != GameStatus_ACTIVE {
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
		gameState.TableStatus = TableStatus_NOT_ENOUGH_PLAYERS
		g.saveState(gameState)

		// TODO:
		// broadcast this message to the players
		// update this message in API server
		if lastTableState != gameState.TableStatus {
			g.broadcastTableState()
		}
		return false, nil
	}
	gameState.TableStatus = TableStatus_GAME_RUNNING

	channelGameLogger.Info().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
		Msg(fmt.Sprintf("Game started. Good luck every one. Players in the table: %d. Waiting list players: %d",
			playersInSeats, len(g.waitingPlayers)))

	// assign the button pos to the first guy in the list
	playersInSeat := gameState.PlayersInSeats
	for seatNo, playerID := range playersInSeat {
		// skip seat no 0
		if seatNo == 0 {
			continue
		}
		if playerID != 0 {
			gameState.ButtonPos = uint32(seatNo)
			break
		}
	}
	gameState.Status = GameStatus_ACTIVE
	err = g.saveState(gameState)
	if err != nil {
		return false, err
	}
	g.running = true

	gameMessage := GameMessage{MessageType: GameCurrentStatus, GameId: g.config.GameId, PlayerId: 0}
	gameMessage.GameMessage = &GameMessage_Status{Status: &GameStatusMessage{Status: gameState.Status, TableStatus: gameState.TableStatus}}
	g.broadcastGameMessage(&gameMessage)

	if g.autoDeal {
		g.dealNewHand()
	}

	return true, nil
}

func (g *Game) maskCards(playerCards []byte, gameToken uint64) ([]uint32, uint64) {
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

func (g *Game) NumCards(gameType GameType) uint32 {
	noCards := 2
	switch gameType {
	case GameType_HOLDEM:
		noCards = 2
	case GameType_PLO:
		noCards = 4
	case GameType_PLO_HILO:
		noCards = 4
	case GameType_FIVE_CARD_PLO:
		noCards = 5
	case GameType_FIVE_CARD_PLO_HILO:
		noCards = 5
	}
	return uint32(noCards)
}

func (g *Game) dealNewHand() error {
	gameState, err := g.loadState()
	if err != nil {
		return err
	}

	// remove the old handstate
	handState1, _ := g.loadHandState(gameState)
	if handState1 != nil {
		g.removeHandState(gameState, handState1)
	}

	moveButton := gameState.HandNum > 1

	if g.testButtonPos != -1 {
		gameState.ButtonPos = uint32(g.testButtonPos)
		moveButton = false
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
	gameType := gameState.GameType

	handState.initialize(gameState, g.testDeckToUse, gameState.ButtonPos, moveButton)

	g.testDeckToUse = nil
	g.testButtonPos = -1
	gameState.ButtonPos = handState.GetButtonPos()

	// save the game and hand
	g.saveState(gameState)
	g.saveHandState(gameState, handState)

	channelGameLogger.Trace().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
		Uint32("hand", handState.HandNum).
		Msg(fmt.Sprintf("Table: %s", handState.PrintTable(g.players)))

	handMessage := HandMessage{
		MessageType: HandNewHand,
		GameId:      g.config.GameId,
		ClubId:      g.config.ClubId,
		HandNum:     handState.HandNum,
		HandStatus:  handState.CurrentState,
	}
	playersCards := make(map[uint32]string)

	// send a new hand message to all players
	newHand := NewHand{
		ButtonPos:      handState.ButtonPos,
		SbPos:          handState.SmallBlindPos,
		BbPos:          handState.BigBlindPos,
		NextActionSeat: handState.NextSeatAction.SeatNo,
		NoCards:        g.NumCards(gameType),
		GameType:       gameType,
		SmallBlind:     handState.SmallBlind,
		BigBlind:       handState.BigBlind,
		BringIn:        handState.BringIn,
		Straddle:       handState.Straddle,
	}
	// we dealt hands and setup for preflop, save handstate
	// if we crash between state: deal and preflop, we will deal the cards again
	g.saveHandState(gameState, handState)

	//newHand.PlayerCards = playersCards
	handMessage.HandMessage = &HandMessage_NewHand{NewHand: &newHand}
	g.broadcastHandMessage(&handMessage)
	if !RunningTests {
		time.Sleep(time.Duration(g.delays.BeforeDeal) * time.Millisecond)
	}

	// indicate the clients card distribution began
	handMessage = HandMessage{
		MessageType: HandDealStarted,
		GameId:      g.config.GameId,
		ClubId:      g.config.ClubId,
		GameCode:    g.config.GameCode,
		HandNum:     handState.HandNum,
		HandStatus:  handState.CurrentState,
	}
	g.broadcastHandMessage(&handMessage)

	activePlayers := uint32(len(gameState.GetPlayersInSeats()))
	cardAnimationTime := time.Duration(activePlayers * g.delays.DealSingleCard * newHand.NoCards)
	// send the cards to each player
	for seatNo, playerID := range gameState.GetPlayersInSeats() {
		if playerID == 0 {
			// empty seat
			continue
		}

		// if the player balance is 0, then don't deal card to him
		if _, ok := handState.PlayersState[playerID]; !ok {
			handState.ActiveSeats[seatNo] = 0
			continue
		}

		// if the player is in break or the player has no balance
		playerState := handState.PlayersState[playerID]
		if playerState.Status == HandPlayerState_SAT_OUT {
			handState.PlayersInSeats[seatNo] = 0
			handState.ActiveSeats[seatNo] = 0
			continue
		}

		// seatNo is the key, cards are value
		playerCards := handState.PlayersCards[uint32(seatNo)]
		message := HandDealCards{SeatNo: uint32(seatNo)}

		cards, maskedCards := g.maskCards(playerCards, gameState.PlayersState[playerID].GameTokenInt)
		playersCards[uint32(seatNo+1)] = fmt.Sprintf("%d", maskedCards)
		message.Cards = fmt.Sprintf("%d", maskedCards)
		message.CardsStr = poker.CardsToString(cards)

		//messageData, _ := proto.Marshal(&message)
		player := g.allPlayers[playerID]
		handMessage := HandMessage{MessageType: HandDeal, GameId: g.config.GameId, ClubId: g.config.ClubId, PlayerId: playerID}
		handMessage.HandMessage = &HandMessage_DealCards{DealCards: &message}
		b, _ := proto.Marshal(&handMessage)

		if *g.messageReceiver != nil {
			(*g.messageReceiver).SendHandMessageToPlayer(&handMessage, playerID)

		} else {
			player.chHand <- b
		}
	}
	if !RunningTests {
		time.Sleep(cardAnimationTime * time.Millisecond)
	}

	// print next action
	channelGameLogger.Trace().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
		Uint32("hand", handState.HandNum).
		Msg(fmt.Sprintf("Next action: %s", handState.NextSeatAction.PrettyPrint(handState, gameState, g.players)))
	g.saveHandState(gameState, handState)

	g.moveToNextAct(gameState, handState)
	return nil
}

func (g *Game) loadState() (*GameState, error) {
	gameState, err := g.manager.gameStatePersist.Load(g.config.ClubId, g.config.GameId)
	if err != nil {
		channelGameLogger.Error().
			Uint32("club", g.config.ClubId).
			Str("game", g.config.GameCode).
			Msg(fmt.Sprintf("Error loading game state.  Error: %v", err))
		return nil, err
	}

	return gameState, err
}

func (g *Game) saveState(gameState *GameState) error {
	err := g.manager.gameStatePersist.Save(g.config.ClubId, g.config.GameId, gameState)
	return err
}

func (g *Game) saveHandState(gameState *GameState, handState *HandState) error {
	err := g.manager.handStatePersist.Save(gameState.GetClubId(),
		gameState.GetGameId(),
		handState.HandNum,
		handState)
	return err
}

func (g *Game) removeHandState(gameState *GameState, handState *HandState) error {
	if gameState == nil || handState == nil {
		return nil
	}

	err := g.manager.handStatePersist.Remove(gameState.GetClubId(),
		gameState.GetGameId(),
		handState.HandNum)
	return err
}

func (g *Game) loadHandState(gameState *GameState) (*HandState, error) {
	handState, err := g.manager.handStatePersist.Load(gameState.GetClubId(),
		gameState.GetGameId(),
		gameState.GetHandNum())
	return handState, err
}

func (g *Game) broadcastHandMessage(message *HandMessage) {
	message.GameCode = g.config.GameCode
	if *g.messageReceiver != nil {
		time.Sleep(time.Duration(g.delays.GlobalBroadcastDelay) * time.Millisecond)
		(*g.messageReceiver).BroadcastHandMessage(message)
		time.Sleep(time.Duration(g.delays.GlobalBroadcastDelay) * time.Millisecond)
	} else {
		b, _ := proto.Marshal(message)
		for _, player := range g.allPlayers {
			player.chHand <- b
		}
	}
}

func (g *Game) broadcastGameMessage(message *GameMessage) {
	message.GameCode = g.config.GameCode
	if *g.messageReceiver != nil {
		time.Sleep(time.Duration(g.delays.GlobalBroadcastDelay) * time.Millisecond)
		(*g.messageReceiver).BroadcastGameMessage(message)
		time.Sleep(time.Duration(g.delays.GlobalBroadcastDelay) * time.Millisecond)
	} else {
		b, _ := proto.Marshal(message)
		for _, player := range g.allPlayers {
			player.chGame <- b
		}
	}
}

func (g *Game) SendGameMessageToChannel(message *GameMessage) {
	b, _ := proto.Marshal(message)
	g.chGame <- b
}

func (g *Game) sendGameMessageToReceiver(message *GameMessage) {
	message.GameCode = g.config.GameCode
	if *g.messageReceiver != nil {
		(*g.messageReceiver).SendGameMessageToPlayer(message, message.PlayerId)
	}
}

func (g *Game) SendHandMessage(message *HandMessage) {
	message.GameCode = g.config.GameCode
	b, _ := proto.Marshal(message)
	g.chHand <- b
}

func (g *Game) sendHandMessageToPlayer(message *HandMessage, playerID uint64) {
	message.GameCode = g.config.GameCode
	if *g.messageReceiver != nil {
		(*g.messageReceiver).SendHandMessageToPlayer(message, playerID)
	} else {
		player := g.allPlayers[playerID]
		if player == nil {
			if message.GetMessageType() == HandMsgAck {
				// Not sure why this causes player to be null, but ignore it for now.
				return
			}
		}
		b, _ := proto.Marshal(message)
		player.chHand <- b
	}
}

func (g *Game) addPlayer(player *Player) error {
	g.lock.Lock()
	defer g.lock.Unlock()
	g.allPlayers[player.PlayerID] = player
	return nil
}

func (g *Game) getPlayersAtTable() ([]*PlayerAtTableState, error) {
	gameState, err := g.loadState()
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

func anyPendingUpdates(apiServerUrl string, gameID uint64, retryDelay uint32) (bool, error) {
	type pendingUpdates struct {
		PendingUpdates bool
	}
	var updates pendingUpdates
	url := fmt.Sprintf("%s/internal/any-pending-updates/gameId/%d", apiServerUrl, gameID)
	retry := true
	for retry {
		resp, err := http.Get(url)
		if resp == nil {
			channelGameLogger.Error().Msgf("Connection to API server is lost. Waiting for %.3f seconds before retrying", float32(retryDelay)/1000)
			time.Sleep(time.Duration(retryDelay) * time.Millisecond)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			channelGameLogger.Fatal().Uint64("game", gameID).Msg(fmt.Sprintf("Failed to get pending status. Error: %d", resp.StatusCode))
			return false, fmt.Errorf("Failed to get pending status")
		}

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return false, err
		}
		err = json.Unmarshal(body, &updates)
		if err != nil {
			return false, err
		}
		retry = false
	}
	return updates.PendingUpdates, nil
}

func (g *Game) GameEnded() error {
	gameState, err := g.loadState()
	if err != nil {
		return err
	}

	if gameState != nil {
		// remove the old handstate
		handState, _ := g.loadHandState(gameState)
		if handState != nil {
			g.removeHandState(gameState, handState)
		}
	}
	return nil
}
