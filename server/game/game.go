package game

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"runtime/debug"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"voyager.com/encryption"
	"voyager.com/logging"
	"voyager.com/server/crashtest"
	"voyager.com/server/internal/encryptionkey"
	"voyager.com/server/networkcheck"
	"voyager.com/server/poker"
	"voyager.com/server/rpc"
	"voyager.com/server/timer"
	"voyager.com/server/util"
)

/**
NOTE: Seat numbers are indexed from 1-9 like the real poker table.
**/

type MessageSender interface {
	BroadcastGameMessage(message *GameMessage, noLog bool)
	BroadcastHandMessage(message *HandMessage)
	SendHandMessageToPlayer(message *HandMessage, playerID uint64)
	SendHandMessageToTournamentPlayer(message *HandMessage, tournamentID uint32, playerID uint64)
	SendGameMessageToPlayer(message *GameMessage, playerID uint64)
}
type Game struct {
	gameID       uint64
	gameCode     string
	tournamentID uint64
	tableNo      uint32

	logger *zerolog.Logger

	manager        *Manager
	end            chan bool
	running        bool
	chHand         chan []byte
	chGame         chan []byte
	chPlayTimedOut chan timer.TimerMsg
	messageSender  *MessageSender // receives messages
	apiServerURL   string
	tournamentURL  string

	// test driver specific variables
	isScriptTest          bool
	scriptTestPrevHandNum uint32
	scriptTestPlayers     map[uint64]*Player // players at the table and the players that are viewing

	handSetupPersist *RedisHandsSetupTracker

	isHandInProgress bool
	testGameConfig   *TestGameConfig
	delays           Delays
	lock             sync.Mutex
	PlayersInSeats   []SeatPlayer
	Status           GameStatus
	TableStatus      TableStatus
	maxRetries       uint32
	retryDelayMillis uint32

	// Whether to allow fractional chip or not
	chipUnit ChipUnit

	// used for storing player configuration of runItTwicePrompt, muckLosingHand
	//playerConfig atomic.Value

	timerCushionSec    uint32
	actionTimer        *timer.ActionTimer
	actionTimer2       *timer.ActionTimer
	networkCheck       *networkcheck.NetworkCheck
	encryptionKeyCache *encryptionkey.Cache
}

func NewPokerGame(
	gameID uint64,
	gameCode string,
	tournamentID uint64,
	tableNo uint32,
	isScriptTest bool,
	gameManager *Manager,
	messageSender *MessageSender,
	delays Delays,
	handStatePersist PersistHandState,
	handSetupPersist *RedisHandsSetupTracker,
	encryptionKeyCache *encryptionkey.Cache,
	apiServerURL string) (*Game, error) {

	logger := logging.GetZeroLogger("game::Game", nil).With().
		Uint64(logging.GameIDKey, gameID).
		Str(logging.GameCodeKey, gameCode).
		Logger()
	g := Game{
		logger:             &logger,
		tournamentID:       tournamentID,
		tableNo:            tableNo,
		gameID:             gameID,
		gameCode:           gameCode,
		isScriptTest:       isScriptTest,
		manager:            gameManager,
		messageSender:      messageSender,
		delays:             delays,
		handSetupPersist:   handSetupPersist,
		apiServerURL:       apiServerURL,
		maxRetries:         10,
		retryDelayMillis:   2000,
		timerCushionSec:    5,
		encryptionKeyCache: encryptionKeyCache,
	}
	g.scriptTestPlayers = make(map[uint64]*Player)
	g.chGame = make(chan []byte, 10)
	g.chHand = make(chan []byte, 10)
	g.end = make(chan bool, 10)
	g.chPlayTimedOut = make(chan timer.TimerMsg, 10)
	timer1Logger := logging.GetZeroLogger("timer::ActionTimer", nil).
		With().Uint64(logging.GameIDKey, gameID).
		Str(logging.GameCodeKey, gameCode).
		Int(logging.TimerIDKey, 1).
		Logger()
	g.actionTimer = timer.NewActionTimer(&timer1Logger, g.queueActionTimeoutMsg, g.crashHandler)

	// Timer 2 is used for run-it-twice player 2.
	timer2Logger := logging.GetZeroLogger("timer::ActionTimer", nil).
		With().Uint64(logging.GameIDKey, gameID).
		Str(logging.GameCodeKey, gameCode).
		Int(logging.TimerIDKey, 2).
		Logger()
	g.actionTimer2 = timer.NewActionTimer(&timer2Logger, g.queueActionTimeoutMsg, g.crashHandler)

	networkCheckLogger := logging.GetZeroLogger("NetworkCheck", nil).
		With().Uint64(logging.GameIDKey, gameID).
		Str(logging.GameCodeKey, gameCode).
		Logger()
	g.networkCheck = networkcheck.NewNetworkCheck(&networkCheckLogger, g.gameID, g.gameCode, g.crashHandler, g.onClientConnLost, g.onClientConnRestored)

	if g.isScriptTest {
		g.initTestGameState()
	}
	return &g, nil
}

