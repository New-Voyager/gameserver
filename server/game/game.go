package game

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/proto"
	"voyager.com/encryption"
	"voyager.com/server/crashtest"
	"voyager.com/server/internal/encryptionkey"
	"voyager.com/server/poker"
	"voyager.com/server/util"
)

/**
NOTE: Seat numbers are indexed from 1-9 like the real poker table.
**/

var channelGameLogger = log.With().Str("logger_name", "game::game").Logger()

type GameMessageReceiver interface {
	BroadcastGameMessage(message *GameMessage)
	BroadcastHandMessage(message *HandMessage)
	BroadcastPingMessage(message *PingPongMessage)
	SendHandMessageToPlayer(message *HandMessage, playerID uint64)
	SendGameMessageToPlayer(message *GameMessage, playerID uint64)
}
type Game struct {
	isScriptTest        bool
	manager             *Manager
	end                 chan bool
	stopNetworkCheck    chan bool
	running             bool
	chHand              chan []byte
	chGame              chan []byte
	chPong              chan []byte
	chPlayTimedOut      chan timerMsg
	chResetTimer        chan timerMsg
	chPauseTimer        chan bool
	allPlayers          map[uint64]*Player   // players at the table and the players that are viewing
	messageReceiver     *GameMessageReceiver // receives messages
	actionTimeStart     time.Time
	players             map[uint64]string
	RemainingActionTime uint32
	apiServerUrl        string
	// test driver specific variables
	autoDeal    bool
	prevHandNum uint32

	handSetupPersist *RedisHandsSetupTracker

	inProcessPendingUpdates bool
	config                  *GameConfig
	delays                  Delays
	lock                    sync.Mutex
	timerSeatNo             uint32
	PlayersInSeats          []SeatPlayer
	Status                  GameStatus
	TableStatus             TableStatus
	retryDelayMillis        uint32

	// used for storing player configuration of runItTwicePrompt, muckLosingHand
	playerConfig atomic.Value

	// For player network connnectivity check
	pingTimeoutSec         uint32
	pingStates             map[uint64]*playerPingState
	pingStatesLock         sync.Mutex
	debugConnectivityCheck bool

	db                 *sqlx.DB
	encryptionKeyCache *encryptionkey.Cache
}

func NewPokerGame(
	isScriptTest bool,
	gameManager *Manager,
	messageReceiver *GameMessageReceiver,
	config *GameConfig,
	delays Delays,
	autoDeal bool,
	handStatePersist PersistHandState,
	handSetupPersist *RedisHandsSetupTracker,
	apiServerURL string,
	db *sqlx.DB) (*Game, error) {

	cache, err := encryptionkey.NewCache(32, db)
	if err != nil || cache == nil {
		return nil, errors.Wrap(err, "Unable to instantiate encryption key cache")
	}

	g := Game{
		isScriptTest:           isScriptTest,
		manager:                gameManager,
		messageReceiver:        messageReceiver,
		config:                 config,
		delays:                 delays,
		autoDeal:               autoDeal,
		handSetupPersist:       handSetupPersist,
		apiServerUrl:           apiServerURL,
		retryDelayMillis:       500,
		pingTimeoutSec:         uint32(util.GameServerEnvironment.GetPingTimeout()),
		debugConnectivityCheck: util.GameServerEnvironment.ShouldDebugConnectivityCheck(),
		db:                     db,
		encryptionKeyCache:     cache,
	}
	g.allPlayers = make(map[uint64]*Player)
	g.chGame = make(chan []byte)
	g.chHand = make(chan []byte, 1)
	g.chPong = make(chan []byte, 100)
	g.chPlayTimedOut = make(chan timerMsg)
	g.chResetTimer = make(chan timerMsg)
	g.chPauseTimer = make(chan bool)
	g.end = make(chan bool)
	g.stopNetworkCheck = make(chan bool)
	g.players = make(map[uint64]string)
	g.pingStates = make(map[uint64]*playerPingState)

	playerConfig := make(map[uint64]PlayerConfigUpdate)
	g.playerConfig.Store(playerConfig)

	if g.isScriptTest {
		g.initGameState()
	}
	return &g, nil
}

