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
	pauseBeforeNextHand     uint32
	scriptTest              bool
	inProcessPendingUpdates bool
	config                  *GameConfig
	delays                  Delays
	lock                    sync.Mutex
	timerSeatNo             uint32

	state *GameState
}

type timerMsg struct {
	seatNo      uint32
	playerID    uint64
	canCheck    bool
	allowedTime time.Duration
}

func NewPokerGame(gameManager *Manager, messageReceiver *GameMessageReceiver, config *GameConfig, delays Delays, autoDeal bool, handStatePersist PersistHandState,
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
	if RunningTests {
		g.startNewGameState()
	}
	return &g, nil
}

func (g *Game) SetScriptTest(scriptTest bool) {
	g.scriptTest = scriptTest
}

func (g *Game) playersInSeatsCount() int {
	playersInSeats := g.state.GetPlayersInSeats()
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
	handState, err := g.loadHandState()
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
	// This is where we used to check if there is an existing game state in redis and reuse that instead of
	// starting a new game state.
	// Now that we don't save game state to redis, how do we resume the state of a crashed game?
	g.startNewGameState()
	return nil
}

func (g *Game) startNewGameState() error {
	playersInSeats := make([]uint64, g.config.MaxPlayers+1) // seat 0: dealer

	var rewardTrackingIds []uint64
	if g.config.RewardTrackingIds != nil && len(g.config.RewardTrackingIds) > 0 {
		rewardTrackingIds = make([]uint64, len(g.config.RewardTrackingIds))
		for i, id := range g.config.RewardTrackingIds {
			rewardTrackingIds[i] = uint64(id)
		}
	}

	// initialize game state
	g.state = &GameState{
		PlayersInSeats: playersInSeats,
		Status:         g.config.Status,
		ButtonPos:      0,
		TableStatus:    g.config.TableStatus,
	}
	fmt.Printf("g.state: %+v\n", g.state)
	return nil
}

func (g *Game) startGame() (bool, error) {
	if g.state == nil {
		return false, fmt.Errorf("Game state has not been initialized")
	}

	handState, err := g.loadHandState()
	if err == nil {
		// There is an existing hand state. The game must've crashed and is now restarting.
		// Continue where we left off.
		err := g.resumeGame(g.state, handState)
		if err != nil {
			channelGameLogger.Error().
				Uint32("club", g.config.ClubId).
				Str("game", g.config.GameCode).
				Msgf("Error while resuming game. Error: %s", err.Error())
		}
		return true, nil
	}

	if !g.config.AutoStart && g.state.Status != GameStatus_ACTIVE {
		return false, nil
	}

	playersInSeats := g.state.GetPlayersInSeats()
	countPlayersInSeats := 0
	for _, playerID := range playersInSeats {
		if playerID != 0 {
			countPlayersInSeats++
		}
	}
	if uint32(countPlayersInSeats) < uint32(g.config.MinPlayers) {
		lastTableState := g.state.TableStatus
		// not enough players
		// set table status as not enough players
		g.state.TableStatus = TableStatus_NOT_ENOUGH_PLAYERS

		// TODO:
		// broadcast this message to the players
		// update this message in API server
		if lastTableState != g.state.TableStatus {
			g.broadcastTableState()
		}
		return false, nil
	}

	g.state.TableStatus = TableStatus_GAME_RUNNING

	channelGameLogger.Info().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
		Msg(fmt.Sprintf("Game started. Good luck every one. Players in the table: %d. Waiting list players: %d",
			playersInSeats, len(g.waitingPlayers)))

	// assign the button pos to the first guy in the list
	playersInSeat := g.state.PlayersInSeats
	for seatNo, playerID := range playersInSeat {
		// skip seat no 0
		if seatNo == 0 {
			continue
		}
		if playerID != 0 {
			g.state.ButtonPos = uint32(seatNo)
			break
		}
	}
	g.state.Status = GameStatus_ACTIVE

	g.running = true

	gameMessage := GameMessage{MessageType: GameCurrentStatus, GameId: g.config.GameId, PlayerId: 0}
	gameMessage.GameMessage = &GameMessage_Status{Status: &GameStatusMessage{Status: g.state.Status, TableStatus: g.state.TableStatus}}
	g.broadcastGameMessage(&gameMessage)

	if g.autoDeal {
		g.dealNewHand()
	}

	return true, nil
}