func NewTestPokerGame(
	gameID uint64,
	gameCode string,
	isScriptTest bool,
	gameManager *Manager,
	messageSender *MessageSender,
	config *TestGameConfig,
	delays Delays,
	handStatePersist PersistHandState,
	handSetupPersist *RedisHandsSetupTracker,
	encryptionKeyCache *encryptionkey.Cache,
	apiServerURL string) (*Game, error) {

	logger := logging.GetZeroLogger("game::Game", nil).With().
		Uint64(logging.GameIDKey, gameID).
		Str(logging.GameCodeKey, gameCode).
		Logger()
	g := Game{
		logger:             &logger,
		gameID:             gameID,
		gameCode:           gameCode,
		isScriptTest:       isScriptTest,
		manager:            gameManager,
		messageSender:      messageSender,
		testGameConfig:     config,
		delays:             delays,
		handSetupPersist:   handSetupPersist,
		apiServerURL:       apiServerURL,
		maxRetries:         10,
		retryDelayMillis:   2000,
		encryptionKeyCache: encryptionKeyCache,
	}
	g.scriptTestPlayers = make(map[uint64]*Player)
	g.chGame = make(chan []byte, 10)
	g.chHand = make(chan []byte, 10)
	g.end = make(chan bool, 10)
	g.chPlayTimedOut = make(chan timer.TimerMsg)
	timer1Logger := logging.GetZeroLogger("ActionTimer", nil).
		With().Uint64(logging.GameIDKey, gameID).
		Str(logging.GameCodeKey, gameCode).
		Int(logging.TimerIDKey, 1).
		Logger()
	g.actionTimer = timer.NewActionTimer(&timer1Logger, g.queueActionTimeoutMsg, g.crashHandler)

	// Timer 2 is used for run-it-twice player 2.
	timer2Logger := logging.GetZeroLogger("ActionTimer", nil).
		With().Uint64(logging.GameIDKey, gameID).
		Str(logging.GameCodeKey, gameCode).
		Int(logging.TimerIDKey, 2).
		Logger()

	g.actionTimer2 = timer.NewActionTimer(&timer2Logger, g.queueActionTimeoutMsg, g.crashHandler)

	networkCheckLogger := logging.GetZeroLogger("NetworkCheck", nil).
		With().Uint64(logging.GameIDKey, gameID).
		Str(logging.GameCodeKey, gameCode).
		Logger()

	g.networkCheck = networkcheck.NewNetworkCheck(&networkCheckLogger, g.gameID, g.gameCode, g.crashHandler, g.onClientConnLost, g.onClientConnRestored)

	if g.isScriptTest {
		g.initTestGameState()
	}

	return &g, nil
}

func (g *Game) playersInSeatsCount() int {
	count := 0
	for _, player := range g.PlayersInSeats {
		if player.PlayerID != 0 {
			count++
		}
	}
	return count
}

func (g *Game) GameStarted() error {
	g.actionTimer.Run()
	g.actionTimer2.Run()
	g.networkCheck.Run()

	go g.runGame()
	return nil
}

func (g *Game) GameEnded() error {
	g.logger.Info().Msg("Cleaning up game")
	g.end <- true
	g.actionTimer.Destroy()
	g.actionTimer2.Destroy()
	g.networkCheck.Destroy()
	g.removeHandState()
	g.logger.Info().Msg("Finished cleaning up game")
	return nil
}