func (g *Game) playersInSeatsCount() int {
	return len(g.PlayersInSeats) - 1
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

func (g *Game) startNetworkCheck() {
	stopPingLoop := make(chan bool)
	defer func() {
		stopPingLoop <- true
	}()
	go g.startPingLoop(stopPingLoop)

	ended := false
	for !ended {
		select {
		case <-g.stopNetworkCheck:
			ended = true
		case message := <-g.chPong:
			var pongMsg PingPongMessage
			err := proto.Unmarshal(message, &pongMsg)
			if err == nil {
				g.handlePongMessage(&pongMsg)
			}
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (g *Game) initGameState() error {
	g.PlayersInSeats = make([]SeatPlayer, g.config.MaxPlayers+1) // 0 is dealer/observer
	return nil
}

func (g *Game) countActivePlayers() int {
	count := 0
	for _, p := range g.PlayersInSeats {
		if p.Status == PlayerStatus_PLAYING {
			count++
		}
	}
	return count
}

func (g *Game) startGame() (bool, error) {
	var numActivePlayers int
	if !g.isScriptTest {
		// Get game config.
		gameConfig, err := g.getGameInfo(g.apiServerUrl, g.config.GameCode, g.retryDelayMillis)
		if err != nil {
			return false, err
		}

		g.config = gameConfig
		g.Status = gameConfig.Status
		// channelGameLogger.Info().Msgf("New Game Config: %+v\n", g.config)

		// Initialize stateful information in the game object.
		g.initGameState()

		if g.running {
			// Get seat info.
			handInfo, err := g.getNewHandInfo()
			if err != nil {
				return false, err
			}
			numActivePlayers = len(handInfo.PlayersInSeats)
		}
	} else {
		numActivePlayers = g.countActivePlayers()
	}

	if !g.isScriptTest {
		handState, err := g.loadHandState()
		if err == nil {
			// There is an existing hand state. The game must've crashed and is now restarting.
			// Continue where we left off.
			err := g.resumeGame(handState)
			if err != nil {
				channelGameLogger.Error().
					Uint32("club", g.config.ClubId).
					Str("game", g.config.GameCode).
					Msgf("Error while resuming game. Error: %s", err.Error())
			}
			return true, nil
		}
	}

	if !g.config.AutoStart && g.Status != GameStatus_ACTIVE {
		return false, nil
	}

	if numActivePlayers < g.config.MinPlayers {
		lastTableState := g.TableStatus
		// not enough players
		// set table status as not enough players
		g.TableStatus = TableStatus_NOT_ENOUGH_PLAYERS

		// TODO:
		// broadcast this message to the players
		// update this message in API server
		if lastTableState != g.TableStatus {
			g.broadcastTableState()
		}
		return false, nil
	}

	g.TableStatus = TableStatus_GAME_RUNNING

	channelGameLogger.Info().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
		Msg(fmt.Sprintf("Game started. Good luck every one. %d players are in the table.", numActivePlayers))

	g.Status = GameStatus_ACTIVE

	g.running = true

	gameMessage := GameMessage{MessageType: GameCurrentStatus, GameId: g.config.GameId, PlayerId: 0}
	gameMessage.GameMessage = &GameMessage_Status{Status: &GameStatusMessage{Status: g.Status, TableStatus: g.TableStatus}}
	g.broadcastGameMessage(&gameMessage)

	if g.autoDeal {
		err := g.moveAPIServerToNextHand(0)
		for err != nil {
			channelGameLogger.Error().Msg(err.Error())
			time.Sleep(5 * time.Second)
			err = g.moveAPIServerToNextHand(0)
		}

		g.dealNewHand()
	}

	return true, nil
}

func (g *Game) resumeGame(handState *HandState) error {
	channelGameLogger.Info().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
		Msgf("Restarting hand at flow state [%s].", handState.FlowState)

	g.running = true
	var err error
	switch handState.FlowState {
	case FlowState_DEAL_HAND:
		err = g.dealNewHand()
	case FlowState_WAIT_FOR_NEXT_ACTION:
		err = g.onPlayerActed(nil, handState)
	case FlowState_PREPARE_NEXT_ACTION:
		err = g.prepareNextAction(handState, 0)
	case FlowState_MOVE_TO_NEXT_HAND:
		err = g.moveToNextHand(handState)
	default:
		err = fmt.Errorf("Unhandled flow state in resumeGame: %s", handState.FlowState)
	}
	return err
}

func (g *Game) MaskCards(playerCards []byte, gameToken uint64) ([]uint32, uint64) {
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
	var handSetup *TestHandSetup
	var buttonPos uint32
	var sbPos uint32
	var bbPos uint32
	newHandNum := g.prevHandNum + 1
	var newHandInfo *NewHandInfo
	var pauseBeforeHand uint32
	var err error

	crashtest.Hit(g.config.GameCode, crashtest.CrashPoint_DEAL_1, 0)

	gameType := g.config.GameType
	playersInSeats := make(map[uint32]*PlayerInSeatState)

	testHandsSetup, err := g.handSetupPersist.Load(g.config.GameCode)
	if err == nil {
		handSetup = testHandsSetup
	}
	pauseBeforeHand = handSetup.Pause

	if !g.isScriptTest {
		// we are not running tests
		// get new hand information from the API server
		// new hand information contains players in seats/balance/status, game type, announce new game
		newHandInfo, err = g.getNewHandInfo()
		if err != nil {
			// right now panic (shouldn't happen)
			panic(err)
		}
		if newHandInfo.TableStatus != TableStatus_GAME_RUNNING {
			return nil
		}

		buttonPos = newHandInfo.ButtonPos
		sbPos = newHandInfo.SbPos
		bbPos = newHandInfo.BbPos

		gameType = newHandInfo.GameType
		newHandNum = newHandInfo.HandNum
		playerUpdateConfig := make(map[uint64]PlayerConfigUpdate)
		for seatNo, player := range g.PlayersInSeats {
			g.PlayersInSeats[seatNo] = SeatPlayer{}
			playerUpdateConfig[player.PlayerID] = PlayerConfigUpdate{
				PlayerId:         player.PlayerID,
				MuckLosingHand:   player.MuckLosingHand,
				RunItTwicePrompt: player.RunItTwicePrompt,
			}
		}
		g.playerConfig.Store(playerUpdateConfig)

		if newHandInfo.AnnounceGameType {
			params := []string{
				newHandInfo.GameType.String(),
			}
			announcement := &Announcement{
				Type:   AnnouncementNewGameType,
				Params: params,
			}
			_ = announcement
			// // announce new game type
			handMessage := HandMessage{
				GameId:     g.config.GameId,
				ClubId:     g.config.ClubId,
				HandNum:    newHandInfo.HandNum,
				HandStatus: HandStatus_DEAL,
				MessageId:  g.generateMsgID("ANNOUNCEMENT", newHandInfo.HandNum, HandStatus_DEAL, 0, "", 0),
				Messages: []*HandMessageItem{
					{
						MessageType: HandAnnouncement,
						Content:     &HandMessageItem_Announcement{Announcement: announcement},
					},
				},
			}
			g.broadcastHandMessage(&handMessage)
		}

		/*
			type SeatPlayer struct {
				SeatNo       uint32
				OpenSeat     bool
				PlayerID     uint64 `json:"playerId"`
				PlayerUUID   string `json:"playerUuid"`
				Name         string
				BuyIn        float32
				Stack        float32
				Status       PlayerStatus
				GameToken    string
				GameTokenInt uint64
				RunItTwice   bool
				BuyInTimeExpAt string
				BreakTimeExpAt string
			}
		*/
		for _, playerInSeat := range newHandInfo.PlayersInSeats {
			if playerInSeat.SeatNo <= uint32(g.config.MaxPlayers) {
				g.PlayersInSeats[playerInSeat.SeatNo] = playerInSeat
			}
			playerUpdateConfig[playerInSeat.PlayerID] = PlayerConfigUpdate{
				PlayerId:         playerInSeat.PlayerID,
				MuckLosingHand:   playerInSeat.MuckLosingHand,
				RunItTwicePrompt: playerInSeat.RunItTwicePrompt,
			}
			playersInSeats[playerInSeat.SeatNo] = &PlayerInSeatState{
				Status:       playerInSeat.Status,
				Stack:        playerInSeat.Stack,
				PlayerId:     playerInSeat.PlayerID,
				Name:         playerInSeat.Name,
				BuyInExpTime: playerInSeat.BuyInTimeExpAt,
				BreakExpTime: playerInSeat.BreakTimeExpAt,
			}
		}

		// change game configuration
		g.config.GameType = newHandInfo.GameType
		g.config.SmallBlind = float64(newHandInfo.SmallBlind)
		g.config.BigBlind = float64(newHandInfo.BigBlind)
	} else {
		// We're in a script test (no api server).

		// assign the button pos to the first guy in the list
		for _, player := range g.PlayersInSeats {
			if player.PlayerID != 0 {
				buttonPos = player.SeatNo
				break
			}
		}

		sbPos = 0
		bbPos = 0
	}

	if handSetup != nil && handSetup.ButtonPos > 0 {
		buttonPos = handSetup.ButtonPos
	}

	handState = &HandState{
		ClubId:        g.config.ClubId,
		GameId:        g.config.GameId,
		HandNum:       uint32(newHandNum),
		GameType:      gameType,
		CurrentState:  HandStatus_DEAL,
		HandStartedAt: uint64(time.Now().Unix()),
	}

	handState.initialize(g.config, handSetup, buttonPos, sbPos, bbPos, g.PlayersInSeats)

	channelGameLogger.Trace().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
		Uint32("hand", handState.HandNum).
		Msg(fmt.Sprintf("Table: %s", handState.PrintTable(g.players)))

	// send a new hand message to all players
	newHand := NewHand{
		HandNum:        handState.HandNum,
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
		Pause:          pauseBeforeHand,
		PlayersInSeats: playersInSeats,
	}

	handMessage := HandMessage{
		GameId:     g.config.GameId,
		ClubId:     g.config.ClubId,
		HandNum:    handState.HandNum,
		HandStatus: handState.CurrentState,
		MessageId:  g.generateMsgID("NEW_HAND", handState.HandNum, handState.CurrentState, 0, "", handState.CurrentActionNum),
		Messages: []*HandMessageItem{
			{
				MessageType: HandNewHand,
				Content:     &HandMessageItem_NewHand{NewHand: &newHand},
			},
		},
	}

	g.broadcastHandMessage(&handMessage)
	crashtest.Hit(g.config.GameCode, crashtest.CrashPoint_DEAL_2, 0)

	if !util.GameServerEnvironment.ShouldDisableDelays() {
		time.Sleep(time.Duration(g.delays.BeforeDeal) * time.Millisecond)
	}

	if pauseBeforeHand != 0 {
		channelGameLogger.Info().
			Uint32("club", g.config.ClubId).
			Str("game", g.config.GameCode).
			Uint32("hand", handState.HandNum).
			Msg(fmt.Sprintf("PAUSING the game %d seconds", pauseBeforeHand))
		time.Sleep(time.Duration(pauseBeforeHand) * time.Second)
	}

	// indicate the clients card distribution began
	handMessage = HandMessage{
		GameId:     g.config.GameId,
		ClubId:     g.config.ClubId,
		GameCode:   g.config.GameCode,
		HandNum:    handState.HandNum,
		HandStatus: handState.CurrentState,
		MessageId:  g.generateMsgID("DEAL", handState.HandNum, handState.CurrentState, 0, "", handState.CurrentActionNum),
		Messages: []*HandMessageItem{
			{
				MessageType: HandDealStarted,
			},
		},
	}
	g.broadcastHandMessage(&handMessage)
	crashtest.Hit(g.config.GameCode, crashtest.CrashPoint_DEAL_3, 0)

	playersCards := make(map[uint32]string)
	numActivePlayers := uint32(g.countActivePlayers())
	cardAnimationTime := time.Duration(numActivePlayers * g.delays.DealSingleCard * newHand.NoCards)
	// send the cards to each player
	for _, player := range g.PlayersInSeats {
		if player.Status != PlayerStatus_PLAYING {
			// Open seat or not playing this hand
			continue
		}

		// if the player balance is 0, then don't deal card to him
		if _, ok := handState.PlayersState[player.PlayerID]; !ok {
			handState.ActiveSeats[int(player.SeatNo)] = 0
			continue
		}

		// seatNo is the key, cards are value
		playerCards := handState.PlayersCards[uint32(player.SeatNo)]
		dealCards := HandDealCards{SeatNo: uint32(player.SeatNo)}

		tmpGameToken := uint64(0)
		cards, maskedCards := g.MaskCards(playerCards, tmpGameToken)
		playersCards[player.SeatNo] = fmt.Sprintf("%d", maskedCards)
		dealCards.Cards = fmt.Sprintf("%d", maskedCards)
		dealCards.CardsStr = poker.CardsToString(cards)

		//messageData, _ := proto.Marshal(&message)
		handMessage := HandMessage{
			GameId:    g.config.GameId,
			ClubId:    g.config.ClubId,
			PlayerId:  player.PlayerID,
			MessageId: g.generateMsgID("CARDS", handState.HandNum, handState.CurrentState, player.PlayerID, "", handState.CurrentActionNum),
			Messages: []*HandMessageItem{
				{
					MessageType: HandDeal,
					Content:     &HandMessageItem_DealCards{DealCards: &dealCards},
				},
			},
		}
		b, _ := proto.Marshal(&handMessage)

		if *g.messageReceiver != nil {
			(*g.messageReceiver).SendHandMessageToPlayer(&handMessage, player.PlayerID)

		} else {
			g.allPlayers[player.PlayerID].chHand <- b
		}

		crashtest.Hit(g.config.GameCode, crashtest.CrashPoint_DEAL_4, 0)
	}
	if !util.GameServerEnvironment.ShouldDisableDelays() {
		time.Sleep(cardAnimationTime * time.Millisecond)
	}

	// print next action
	channelGameLogger.Trace().
		Uint32("club", g.config.ClubId).
		Str("game", g.config.GameCode).
		Uint32("hand", handState.HandNum).
		Msg(fmt.Sprintf("Next action: %s", handState.NextSeatAction.PrettyPrint(handState, g.PlayersInSeats)))

	handState.FlowState = FlowState_MOVE_TO_NEXT_ACTION
	msgItems, err := g.moveToNextAction(handState)
	if err != nil {
		return err
	}
	handMsg := HandMessage{
		ClubId:     g.config.ClubId,
		GameId:     g.config.GameId,
		HandNum:    handState.HandNum,
		HandStatus: handState.CurrentState,
		MessageId:  g.generateMsgID("INITIAL_ACTION", handState.HandNum, handState.CurrentState, 0, "", handState.CurrentActionNum),
		Messages:   msgItems,
	}
	g.broadcastHandMessage(&handMsg)
	crashtest.Hit(g.config.GameCode, crashtest.CrashPoint_DEAL_5, 0)

	g.saveHandState(handState)
	crashtest.Hit(g.config.GameCode, crashtest.CrashPoint_DEAL_6, 0)
	return nil
}

func (g *Game) generateMsgID(prefix string, handNum uint32, handStatus HandStatus, playerID uint64, originalMsgID string, currentActionNum uint32) string {
	return fmt.Sprintf("%s:%d:%s:%d:%s:%d", prefix, handNum, handStatus, playerID, originalMsgID, currentActionNum)
}

func (g *Game) GenerateMsgID(prefix string, handNum uint32, handStatus HandStatus, playerID uint64, originalMsgID string, currentActionNum uint32) string {
	return g.generateMsgID(prefix, handNum, handStatus, playerID, originalMsgID, currentActionNum)
}

func (g *Game) saveHandState(handState *HandState) error {
	err := g.manager.handStatePersist.Save(
		g.config.GameCode,
		handState)
	return err
}

func (g *Game) removeHandState() error {
	err := g.manager.handStatePersist.Remove(g.config.GameCode)
	return err
}

func (g *Game) loadHandState() (*HandState, error) {
	handState, err := g.manager.handStatePersist.Load(g.config.GameCode)
	return handState, err
}

func (g *Game) GetHandState() (*HandState, error) {
	handState, err := g.manager.handStatePersist.Load(g.config.GameCode)
	return handState, err
}

func (g *Game) broadcastHandMessage(message *HandMessage) {
	message.GameCode = g.config.GameCode
	if *g.messageReceiver != nil {
		(*g.messageReceiver).BroadcastHandMessage(message)
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
		(*g.messageReceiver).BroadcastGameMessage(message)
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

func (g *Game) SendPongMessage(message *PingPongMessage) {
	message.GameCode = g.config.GameCode
	b, _ := proto.Marshal(message)
	g.chPong <- b
}

func (g *Game) sendHandMessageToPlayer(message *HandMessage, playerID uint64) {
	message.GameCode = g.config.GameCode
	if *g.messageReceiver != nil {
		(*g.messageReceiver).SendHandMessageToPlayer(message, playerID)
	} else {
		player := g.allPlayers[playerID]
		if player == nil {
			return
		}
		b, _ := proto.Marshal(message)
		player.chHand <- b
	}
}

func (g *Game) BroadcastPingMessage(message *PingPongMessage) {
	message.GameCode = g.config.GameCode
	if *g.messageReceiver != nil {
		(*g.messageReceiver).BroadcastPingMessage(message)
	}
}

func (g *Game) addPlayer(player *Player, buyIn float32) error {
	g.lock.Lock()
	defer g.lock.Unlock()
	g.allPlayers[player.PlayerID] = player

	// add the player to playerSeatInfos
	g.PlayersInSeats[int(player.SeatNo)] = SeatPlayer{
		Name:       player.PlayerName,
		PlayerID:   player.PlayerID,
		PlayerUUID: fmt.Sprintf("%d", player.PlayerID),
		Status:     PlayerStatus_PLAYING,
		Stack:      buyIn,
		OpenSeat:   false,
		SeatNo:     player.SeatNo,
	}
	return nil
}

func (g *Game) getPlayersAtTable() ([]*PlayerAtTableState, error) {
	ret := make([]*PlayerAtTableState, 0)
	for _, player := range g.PlayersInSeats {
		if player.PlayerID != 0 {
			playerAtTable := &PlayerAtTableState{
				PlayerId:       player.PlayerID,
				SeatNo:         player.SeatNo,
				BuyIn:          player.BuyIn,
				CurrentBalance: player.Stack,
				Status:         player.Status,
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

	// debug flag
	ignore := false
	for retry {
		// SOMA: I added this for debugging
		// I delete games (resetDB) when testing from the app
		// I want the game server to ignore the games that don't exist
		if ignore {
			time.Sleep(time.Duration(6000000))
			continue
		}

		resp, err := http.Get(url)
		if resp == nil {
			channelGameLogger.Error().Msgf("Connection to API server is lost. Waiting for %.3f seconds before retrying", float32(retryDelay)/1000)
			time.Sleep(time.Duration(retryDelay) * time.Millisecond)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			channelGameLogger.Error().
				Str("gameCode", gameCode).
				Msgf("Failed to fetch game info from api server (%s). Error: %d", apiServerURL, resp.StatusCode)
			time.Sleep(time.Duration(retryDelay) * time.Millisecond)
			ignore = true
			continue
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

func (g *Game) GameStarted() {
	go g.runGame()
	if !g.isScriptTest {
		go g.startNetworkCheck()
	}
}

func (g *Game) GameEnded() error {
	g.removeHandState()
	g.end <- true
	g.stopNetworkCheck <- true
	return nil
}

func (g *Game) GetEncryptionKey(playerID uint64) (string, error) {
	encryptionKey, err := g.encryptionKeyCache.Get(playerID)
	if err != nil {
		return "", err
	}
	return encryptionKey, nil
}

func (g *Game) EncryptForPlayer(data []byte, playerID uint64) ([]byte, error) {
	encryptionKey, err := g.GetEncryptionKey(playerID)
	if err != nil {
		return nil, fmt.Errorf("Unable to get encryption key for player %d", playerID)
	}
	encryptedData, err := encryption.EncryptWithUUIDStrKey(data, encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("Unable to encrypt message to player %d", playerID)
	}
	return encryptedData, nil
}

func (g *Game) EncryptAndB64ForPlayer(data []byte, playerID uint64) (string, error) {
	encryptedData, err := g.EncryptForPlayer(data, playerID)
	if err != nil {
		return "", err
	}
	return encryption.B64EncodeToString(encryptedData), nil
}