func (g *Game) resumeGame(gameState *GameState, handState *HandState) error {
	channelGameLogger.Info().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
		Msgf("Restarting hand at flow state [%s].", handState.FlowState)

	switch handState.FlowState {
	case FlowState_WAIT_FOR_NEXT_ACTION:
		// We're relying on the client to resend the action message.
		break
	case FlowState_PREPARE_NEXT_ACTION:
		return g.prepareNextAction(gameState, handState)
	case FlowState_MOVE_TO_NEXT_ACTION:
		return g.moveToNextAction(gameState, handState)
	case FlowState_MOVE_TO_NEXT_ROUND:
		return g.moveToNextRound(gameState, handState)
	case FlowState_ALL_PLAYERS_ALL_IN:
		return g.allPlayersAllIn(gameState, handState)
	case FlowState_ONE_PLAYER_REMAINING:
		return g.onePlayerRemaining(gameState, handState)
	case FlowState_SHOWDOWN:
		return g.showdown(gameState, handState)
	case FlowState_HAND_ENDED:
		return g.handEnded(gameState, handState)
	default:
		return fmt.Errorf("Unhandled flow state: %s", handState.FlowState)
	}
	return nil
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
	var handState *HandState

	prevHandState, _ := g.loadHandState()
	prevHandNum := 0
	if prevHandState != nil {
		prevHandNum = int(prevHandState.HandNum)
	}

	moveButton := prevHandNum > 1

	if g.testButtonPos > 0 {
		g.state.ButtonPos = uint32(g.testButtonPos)
		moveButton = false
	}

	handState = &HandState{
		ClubId:        g.config.ClubId,
		GameId:        g.config.GameId,
		HandNum:       uint32(prevHandNum) + 1,
		GameType:      g.config.GameType,
		CurrentState:  HandStatus_DEAL,
		HandStartedAt: uint64(time.Now().Unix()),
	}

	deck := g.testDeckToUse
	if deck == nil || deck.Empty() {
		deck = poker.NewDeck(nil).Shuffle()
	}

	handState.initialize(g.config, g.state, deck, g.state.ButtonPos, moveButton)

	g.state.ButtonPos = handState.GetButtonPos()
	g.testDeckToUse = nil
	g.testButtonPos = -1

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

	gameType := g.config.GameType
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
		Pause:          g.pauseBeforeNextHand,
	}

	//newHand.PlayerCards = playersCards
	handMessage.HandMessage = &HandMessage_NewHand{NewHand: &newHand}
	g.broadcastHandMessage(&handMessage)
	if !RunningTests {
		time.Sleep(time.Duration(g.delays.BeforeDeal) * time.Millisecond)
	}

	if g.pauseBeforeNextHand != 0 {
		channelGameLogger.Info().
			Uint32("club", g.config.ClubId).
			Str("game", g.config.GameCode).
			Uint32("hand", handState.HandNum).
			Msg(fmt.Sprintf("PAUSING the game %d seconds", g.pauseBeforeNextHand))
		time.Sleep(time.Duration(g.pauseBeforeNextHand) * time.Second)
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

	playersCards := make(map[uint32]string)
	activePlayers := uint32(len(g.state.GetPlayersInSeats()))
	cardAnimationTime := time.Duration(activePlayers * g.delays.DealSingleCard * newHand.NoCards)
	// send the cards to each player
	for seatNo, playerID := range g.state.GetPlayersInSeats() {
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

		cards, maskedCards := g.maskCards(playerCards, g.state.PlayersState[playerID].GameTokenInt)
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
		Msg(fmt.Sprintf("Next action: %s", handState.NextSeatAction.PrettyPrint(handState, g.state, g.players)))

	handState.FlowState = FlowState_MOVE_TO_NEXT_ACTION
	g.saveHandState(handState)
	g.moveToNextAction(g.state, handState)
	return nil
}

func (g *Game) saveHandState(handState *HandState) error {
	err := g.manager.handStatePersist.Save(
		g.config.GameCode,
		handState)
	return err
}

func (g *Game) removeHandState(gameState *GameState, handState *HandState) error {
	if gameState == nil || handState == nil {
		return nil
	}

	err := g.manager.handStatePersist.Remove(
		g.config.GameCode)
	return err
}

func (g *Game) loadHandState() (*HandState, error) {
	handState, err := g.manager.handStatePersist.Load(g.config.GameCode)
	return handState, err
}

func (g *Game) broadcastHandMessage(message *HandMessage) {
	message.GameCode = g.config.GameCode
	if *g.messageReceiver != nil {
		if !RunningTests {
			time.Sleep(time.Duration(g.delays.GlobalBroadcastDelay) * time.Millisecond)
		}
		(*g.messageReceiver).BroadcastHandMessage(message)
		if !RunningTests {
			time.Sleep(time.Duration(g.delays.GlobalBroadcastDelay) * time.Millisecond)
		}
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
	ret := make([]*PlayerAtTableState, 0)
	playersInSeats := g.state.GetPlayersInSeats()
	for seatNo, playerID := range playersInSeats {
		if playerID != 0 {
			playerState := g.state.PlayersState[playerID]
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

func (g *Game) getGameInfo(apiServerURL string, gameCode string, retryDelay uint32) (*GameConfig, error) {
	var gameConfig GameConfig
	url := fmt.Sprintf("%s/internal/game-info/game_num/%s", apiServerURL, gameCode)

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
			channelGameLogger.Fatal().
				Str("gameCode", gameCode).
				Msg(fmt.Sprintf("Failed to fetch game info. Error: %d", resp.StatusCode))
		}

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(body, &gameConfig)
		if err != nil {
			return nil, err
		}
		retry = false
	}
	return &gameConfig, nil
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
	if g.state != nil {
		// remove the old handstate
		handState, _ := g.loadHandState()
		if handState != nil {
			g.removeHandState(g.state, handState)
		}
	}
	return nil
}