func (g *Game) runGame() {
	defer func() {
		if err := recover(); err != nil {
			// Panic occurred.
			debug.PrintStack()
			g.logger.Error().
				Msgf("runGame returning due to panic: %s\nStack Trace:\n%s", err, string(debug.Stack()))

			g.crashHandler()
			g.logger.Info().Msg("Requesting to end game")
			_, err2 := g.requestEndGame(true)
			if err2 != nil {
				g.logger.Error().Err(err2).Msgf("Error in requestEndGame in panic handler")
			}
		}
	}()

	ended := false
	for !ended {
		if g.isScriptTest && !g.running {
			started, err := g.startTestGame()
			if err != nil {
				g.logger.Error().Msg(fmt.Sprintf("Failed to start game: %v", err))
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
			if err != nil {
				g.logger.Error().Err(err).Msgf("Could not proto-unmarshal hand msg. Ignoring the msg")
				break
			}
			err = g.handleHandMessage(&handMessage)
			if err != nil {
				g.logger.Error().Err(err).Msgf("Could not process hand msg")
				panic("Could not process hand message.")
			}
		case message := <-g.chGame:
			var gameMessage GameMessage
			err := proto.Unmarshal(message, &gameMessage)
			if err != nil {
				g.logger.Error().Err(err).Msg("Could not proto-unmarshal game msg. Ignoring the msg")
				break
			}
			err = g.handleGameMessage(&gameMessage)
			if err != nil {
				g.logger.Error().
					Err(err).
					Str(logging.MsgTypeKey, gameMessage.MessageType).
					Msg("Could not process game msg")
				panic("Could not process game msg")
			}
		case timeoutMsg := <-g.chPlayTimedOut:
			err := g.handlePlayTimeout(timeoutMsg)
			if err != nil {
				g.logger.Error().Err(err).Msgf("Could not process player timeout")
				panic("Could not process player timeout")
			}
		default:
			if g.isScriptTest && !g.running {
				playersInSeats := g.playersInSeatsCount()
				g.logger.Trace().
					Msg(fmt.Sprintf("Waiting for players to join. %d players in the table, and waiting for %d more players",
						playersInSeats, g.testGameConfig.MinPlayers-playersInSeats))
				time.Sleep(50 * time.Millisecond)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	g.manager.gameEnded(g)
}

func (g *Game) crashHandler() {
	g.manager.OnGameCrash(g.gameID, g.gameCode)
}

func (g *Game) initTestGameState() error {
	// TODO: Initialize this for the real game using hand info.
	g.PlayersInSeats = make([]SeatPlayer, g.testGameConfig.MaxPlayers+1) // 0 is dealer/observer
	return nil
}

func (g *Game) countActivePlayers() int {
	count := 0
	for _, p := range g.PlayersInSeats {
		if p.Status == PlayerStatus_PLAYING && p.Inhand {
			count++
		}
	}
	return count
}

func (g *Game) startTestGame() (bool, error) {
	if !g.testGameConfig.AutoStart && g.Status != GameStatus_ACTIVE {
		return false, nil
	}

	numActivePlayers := g.countActivePlayers()
	if numActivePlayers < g.testGameConfig.MinPlayers {
		lastTableState := g.TableStatus
		g.TableStatus = TableStatus_NOT_ENOUGH_PLAYERS

		if lastTableState != g.TableStatus {
			g.broadcastTableState()
		}
		return false, nil
	}

	g.logger.Info().Msgf("Test game starting")

	g.Status = GameStatus_ACTIVE
	g.TableStatus = TableStatus_GAME_RUNNING
	g.running = true

	return true, nil
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

	// TODO: mask it.
	mask := uint64(0)
	//mask := gameToken
	maskCards := uint64(cardsUint64)
	if mask != 0 {
		maskCards = uint64(cardsUint64 ^ mask)
	}
	maskedCards := uint64(maskCards) & uint64(0x0000FFFFFFFFFFFF)
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
	case GameType_SIX_CARD_PLO:
		noCards = 6
	case GameType_SIX_CARD_PLO_HILO:
		noCards = 6
	}
	return uint32(noCards)
}

func (g *Game) DealTournamentHand(newHandInfo *NewHandInfo) error {
	g.dealNewHand(newHandInfo)
	return nil
}

func (g *Game) dealNewHand(newHandInfo *NewHandInfo) error {
	var handState *HandState
	var testHandSetup *TestHandSetup
	var buttonPos uint32
	var sbPos uint32
	var bbPos uint32
	var newHandNum uint32
	var gameType GameType
	var err error

	crashtest.Hit(g.gameCode, crashtest.CrashPoint_DEAL_1, 0)

	v, err := g.handSetupPersist.Load(g.gameCode)
	if err == nil {
		testHandSetup = v
	}

	if testHandSetup != nil {
		pauseBeforeHand := testHandSetup.Pause
		if pauseBeforeHand != 0 {
			g.logger.Debug().
				Uint32(logging.HandNumKey, newHandNum).
				Msg(fmt.Sprintf("PAUSING the game %d seconds", pauseBeforeHand))
			time.Sleep(time.Duration(pauseBeforeHand) * time.Second)
		}
	}

	// tournament hand
	if newHandInfo != nil {
		buttonPos = newHandInfo.ButtonPos
		sbPos = newHandInfo.SbPos
		bbPos = newHandInfo.BbPos
		g.PlayersInSeats = make([]SeatPlayer, newHandInfo.MaxPlayers+1) // 0 is dealer/observer
		for _, playerInSeat := range newHandInfo.PlayersInSeats {
			if playerInSeat.SeatNo <= uint32(newHandInfo.MaxPlayers) {
				g.PlayersInSeats[playerInSeat.SeatNo] = playerInSeat
			}
		}
		g.tournamentURL = newHandInfo.TournamentURL
	}

	var resultPauseTime uint32
	if !g.isScriptTest && g.tournamentID == 0 {
		// we are not running tests
		// get new hand information from the API server
		// new hand information contains players in seats/balance/status, game type, announce new game
		newHandInfo, err = g.getNewHandInfo()
		if err != nil {
			return errors.Wrap(err, "Error in getNewhandInfo")
		}
		if newHandInfo.TableStatus != TableStatus_GAME_RUNNING {
			return nil
		}
		g.chipUnit = newHandInfo.ChipUnit
		g.PlayersInSeats = make([]SeatPlayer, newHandInfo.MaxPlayers+1) // 0 is dealer/observer
		resultPauseTime = newHandInfo.ResultPauseTime
		buttonPos = newHandInfo.ButtonPos
		sbPos = newHandInfo.SbPos
		bbPos = newHandInfo.BbPos

		gameType = newHandInfo.GameType
		newHandNum = newHandInfo.HandNum

		for _, p := range newHandInfo.PlayersInSeats {
			g.encryptionKeyCache.Add(p.PlayerID, p.EncryptionKey)
		}

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
				BuyIn        float64
				Stack        float64
				Status       PlayerStatus
				GameToken    string
				GameTokenInt uint64
				RunItTwice   bool
				BuyInTimeExpAt string
				BreakTimeExpAt string
			}
		*/
		for _, playerInSeat := range newHandInfo.PlayersInSeats {
			if playerInSeat.SeatNo <= uint32(newHandInfo.MaxPlayers) {
				g.PlayersInSeats[playerInSeat.SeatNo] = playerInSeat
			}
		}
	} else if g.isScriptTest {
		// We're in a script test (no api server).
		gameType = g.testGameConfig.GameType
		if g.testGameConfig.ChipUnit == "CENT" {
			g.chipUnit = ChipUnit_CENT
		} else {
			g.chipUnit = ChipUnit_DOLLAR
		}

		newHandNum = g.scriptTestPrevHandNum + 1
		if testHandSetup != nil {
			if testHandSetup.HandNum != 0 {
				newHandNum = testHandSetup.HandNum
			}
		}

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

	if testHandSetup != nil {
		if testHandSetup.ButtonPos > 0 {
			buttonPos = testHandSetup.ButtonPos
		}
	}

	handState = &HandState{
		GameId:        g.gameID,
		HandNum:       newHandNum,
		GameType:      gameType,
		CurrentState:  HandStatus_DEAL,
		HandStartedAt: uint64(time.Now().Unix()),
	}

	err = handState.initialize(g.testGameConfig, newHandInfo, testHandSetup, buttonPos, sbPos, bbPos, g.PlayersInSeats, g.chipUnit)
	if err != nil {
		return errors.Wrapf(err, "Error while initializing hand state")
	}
	if handState.GetNoActiveSeats() < 2 {
		// Shouldn't get here. Just being resilient against api server.
		g.logger.Warn().Msgf("Aborting dealing due to not enough active seats (%d). Players In Seats: %+v", handState.GetNoActiveSeats(), handState.GetPlayersInSeats())

		return NotReadyToDealError{
			Msg: "Not enough active seats",
		}
	}

	if testHandSetup != nil {
		resultPauseTime = testHandSetup.ResultPauseTime
	}
	if resultPauseTime == 0 {
		g.logger.Warn().
			Msgf("Using the default result delay value (delays.ResultPerWinner = %d) instead of the one from the hand config", g.delays.ResultPerWinner)
		resultPauseTime = g.delays.ResultPerWinner
	}

	handState.ResultPauseTime = resultPauseTime

	if !g.isScriptTest {
		var playerIDs []uint64
		for _, playerID := range handState.GetActiveSeats() {
			if playerID != 0 {
				playerIDs = append(playerIDs, playerID)
			}
		}
	}

	if g.isScriptTest {
		g.logger.Trace().
			Uint32(logging.HandNumKey, handState.HandNum).
			Msg(fmt.Sprintf("Table: %s", handState.PrintTable(g.scriptTestPlayers)))
	}

	playersActed := make(map[uint32]*PlayerActRound)
	for seatNo, action := range handState.PlayersActed {
		if action.Action == ACTION_EMPTY_SEAT {
			continue
		}
		playersActed[uint32(seatNo)] = action
	}
	bettingState := handState.RoundState[uint32(handState.CurrentState)]
	currentBettingRound := bettingState.Betting

	handPlayerInSeats := make(map[uint32]*PlayerInSeatState)
	for _, playerInSeat := range handState.PlayersInSeats {
		copiedState := &PlayerInSeatState{
			SeatNo:            playerInSeat.SeatNo,
			Status:            playerInSeat.Status,
			Stack:             playerInSeat.Stack,
			PlayerId:          playerInSeat.PlayerId,
			Name:              playerInSeat.Name,
			BuyInExpTime:      playerInSeat.BuyInExpTime,
			BreakExpTime:      playerInSeat.BreakExpTime,
			Inhand:            playerInSeat.Inhand,
			RunItTwice:        playerInSeat.RunItTwice,
			MissedBlind:       playerInSeat.MissedBlind,
			ButtonStraddle:    playerInSeat.ButtonStraddle,
			MuckLosingHand:    playerInSeat.MuckLosingHand,
			AutoStraddle:      playerInSeat.AutoStraddle,
			ButtonStraddleBet: playerInSeat.ButtonStraddleBet,
		}
		handPlayerInSeats[playerInSeat.SeatNo] = copiedState
		handPlayerInSeats[playerInSeat.SeatNo].Stack = playerInSeat.Stack - currentBettingRound.SeatBet[playerInSeat.SeatNo]
	}

	var nextSeatNo uint32
	if handState.NextSeatAction != nil {
		nextSeatNo = handState.NextSeatAction.SeatNo
	}
	potUpdates := float64(0)
	pots := make([]float64, 0)

	currentRoundState, ok := handState.RoundState[uint32(handState.CurrentState)]
	if ok {
		for _, bet := range currentRoundState.Betting.SeatBet {
			potUpdates = potUpdates + bet
		}
	}

	// send a new hand message to all players
	newHand := NewHand{
		HandNum:        handState.HandNum,
		ButtonPos:      handState.ButtonPos,
		SbPos:          handState.SmallBlindPos,
		BbPos:          handState.BigBlindPos,
		NextActionSeat: nextSeatNo,
		NoCards:        g.NumCards(gameType),
		GameType:       gameType,
		SmallBlind:     handState.SmallBlind,
		BigBlind:       handState.BigBlind,
		BringIn:        handState.BringIn,
		Straddle:       handState.Straddle,
		Ante:           handState.Ante,
		PlayersInSeats: handPlayerInSeats,
		PlayersActed:   playersActed,
		BombPot:        handState.BombPot,
		BombPotBet:     handState.BombPotBet,
		DoubleBoard:    handState.DoubleBoard,
		PotUpdates:     potUpdates,
		Pots:           pots,
	}

	handMessage := HandMessage{
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
	crashtest.Hit(g.gameCode, crashtest.CrashPoint_DEAL_2, 0)

	// indicate the clients card distribution began
	handMessage = HandMessage{
		GameCode:   g.gameCode,
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
	crashtest.Hit(g.gameCode, crashtest.CrashPoint_DEAL_3, 0)

	playersCards := make(map[uint32]string)
	// send the cards to each player
	for _, player := range handState.PlayersInSeats {
		if !player.Inhand {
			// Open seat or not playing this hand
			continue
		}

		// if the player balance is 0, then don't deal card to him
		if player.Stack == 0 {
			if handState.AllInPlayers[player.SeatNo] == 0 {
				handState.ActiveSeats[int(player.SeatNo)] = 0
				continue
			}
		}

		// seatNo is the key, cards are value
		playerCards := handState.PlayersCards[uint32(player.SeatNo)]
		dealCards := HandDealCards{SeatNo: uint32(player.SeatNo)}

		tmpGameToken := uint64(0)
		cards, maskedCards := g.MaskCards(playerCards, tmpGameToken)
		playersCards[player.SeatNo] = fmt.Sprintf("%d", maskedCards)
		dealCards.Cards = fmt.Sprintf("%d", maskedCards)
		dealCards.CardsStr = poker.CardsToString(cards)
		dealCards.CardsInt = poker.ByteCardsToUint32Cards(playerCards)

		//messageData, _ := proto.Marshal(&message)
		handMessage := HandMessage{
			PlayerId:  player.PlayerId,
			MessageId: g.generateMsgID("CARDS", handState.HandNum, handState.CurrentState, player.PlayerId, "", handState.CurrentActionNum),
			Messages: []*HandMessageItem{
				{
					MessageType: HandDeal,
					Content:     &HandMessageItem_DealCards{DealCards: &dealCards},
				},
			},
		}

		g.sendHandMessageToPlayer(&handMessage, player.PlayerId)

		crashtest.Hit(g.gameCode, crashtest.CrashPoint_DEAL_4, 0)
	}

	// print next action
	if handState.NextSeatAction == nil {
		g.logger.Trace().
			Uint32(logging.HandNumKey, handState.HandNum).
			Msg(fmt.Sprintf("Next action is nil"))
	} else {
		g.logger.Trace().
			Uint32(logging.HandNumKey, handState.HandNum).
			Msg(fmt.Sprintf("Next action: %s", handState.NextSeatAction.PrettyPrint(handState, g.PlayersInSeats)))
	}

	allMsgItems := make([]*HandMessageItem, 0)
	if handState.BombPot {
		bombPotMessage := &HandMessageItem{
			MessageType: HandBombPot,
		}
		allMsgItems = append(allMsgItems, bombPotMessage)
		messages, err := g.gotoFlop(handState)
		if err == nil {
			allMsgItems = append(allMsgItems, messages...)
		}
	}
	var handMsg HandMessage
	var nextFlowState FlowState
	if handState.allActionComplete() {
		msgItems, err := g.allPlayersAllIn(handState)
		if err != nil {
			return err
		}
		nextFlowState = FlowState_MOVE_TO_NEXT_HAND
		allMsgItems = append(allMsgItems, msgItems...)
		handMsg = HandMessage{
			HandNum:    handState.HandNum,
			HandStatus: handState.CurrentState,
			MessageId:  g.generateMsgID("NO_ACTION_THIS_HAND", handState.HandNum, handState.CurrentState, 0, "", handState.CurrentActionNum),
			Messages:   allMsgItems,
		}
	} else {
		if handState.NextSeatAction == nil {
			bytes, err := protojson.Marshal(handState)
			var errMsg string
			if err != nil {
				errMsg = "NextSeatAction is nil when dealing new hand"
			} else {
				errMsg = fmt.Sprintf("NextSeatAction is nil when dealing new hand. HandState: %s", string(bytes))
			}
			g.logger.Panic().
				Uint32(logging.HandNumKey, handState.HandNum).
				Uint32(logging.ButtonPosKey, handState.ButtonPos).
				Uint32(logging.SbPosKey, handState.SmallBlindPos).
				Uint32(logging.BbPosKey, handState.BigBlindPos).
				Float64(logging.SbAmtKey, handState.SmallBlind).
				Float64(logging.BbAmtKey, handState.BigBlind).
				Msg(errMsg)
		}

		msgItems, err := g.moveToNextAction(handState)
		if err != nil {
			return err
		}
		allMsgItems = append(allMsgItems, msgItems...)
		handMsg = HandMessage{
			HandNum:    handState.HandNum,
			HandStatus: handState.CurrentState,
			MessageId:  g.generateMsgID("INITIAL_ACTION", handState.HandNum, handState.CurrentState, 0, "", handState.CurrentActionNum),
			Messages:   allMsgItems,
		}
		nextFlowState = FlowState_WAIT_FOR_NEXT_ACTION
	}

	g.broadcastHandMessage(&handMsg)

	crashtest.Hit(g.gameCode, crashtest.CrashPoint_DEAL_5, 0)

	err = g.saveHandState(handState, nextFlowState)
	if err != nil {
		msg := fmt.Sprintf("Could not save hand state after dealing")
		g.logger.Error().
			Uint32(logging.HandNumKey, handState.GetHandNum()).
			Err(err).
			Msgf(msg)
		return errors.Wrap(err, msg)
	}
	crashtest.Hit(g.gameCode, crashtest.CrashPoint_DEAL_6, 0)
	g.handleHandEnded(handState, handState.TotalResultPauseTime, allMsgItems)
	return nil
}

func (g *Game) dealTournamentHand(handInfo *rpc.HandInfo) error {
	// construct old new hand info and continue
	newHandInfo := NewHandInfo{
		GameID:          handInfo.GameId,
		GameCode:        handInfo.GameCode,
		GameType:        GameType(handInfo.HandDetails.GameType),
		MaxPlayers:      handInfo.HandDetails.MaxPlayers,
		SmallBlind:      handInfo.HandDetails.Sb,
		BigBlind:        handInfo.HandDetails.Bb,
		Ante:            handInfo.HandDetails.Ante,
		ButtonPos:       handInfo.HandDetails.ButtonPos,
		HandNum:         handInfo.HandDetails.HandNum,
		ActionTime:      handInfo.HandDetails.ActionTime,
		ChipUnit:        ChipUnit_DOLLAR,
		GameStatus:      GameStatus(GameStatus_ACTIVE),
		TableStatus:     TableStatus(TableStatus_GAME_RUNNING),
		SbPos:           handInfo.HandDetails.SbPos,
		BbPos:           handInfo.HandDetails.BbPos,
		ResultPauseTime: handInfo.HandDetails.ResultPauseTime,
		Tournament:      true,
	}

	newHandInfo.PlayersInSeats = make([]SeatPlayer, len(handInfo.Seats))
	for i, seat := range handInfo.Seats {
		playerInSeat := SeatPlayer{
			SeatNo:     seat.SeatNo,
			PlayerID:   seat.PlayerId,
			PlayerUUID: seat.PlayerUuid,
			Name:       seat.Name,
			Stack:      seat.Stack,
			Inhand:     seat.Inhand,
			OpenSeat:   seat.OpenSeat,
			Status:     PlayerStatus(PlayerStatus_PLAYING),
		}
		newHandInfo.PlayersInSeats[i] = playerInSeat
	}
	// deal a new hand
	g.dealNewHand(&newHandInfo)
	return nil
}

func (g *Game) generateMsgID(prefix string, handNum uint32, handStatus HandStatus, playerID uint64, originalMsgID string, currentActionNum uint32) string {
	return fmt.Sprintf("%s:%d:%s:%d:%s:%d", prefix, handNum, handStatus, playerID, originalMsgID, currentActionNum)
}

func (g *Game) GenerateMsgID(prefix string, handNum uint32, handStatus HandStatus, playerID uint64, originalMsgID string, currentActionNum uint32) string {
	return g.generateMsgID(prefix, handNum, handStatus, playerID, originalMsgID, currentActionNum)
}

func (g *Game) saveHandState(handState *HandState, nextFlowState FlowState) error {
	if handState == nil {
		// We should never call it with nil. Panic for stack trace.
		panic("saveHandState called with nil hand state")
	}

	handState.FlowState = nextFlowState

	err := g.manager.handStatePersist.Save(
		g.gameCode,
		handState)
	return err
}

func (g *Game) removeHandState() error {
	err := g.manager.handStatePersist.Remove(g.gameCode)
	return err
}

func (g *Game) loadHandState() (*HandState, error) {
	handState, err := g.manager.handStatePersist.Load(g.gameCode)
	return handState, err
}

func (g *Game) broadcastHandMessage(message *HandMessage) {
	message.GameCode = g.gameCode
	var outMsg *HandMessage = &HandMessage{}
	err := g.convertToClientUnits(message, outMsg)
	if err != nil {
		msg := "Could not convert to client units"
		g.logger.Error().Err(err).Msg(msg)
		panic(msg)
	}

	if *g.messageSender != nil {
		(*g.messageSender).BroadcastHandMessage(outMsg)
	} else {
		b, _ := proto.Marshal(outMsg)
		for _, player := range g.scriptTestPlayers {
			player.chHand <- b
		}
	}

	for _, msgItem := range outMsg.GetMessages() {
		msgType := msgItem.GetMessageType()
		switch msgType {
		case HandNewHand:
			util.Metrics.NewHandMsgSent()
		case HandEnded:
			util.Metrics.HandEndMsgSent()
		}
	}
}

func (g *Game) broadcastGameMessage(message *GameMessage) {
	message.GameCode = g.gameCode
	if *g.messageSender != nil {
		(*g.messageSender).BroadcastGameMessage(message, false)
	} else {
		b, _ := proto.Marshal(message)
		for _, player := range g.scriptTestPlayers {
			player.chGame <- b
		}
	}
}

func (g *Game) QueueGameMessage(message *GameMessage) {
	b, _ := proto.Marshal(message)
	g.chGame <- b
}

func (g *Game) sendGameMessageToPlayer(message *GameMessage) {
	message.GameCode = g.gameCode
	if *g.messageSender != nil {
		(*g.messageSender).SendGameMessageToPlayer(message, message.PlayerId)
	}
}

func (g *Game) QueueHandMessage(message *HandMessage) {
	message.GameCode = g.gameCode
	b, _ := proto.Marshal(message)
	g.chHand <- b
}

func (g *Game) sendHandMessageToPlayer(message *HandMessage, playerID uint64) {
	message.GameCode = g.gameCode
	var outMsg *HandMessage = &HandMessage{}
	err := g.convertToClientUnits(message, outMsg)
	if err != nil {
		msg := "Could not convert to client units"
		g.logger.Error().Err(err).Msg(msg)
		panic(msg)
	}

	if *g.messageSender != nil {
		(*g.messageSender).SendHandMessageToPlayer(outMsg, playerID)
	} else {
		player := g.scriptTestPlayers[playerID]
		if player == nil {
			return
		}
		b, _ := proto.Marshal(outMsg)
		player.chHand <- b
	}
}

func (g *Game) sendTournamentMessageToPlayer(message *HandMessage, tournamentID uint32, playerID uint64) {
	message.GameCode = g.gameCode
	var outMsg *HandMessage = &HandMessage{}
	err := g.convertToClientUnits(message, outMsg)
	if err != nil {
		msg := "Could not convert to client units"
		g.logger.Error().Err(err).Msg(msg)
		panic(msg)
	}

	if *g.messageSender != nil {
		(*g.messageSender).SendHandMessageToTournamentPlayer(outMsg, tournamentID, playerID)
	} else {
		player := g.scriptTestPlayers[playerID]
		if player == nil {
			return
		}
		b, _ := proto.Marshal(outMsg)
		player.chHand <- b
	}
}

func (g *Game) HandlePlayerMovedTable(gameCode string, tournamentID uint32, oldTableNo uint32, newTableNo uint32, newSeatNo uint32, playerID uint64, gameInfo string) error {
	playerMovedTable := PlayerMovedTable{
		TournamentId:   tournamentID,
		OldTableNo:     oldTableNo,
		NewTableNo:     newTableNo,
		NewTableSeatNo: newSeatNo,
		GameCode:       gameCode,
		PlayerId:       playerID,
		GameInfo:       gameInfo,
	}
	tableMovedMsg := &HandMessageItem{
		MessageType: HandPlayerMovedTable,
		Content:     &HandMessageItem_PlayerMovedTable{PlayerMovedTable: &playerMovedTable},
	}

	handState, err := g.loadHandState()
	if err != nil || handState == nil ||
		handState.HandNum == 0 ||
		handState.CurrentState == HandStatus_HAND_CLOSED {
		currentHandState := CurrentHandState{
			HandNum: 0,
		}
		handStateMsg := &HandMessageItem{
			MessageType: HandQueryCurrentHand,
			Content:     &HandMessageItem_CurrentHandState{CurrentHandState: &currentHandState},
		}
		serverMsg := &HandMessage{
			PlayerId:  playerID,
			HandNum:   0,
			MessageId: "",
			Messages:  []*HandMessageItem{tableMovedMsg, handStateMsg},
		}
		g.sendTournamentMessageToPlayer(serverMsg, tournamentID, playerID)
		return nil
	}

	currentHandState, err := g.GetCurrentHandState(handState, playerID)
	if err != nil {
		return err
	}

	handStateMsg := &HandMessageItem{
		MessageType: HandQueryCurrentHand,
		Content:     &HandMessageItem_CurrentHandState{CurrentHandState: currentHandState},
	}

	serverMsg := &HandMessage{
		PlayerId:   playerID,
		HandNum:    handState.HandNum,
		HandStatus: handState.CurrentState,
		MessageId: g.GenerateMsgID("CURRENT_HAND", handState.HandNum,
			handState.CurrentState, playerID, "", handState.CurrentActionNum),
		Messages: []*HandMessageItem{tableMovedMsg, handStateMsg},
	}
	g.sendTournamentMessageToPlayer(serverMsg, tournamentID, playerID)
	return nil
}

func (g *Game) HandleQueryCurrentHand(playerID uint64, messageID string) error {
	handState, err := g.loadHandState()
	if err != nil || handState == nil ||
		handState.HandNum == 0 ||
		handState.CurrentState == HandStatus_HAND_CLOSED {
		currentHandState := CurrentHandState{
			HandNum: 0,
		}
		handStateMsg := &HandMessageItem{
			MessageType: HandQueryCurrentHand,
			Content:     &HandMessageItem_CurrentHandState{CurrentHandState: &currentHandState},
		}
		serverMsg := &HandMessage{
			PlayerId:  playerID,
			HandNum:   0,
			MessageId: messageID,
			Messages:  []*HandMessageItem{handStateMsg},
		}
		g.sendHandMessageToPlayer(serverMsg, playerID)
		return nil
	}

	currentHandState, err := g.GetCurrentHandState(handState, playerID)
	if err != nil {
		return err
	}

	handStateMsg := &HandMessageItem{
		MessageType: HandQueryCurrentHand,
		Content:     &HandMessageItem_CurrentHandState{CurrentHandState: currentHandState},
	}

	serverMsg := &HandMessage{
		PlayerId:   playerID,
		HandNum:    handState.HandNum,
		HandStatus: handState.CurrentState,
		MessageId: g.GenerateMsgID("CURRENT_HAND", handState.HandNum,
			handState.CurrentState, playerID, messageID, handState.CurrentActionNum),
		Messages: []*HandMessageItem{handStateMsg},
	}
	g.sendHandMessageToPlayer(serverMsg, playerID)
	return nil
}

func (g *Game) GetCurrentHandState(handState *HandState, playerID uint64) (*CurrentHandState, error) {
	boardCards := make([]uint32, len(handState.BoardCards))
	for i, card := range handState.BoardCards {
		boardCards[i] = uint32(card)
	}

	var board2Cards []uint32
	if len(handState.Boards) >= 2 {
		board2Cards = make([]uint32, len(handState.Boards[1].Cards))
		for i, card := range handState.Boards[1].Cards {
			board2Cards[i] = uint32(card)
		}
	}

	pots := make([]float64, 0)
	for _, pot := range handState.Pots {
		pots = append(pots, pot.Pot)
	}
	var currentPot float64 = 0
	if len(pots) > 0 {
		currentPot = pots[len(pots)-1]
	}
	bettingInProgress := handState.CurrentState == HandStatus_PREFLOP ||
		handState.CurrentState == HandStatus_FLOP ||
		handState.CurrentState == HandStatus_TURN ||
		handState.CurrentState == HandStatus_RIVER
	if bettingInProgress {
		currentRoundState, ok := handState.RoundState[uint32(handState.CurrentState)]
		if !ok || currentRoundState == nil {
			b, err := json.Marshal(handState)
			if err != nil {
				return nil, fmt.Errorf("unable to find current round state. currentRoundState: %+v. handState.CurrentState: %d handState.RoundState: %+v", currentRoundState, handState.CurrentState, handState.RoundState)
			}
			return nil, fmt.Errorf("unable to find current round state. handState: %s", string(b))
		}
		currentBettingRound := currentRoundState.Betting
		for _, bet := range currentBettingRound.SeatBet {
			currentPot = currentPot + bet
		}
	}

	var boardCardsOut []uint32
	switch handState.CurrentState {
	case HandStatus_FLOP:
		boardCardsOut = boardCards[:3]
	case HandStatus_TURN:
		boardCardsOut = boardCards[:4]

	case HandStatus_RIVER:
		boardCardsOut = boardCards
	case HandStatus_RESULT:
		boardCardsOut = boardCards
	case HandStatus_SHOW_DOWN:
		boardCardsOut = boardCards

	default:
		boardCardsOut = make([]uint32, 0)
	}

	var board2CardsOut []uint32
	if len(board2Cards) >= 5 {
		switch handState.CurrentState {
		case HandStatus_FLOP:
			board2CardsOut = board2Cards[:3]
		case HandStatus_TURN:
			board2CardsOut = board2Cards[:4]

		case HandStatus_RIVER:
			board2CardsOut = board2Cards
		case HandStatus_RESULT:
			board2CardsOut = board2Cards
		case HandStatus_SHOW_DOWN:
			board2CardsOut = boardCards

		default:
			board2CardsOut = make([]uint32, 0)
		}
	}
	cardsStr := poker.CardsToString(boardCardsOut)
	cards2Str := poker.CardsToString(board2CardsOut)

	currentHandState := CurrentHandState{
		HandNum:       handState.HandNum,
		GameType:      handState.GameType,
		CurrentRound:  handState.CurrentState,
		BoardCards:    boardCardsOut,
		BoardCards_2:  board2CardsOut,
		Cards2Str:     cards2Str,
		CardsStr:      cardsStr,
		Pots:          pots,
		PotUpdates:    currentPot,
		ButtonPos:     handState.ButtonPos,
		SmallBlindPos: handState.SmallBlindPos,
		BigBlindPos:   handState.BigBlindPos,
		SmallBlind:    handState.SmallBlind,
		BigBlind:      handState.BigBlind,
		NoCards:       g.NumCards(handState.GameType),
	}
	currentHandState.PlayersActed = make(map[uint32]*PlayerActRound)

	var playerSeatNo uint32
	for seatNo, player := range handState.PlayersInSeats {
		if player.PlayerId == playerID {
			playerSeatNo = uint32(seatNo)
			break
		}
	}

	for seatNo, action := range handState.PlayersActed {
		if action.Action == ACTION_EMPTY_SEAT {
			continue
		}
		currentHandState.PlayersActed[uint32(seatNo)] = action
	}

	if playerSeatNo != 0 && len(g.PlayersInSeats) > 0 {
		player := g.PlayersInSeats[playerSeatNo]
		_, maskedCards := g.MaskCards(handState.GetPlayersCards()[playerSeatNo], player.GameTokenInt)
		currentHandState.PlayerCards = fmt.Sprintf("%d", maskedCards)
		currentHandState.PlayerSeatNo = playerSeatNo
	}

	if bettingInProgress && handState.NextSeatAction != nil {
		remainingActionTime := g.GetRemainingActionTime()
		if playerSeatNo != handState.NextSeatAction.SeatNo {
			if remainingActionTime > g.timerCushionSec {
				remainingActionTime = remainingActionTime - g.timerCushionSec
			} else {
				remainingActionTime = 0
			}
		}
		currentHandState.NextSeatToAct = handState.NextSeatAction.SeatNo
		currentHandState.RemainingActionTime = g.GetRemainingActionTime()
		currentHandState.NextSeatAction = handState.NextSeatAction
	}
	currentHandState.PlayersStack = make(map[uint64]float64)
	for seatNo, player := range handState.PlayersInSeats {
		if player.OpenSeat {
			continue
		}
		if player.PlayerId == 0 {
			continue
		}
		currentHandState.PlayersStack[uint64(seatNo)] = player.Stack
	}

	return &currentHandState, nil
}

func (g *Game) HandleAliveMessage(message *ClientAliveMessage) {
	g.networkCheck.ClientAlive(&networkcheck.AliveMsg{
		PlayerID: message.PlayerId,
	})
}

func (g *Game) addScriptTestPlayer(player *Player, buyIn float64, postBlind bool) error {
	g.lock.Lock()
	defer g.lock.Unlock()
	g.scriptTestPlayers[player.PlayerID] = player
	inHand := true
	if player.SeatNo == 0 {
		inHand = false
	}
	// add the player to playerSeatInfos
	g.PlayersInSeats[int(player.SeatNo)] = SeatPlayer{
		Name:        player.PlayerName,
		PlayerID:    player.PlayerID,
		PlayerUUID:  fmt.Sprintf("%d", player.PlayerID),
		Status:      PlayerStatus_PLAYING,
		Stack:       buyIn,
		OpenSeat:    false,
		Inhand:      inHand,
		SeatNo:      player.SeatNo,
		PostedBlind: postBlind,
		RunItTwice:  player.RunItTwice,
	}
	return nil
}

func (g *Game) resetBlinds() {
	playerInSeats := make([]SeatPlayer, 0)
	for _, player := range g.PlayersInSeats {
		player.PostedBlind = false
		playerInSeats = append(playerInSeats, player)
	}
	g.PlayersInSeats = playerInSeats
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

func (g *Game) getGameInfoOld(apiServerURL string, gameCode string, retryDelay uint32) (*TestGameConfig, error) {
	var gameConfig TestGameConfig
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
			g.logger.Error().Msgf("Connection to API server is lost. Waiting for %.3f seconds before retrying", float32(retryDelay)/1000)
			time.Sleep(time.Duration(retryDelay) * time.Millisecond)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			g.logger.Error().
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

func (g *Game) anyPendingUpdates(apiServerUrl string, gameID uint64, retryDelay uint32) (bool, error) {
	type pendingUpdates struct {
		PendingUpdates bool
	}
	var updates pendingUpdates
	url := fmt.Sprintf("%s/internal/any-pending-updates/gameId/%d", apiServerUrl, gameID)
	retry := true
	for retry {
		resp, err := http.Get(url)
		if resp == nil {
			g.logger.Error().Msgf("Connection to API server is lost. Waiting for %.3f seconds before retrying", float32(retryDelay)/1000)
			time.Sleep(time.Duration(retryDelay) * time.Millisecond)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			g.logger.Error().Msgf("Failed to get pending status. Error: %d", resp.StatusCode)
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
		return nil, errors.Wrapf(err, "Unable to get encryption key for player %d", playerID)
	}
	playerName := ""
	seatNo := 0
	for _, player := range g.PlayersInSeats {
		if player.PlayerID == playerID {
			playerName = player.Name
			seatNo = int(player.SeatNo)
			break
		}
	}
	encryptedData, err := encryption.EncryptWithUUIDStrKey(data, encryptionKey)
	if err != nil {
		return nil, errors.Wrapf(err, "Unable to encrypt message to player %d", playerID)
	}
	g.logger.Info().Msgf("ENCRYPT: [%s]: player: %s playerID: %d, seatNo: %d key: %s data: %s",
		g.gameCode, playerName, playerID, seatNo, encryptionKey, encryption.B64EncodeToString(encryptedData))
	return encryptedData, nil
}

func (g *Game) EncryptAndB64ForPlayer(data []byte, playerID uint64) (string, error) {
	encryptedData, err := g.EncryptForPlayer(data, playerID)
	if err != nil {
		return "", err
	}
	return encryption.B64EncodeToString(encryptedData), nil
}

func (g *Game) GetRemainingActionTime() uint32 {
	return g.actionTimer.GetRemainingSec()
}

func (g *Game) onClientConnLost(a networkcheck.Action) {
	playerIDs := []uint64{a.PlayerID}
	g.broadcastConnectivityLost(playerIDs)
}

func (g *Game) broadcastConnectivityLost(playerIDs []uint64) {
	if *g.messageSender == nil {
		return
	}

	gameMessage := GameMessage{
		MessageType: GamePlayerConnectivityLost,
		GameId:      g.gameID,
		GameCode:    g.gameCode,
		PlayerId:    0,
	}
	gameMessage.GameMessage = &GameMessage_NetworkConnectivity{
		NetworkConnectivity: &GameNetworkConnectivityMessage{
			PlayerIds: playerIDs,
		},
	}
	skipLog := !util.Env.ShouldDebugConnectivityCheck()
	(*g.messageSender).BroadcastGameMessage(&gameMessage, skipLog)
}

func (g *Game) onClientConnRestored(a networkcheck.Action) {
	playerIDs := []uint64{a.PlayerID}
	g.broadcastConnectivityRestored(playerIDs)
}

func (g *Game) broadcastConnectivityRestored(playerIDs []uint64) {
	if *g.messageSender == nil {
		return
	}

	gameMessage := GameMessage{
		MessageType: GamePlayerConnectivityRestored,
		GameId:      g.gameID,
		GameCode:    g.gameCode,
		PlayerId:    0,
	}
	gameMessage.GameMessage = &GameMessage_NetworkConnectivity{
		NetworkConnectivity: &GameNetworkConnectivityMessage{
			PlayerIds: playerIDs,
		},
	}
	skipLog := !util.Env.ShouldDebugConnectivityCheck()
	(*g.messageSender).BroadcastGameMessage(&gameMessage, skipLog)
}
