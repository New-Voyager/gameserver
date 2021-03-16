package player

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	jsoniter "github.com/json-iterator/go"
	"github.com/looplab/fsm"
	"github.com/machinebox/graphql"
	natsgo "github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/encoding/protojson"
	"voyager.com/botrunner/internal/game"
	"voyager.com/botrunner/internal/gql"
	"voyager.com/botrunner/internal/msgcheck"
	"voyager.com/botrunner/internal/poker"
	"voyager.com/botrunner/internal/util"
	"voyager.com/gamescript"
)

// Config holds the configuration for a bot object.
type Config struct {
	Name               string
	DeviceID           string
	Email              string
	Password           string
	IsHuman            bool
	IsObserver         bool
	IsHost             bool
	BotActionPauseTime uint32
	APIServerURL       string
	NatsURL            string
	GQLTimeoutSec      int
	Players            *gamescript.Players
	Script             *gamescript.Script
}

// BotPlayer represents a bot user.
type BotPlayer struct {
	logger          *zerolog.Logger
	config          Config
	gqlHelper       *gql.GQLHelper
	natsConn        *natsgo.Conn
	apiAuthToken    string
	clubCode        string
	clubID          uint64
	gameCode        string
	gameID          uint64
	PlayerID        uint64
	RewardsNameToId map[string]uint32

	// state of the bot
	sm *fsm.FSM

	// current status
	buyInAmount uint32
	havePair    bool
	pairCard    uint32
	balance     float32

	// used by host bot for tracking next deck
	deckHandNum uint32

	// For message acknowledgement
	lastMsgID   uint32
	lastMsgType string
	ackMaxWait  int

	// Message channels
	chGame chan *game.GameMessage
	chHand chan *game.HandMessage
	end    chan bool

	// Points to the most recent messages from the game server.
	lastGameMessage    *game.GameMessage
	lastHandMessage    *game.HandMessage
	playerStateMessage *game.GameTableStateMessage

	// GameInfo received from the api server.
	gameInfo *game.GameInfo

	// Seat change variables
	requestedSeatChange bool
	confirmSeatChange   bool

	// wait list variables
	inWaitList      bool
	confirmWaitlist bool

	// Nats subjects
	gameToAll string
	handToAll string
	handToMe  string
	meToHand  string

	// Nats subscription objects
	gameMsgSub       *natsgo.Subscription
	handMsgAllSub    *natsgo.Subscription
	handMsgPlayerSub *natsgo.Subscription

	game      *gameView
	handNum   uint32 // hand number
	seatNo    uint32
	observing bool // if a player is playing, then he is also an observer

	logPrefix string

	// Print nats messages for debugging.
	printGameMsg  bool
	printHandMsg  bool
	printStateMsg bool

	// Collect nats messages for testing.
	msgCollector *msgcheck.MsgCollector

	decision ScriptBasedDecision

	isSeated bool

	// Error msg if the bot is in an error state (BotState__ERROR).
	errorStateMsg string
}

// NewBotPlayer creates an instance of BotPlayer.
func NewBotPlayer(playerConfig Config, logger *zerolog.Logger, msgCollector *msgcheck.MsgCollector) (*BotPlayer, error) {
	nc, err := natsgo.Connect(playerConfig.NatsURL)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("Error connecting to NATS server [%s]", playerConfig.NatsURL))
	}

	var logPrefix string
	if playerConfig.IsHuman {
		logPrefix = fmt.Sprintf("Player [%s]", playerConfig.Name)
	} else {
		logPrefix = fmt.Sprintf("Bot [%s]", playerConfig.Name)
	}

	gqlClient := graphql.NewClient(util.GetGqlURL(playerConfig.APIServerURL))
	gqlHelper := gql.NewGQLHelper(gqlClient, uint32(playerConfig.GQLTimeoutSec), "")

	bp := BotPlayer{
		logger:          logger,
		config:          playerConfig,
		gqlHelper:       gqlHelper,
		natsConn:        nc,
		chGame:          make(chan *game.GameMessage, 10),
		chHand:          make(chan *game.HandMessage, 10),
		end:             make(chan bool),
		logPrefix:       logPrefix,
		printGameMsg:    util.Env.ShouldPrintGameMsg(),
		printHandMsg:    util.Env.ShouldPrintHandMsg(),
		printStateMsg:   util.Env.ShouldPrintStateMsg(),
		msgCollector:    msgCollector,
		RewardsNameToId: make(map[string]uint32),
		ackMaxWait:      30,
	}

	bp.sm = fsm.NewFSM(
		BotState__NOT_IN_GAME,
		fsm.Events{
			{
				Name: BotEvent__SUBSCRIBE,
				Src:  []string{BotState__NOT_IN_GAME},
				Dst:  BotState__OBSERVING,
			},
			{
				Name: BotEvent__REQUEST_SIT,
				Src:  []string{BotState__OBSERVING},
				Dst:  BotState__JOINING,
			},
			{
				Name: BotEvent__REJOIN,
				Src:  []string{BotState__OBSERVING},
				Dst:  BotState__REJOINING,
			},
			{
				Name: BotEvent__SUCCEED_BUYIN,
				Src:  []string{BotState__JOINING, BotState__REJOINING},
				Dst:  BotState__WAITING_FOR_MY_TURN,
			},
			{
				Name: BotEvent__RECEIVE_YOUR_ACTION,
				Src:  []string{BotState__WAITING_FOR_MY_TURN},
				Dst:  BotState__MY_TURN,
			},
			{
				Name: BotEvent__SEND_MY_ACTION,
				Src:  []string{BotState__MY_TURN, BotState__REJOINING},
				Dst:  BotState__ACTED_WAITING_FOR_ACK,
			},
			{
				Name: BotEvent__RECEIVE_ACK,
				Src:  []string{BotState__ACTED_WAITING_FOR_ACK},
				Dst:  BotState__WAITING_FOR_MY_TURN,
			},
			{
				Name: BotEvent__ACTION_TIMEDOUT,
				Src:  []string{BotState__ACTED_WAITING_FOR_ACK},
				Dst:  BotState__WAITING_FOR_MY_TURN,
			},
		},
		fsm.Callbacks{
			"enter_state": func(e *fsm.Event) { bp.enterState(e) },
		},
	)
	go bp.messageLoop()
	return &bp, nil
}

func (bp *BotPlayer) enterState(e *fsm.Event) {
	if bp.printStateMsg {
		bp.logger.Info().Msgf("%s: [%s] ===> [%s]", bp.logPrefix, e.Src, e.Dst)
	}
}

func (bp *BotPlayer) event(event string) {
	err := bp.sm.Event(event)
	if err != nil {
		bp.logger.Error().Msgf("%s: Error from state machine: %s", bp.logPrefix, err.Error())
	}
}

func (bp *BotPlayer) queueGameMsg(msg *natsgo.Msg) {
	if bp.printGameMsg {
		bp.logger.Info().Msg(fmt.Sprintf("%s: Received game message %s", bp.logPrefix, string(msg.Data)))
	}

	var message game.GameMessage
	err := protojson.Unmarshal(msg.Data, &message)
	if err != nil {
		bp.logger.Error().Msgf("%s: Error [%s] while unmarshalling protobuf message [%s]", bp.logPrefix, err, string(msg.Data))
		return
	}

	bp.collectGameMsg(&message, msg.Data)

	bp.chGame <- &message
}

func (bp *BotPlayer) queueHandMsg(msg *natsgo.Msg) {
	if bp.printHandMsg {
		bp.logger.Info().Msg(fmt.Sprintf("%s: Subject: %s Received hand message %s", bp.logPrefix, msg.Subject, string(msg.Data)))
	}

	var message game.HandMessage
	err := protojson.Unmarshal(msg.Data, &message)
	if err != nil {
		bp.logger.Error().Msgf("%s: Error [%s] while unmarshalling protobuf message [%s]", bp.logPrefix, err, string(msg.Data))
		return
	}

	bp.collectHandMsg(&message, msg.Data)

	bp.chHand <- &message
}

func (bp *BotPlayer) collectGameMsg(msg *game.GameMessage, rawMsg []byte) {
	if bp.msgCollector == nil {
		return
	}

	bp.msgCollector.AddGameMsg(bp.config.Name, msg, rawMsg)
}

func (bp *BotPlayer) collectHandMsg(msg *game.HandMessage, rawMsg []byte) {
	if bp.msgCollector == nil {
		return
	}

	bp.msgCollector.AddHandMsg(bp.config.Name, msg, rawMsg)
}

func (bp *BotPlayer) messageLoop() {
	for {
		select {
		case <-bp.end:
			return
		case message := <-bp.chGame:
			bp.handleGameMessage(message)
		case message := <-bp.chHand:
			bp.handleHandMessage(message)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (bp *BotPlayer) updateLogPrefix() {
	if bp.config.IsHuman {
		bp.logPrefix = fmt.Sprintf("Player [%s:%d:%d]", bp.config.Name, bp.PlayerID, bp.seatNo)
	} else {
		bp.logPrefix = fmt.Sprintf("Bot [%s:%d:%d]", bp.config.Name, bp.PlayerID, bp.seatNo)
	}
}

func (bp *BotPlayer) handleHandMessage(message *game.HandMessage) {
	if bp.IsErrorState() {
		bp.logger.Info().Msgf("%s: Bot is in error state. Ignoring hand message.", bp.logPrefix)
		return
	}

	if message.PlayerId != 0 && message.PlayerId != bp.PlayerID {
		// drop this message
		// this message was targeted for another player
		return
	}

	bp.lastHandMessage = message

	switch message.MessageType {
	case game.HandDeal:
		deal := message.GetDealCards()
		maskedCards := deal.Cards
		c, _ := strconv.ParseInt(maskedCards, 10, 64)
		cardBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(cardBytes, uint64(c))
		cards := make([]uint32, 0)
		i := 0
		for _, card := range cardBytes {
			if card == 0 {
				break
			}
			cards = append(cards, uint32(card))
			i++
		}
		bp.pairCard = 0
		bp.havePair = false
		for i, card := range cards {
			cardValue := int(card / 16)
			for j, checkChard := range cards {
				if i != j {
					checkCardValue := int(checkChard / 16)
					if cardValue == checkCardValue {
						bp.havePair = true
						bp.pairCard = card
						break
					}
				}
			}
		}
		bp.logger.Info().Msgf("%s: Received cards: %s (%+v)", bp.logPrefix, poker.CardsToString(cards), cards)

	case game.HandNewHand:
		/* MessageType: NEW_HAND */
		bp.game.table.handStatus = message.GetHandStatus()
		newHand := message.GetNewHand()
		bp.game.table.buttonPos = newHand.GetButtonPos()
		bp.game.table.sbPos = newHand.GetSbPos()
		bp.game.table.bbPos = newHand.GetBbPos()
		bp.game.table.nextActionSeat = newHand.GetNextActionSeat()
		bp.game.table.playersActed = make(map[uint32]*game.PlayerActRound)
		bp.handNum = message.HandNum
		if bp.IsHost() {
			bp.logger.Info().Msgf("A new hand is started. Hand Num: %d", message.HandNum)
			if !bp.config.Script.AutoPlay {
				if int(message.HandNum) == len(bp.config.Script.Hands) {
					bp.logger.Info().Msgf("%s: Last hand: %d Game will be ended in next hand", bp.logPrefix, message.HandNum)

					// The host bot should schedule to end the game after this hand is over.
					go func() {
						// API server caches game status for 1 second (typeorm cache).
						// Since bots are fast and change the game status more than once
						// (configure game -> start game -> request to end game) within a second,
						// the endGame request is acting on stale game status (thinks it's not active yet)
						// ending the game immediately instead of waiting for the hand that just started.
						// Give some delay for the cache to clear and the game to be recognized as active.
						time.Sleep(1 * time.Second)
						bp.RequestEndGame(bp.gameCode)
					}()
				} else {
					// setup next hand
					bp.setupNextHand()
				}
			}
		}

		if bp.IsHost() {
			bp.pauseGameIfNeeded()
		}

		// setup seat change requests
		bp.setupSeatChange()

		// setup waitlist requests
		bp.setupWaitList()

		// process any leave game requests
		// the player will after this hand
		bp.setupLeaveGame()

		bp.storeGameInfo()

	case game.HandFlop:
		/* MessageType: FLOP */
		bp.game.table.handStatus = message.GetHandStatus()
		bp.game.table.flopCards = message.GetFlop().GetBoard()
		if bp.IsHuman() || bp.IsObserver() {
			bp.logger.Info().Msgf("%s: Flop cards shown: %s", bp.logPrefix, message.GetFlop().GetCardsStr())
		}
		bp.verifyBoard()
		bp.game.table.playersActed = make(map[uint32]*game.PlayerActRound)

	case game.HandTurn:
		/* MessageType: TURN */
		bp.game.table.handStatus = message.GetHandStatus()
		bp.game.table.turnCards = message.GetTurn().GetBoard()
		if bp.IsHuman() || bp.IsObserver() {
			bp.logger.Info().Msgf("%s: Turn cards shown: %s", bp.logPrefix, message.GetTurn().GetCardsStr())
		}
		bp.verifyBoard()
		bp.game.table.playersActed = make(map[uint32]*game.PlayerActRound)

	case game.HandRiver:
		/* MessageType: RIVER */
		bp.game.table.handStatus = message.GetHandStatus()
		bp.game.table.riverCards = message.GetRiver().GetBoard()
		if bp.IsHuman() || bp.IsObserver() {
			bp.logger.Info().Msgf("%s: River cards shown: %s", bp.logPrefix, message.GetRiver().GetCardsStr())
		}
		bp.verifyBoard()
		bp.game.table.playersActed = make(map[uint32]*game.PlayerActRound)

	case game.HandPlayerAction:
		/* MessageType: YOUR_ACTION */
		bp.event(BotEvent__RECEIVE_YOUR_ACTION)
		bp.game.table.handStatus = message.GetHandStatus()
		seatAction := message.GetSeatAction()
		seatNo := seatAction.GetSeatNo()
		if bp.IsObserver() && bp.config.Script.IsSeatHuman(seatNo) {
			bp.logger.Info().Msgf("%s: Waiting on seat %d (%s/human) to act.", bp.logPrefix, seatNo, bp.getPlayerNameBySeatNo(seatNo))
		}
		if seatNo != bp.seatNo {
			// It's not my turn.
			break
		}
		bp.game.table.handNum = message.HandNum
		bp.act(seatAction)

	case game.HandPlayerActed:
		/* MessageType: PLAYER_ACTED */
		bp.game.table.handNum = message.HandNum
		playerActed := message.GetPlayerActed()
		seatNo := playerActed.GetSeatNo()
		action := playerActed.GetAction()
		amount := playerActed.GetAmount()
		isTimedOut := playerActed.GetTimedOut()
		var timedout string
		if isTimedOut {
			timedout = " (TIMED OUT)"
		}
		actedPlayerName := bp.getPlayerNameBySeatNo(seatNo)
		bp.rememberPlayerAction(seatNo, action, amount, isTimedOut, bp.game.table.handStatus)
		if bp.IsObserver() {
			actedPlayerType := "bot"
			if bp.config.Script.IsSeatHuman(seatNo) {
				actedPlayerType = "human"
			}
			bp.logger.Info().Msgf("%s: Seat %d (%s/%s) acted [%s %f] Stage:%s.", bp.logPrefix, seatNo, actedPlayerName, actedPlayerType, action, amount, bp.game.table.handStatus)
		}
		if bp.IsHuman() && seatNo != bp.seatNo {
			// I'm a human and I see another player acted.
			bp.logger.Info().Msgf("%s: Seat %d: %s %f%s", bp.logPrefix, seatNo, action, amount, timedout)
		}
		if seatNo == bp.seatNo && isTimedOut {
			bp.event(BotEvent__ACTION_TIMEDOUT)
		}

	case game.HandMsgAck:
		/* MessageType: MSG_ACK */
		msgType := message.GetMsgAck().GetMessageType()
		msgID := message.GetMsgAck().GetMessageId()
		msg := fmt.Sprintf("%s: Received unexpected ack msg - %s:%d BotState: %s, CurrentMsgType: %s, CurrentMsgID: %d", bp.logPrefix, msgType, msgID, bp.sm.Current(), bp.lastMsgType, bp.lastMsgID)
		if bp.sm.Current() != BotState__ACTED_WAITING_FOR_ACK {
			bp.logger.Info().Msg(msg)
			return
		}
		if msgType != bp.lastMsgType {
			bp.logger.Info().Msg(msg)
			return
		}
		if msgID != bp.lastMsgID {
			bp.logger.Info().Msg(msg)
			return
		}
		bp.event(BotEvent__RECEIVE_ACK)

	case game.HandResultMessage:
		/* MessageType: RESULT */
		bp.game.table.handStatus = message.GetHandStatus()
		bp.game.table.handResult = message.GetHandResult()
		if bp.IsObserver() {
			bp.PrintHandResult()
		}

		result := bp.game.table.handResult
		for seatNo, player := range result.Players {
			if seatNo == bp.seatNo {
				if player.Balance.After == 0.0 {
					// reload chips
					bp.reload()
				}
				break
			}
		}

	case game.HandEnded:
		bp.logger.Info().Msgf("%s: IsHost: %v handNum: %d ended", bp.logPrefix, bp.IsHost(), message.HandNum)
		if bp.IsHost() {
			// process post hand steps if specified
			bp.processPostHandSteps()
		}

	case game.HandQueryCurrentHand:
		currentState := message.GetCurrentHandState()
		bp.logger.Info().Msgf("%s: Received current hand state: %+v", bp.logPrefix, currentState)
		handStatus := currentState.GetCurrentRound()
		playersActed := currentState.GetPlayersActed()
		nextSeatAction := currentState.GetNextSeatAction()
		actionSeatNo := nextSeatAction.GetSeatNo()
		bp.game.table.handStatus = handStatus
		bp.game.table.nextActionSeat = actionSeatNo
		bp.game.table.playersActed = playersActed
		bp.game.table.handNum = message.HandNum

		if actionSeatNo != bp.seatNo {
			return
		}
		if bp.sm.Current() == BotState__REJOINING {
			// When you are rejoining the game you were playing, and the timer is on you,
			// you need to act based on the current hand state message instead of the
			// YOUR_ACTION message you already missed while you were out.
			bp.act(nextSeatAction)
		}
	}
}

func (bp *BotPlayer) verifyBoard() {
	var expectedBoard []string
	var currentBoard []uint32
	scriptCurrentHand := bp.config.Script.GetHand(bp.handNum)
	switch bp.game.table.handStatus {
	case game.HandStatus_FLOP:
		expectedBoard = scriptCurrentHand.Flop.Verify.Board
		currentBoard = bp.game.table.flopCards
	case game.HandStatus_TURN:
		expectedBoard = scriptCurrentHand.Turn.Verify.Board
		currentBoard = bp.game.table.turnCards
	case game.HandStatus_RIVER:
		expectedBoard = scriptCurrentHand.River.Verify.Board
		currentBoard = bp.game.table.riverCards
	}
	if len(expectedBoard) == 0 {
		// No verify in yaml.
		return
	}
	expectedBoardCards := make([]poker.Card, 0)
	currentBoardCards := make([]poker.Card, 0)
	for _, c := range expectedBoard {
		expectedBoardCards = append(expectedBoardCards, poker.NewCard(c))
	}
	for _, c := range currentBoard {
		currentBoardCards = append(currentBoardCards, poker.NewCardFromByte(uint8(c)))
	}
	match := true
	if len(expectedBoardCards) != len(currentBoardCards) {
		match = false
	}
	for i := 0; i < len(expectedBoardCards); i++ {
		if currentBoardCards[i] != expectedBoardCards[i] {
			match = false
			break
		}
	}

	if !match {
		bp.logger.Panic().Msgf("%s: Hand %d %s verify failed. Board does not match the expected. Current board: %v. Expected board: %v.", bp.logPrefix, bp.handNum, bp.game.table.handStatus, currentBoardCards, expectedBoardCards)
	}
}

func (bp *BotPlayer) SetClubCode(clubCode string) {
	bp.clubCode = clubCode
}

func (bp *BotPlayer) GetSeatNo() uint32 {
	return bp.seatNo
}

func (bp *BotPlayer) SetBalance(balance float32) {
	bp.balance = balance
}

// Register registers the bot to the Poker service as a user.
func (bp *BotPlayer) Register() error {
	bp.logger.Info().Msgf("%s: Registering as a user.", bp.logPrefix)

	playerUUID, err := bp.gqlHelper.CreatePlayer(bp.config.Name, bp.config.DeviceID, bp.config.Email, bp.config.Password, !bp.IsHuman())
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to create user", bp.logPrefix))
	}

	userJwt, err := bp.GetJWT(playerUUID, bp.config.DeviceID)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to get auth token", bp.logPrefix))
	}
	bp.apiAuthToken = fmt.Sprintf("jwt %s", userJwt)
	bp.gqlHelper.SetAuthToken(bp.apiAuthToken)

	playerID, err := bp.getPlayerID()
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to get the player ID", bp.logPrefix))
	}

	bp.PlayerID = playerID
	bp.logger.Info().Msgf("%s: Successfully registered as a user. Player UUID: [%s] Player ID: [%d].", bp.logPrefix, playerUUID, bp.PlayerID)
	return nil
}

// CreateClub creates a new club.
func (bp *BotPlayer) CreateClub(name string, description string) (string, error) {
	bp.logger.Info().Msgf("%s: Creating a new club [%s].", bp.logPrefix, name)

	clubCode, err := bp.gqlHelper.CreateClub(name, description)
	if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("%s: Unable to create a new club", bp.logPrefix))
	}

	bp.logger.Info().Msgf("%s: Successfully created a new club. Club Code: [%s]", bp.logPrefix, clubCode)
	bp.clubCode = clubCode
	return bp.clubCode, nil
}

// JoinClub joins the bot to a club.
func (bp *BotPlayer) JoinClub(clubCode string) error {
	bp.logger.Info().Msgf("%s: Applying to club [%s].", bp.logPrefix, clubCode)

	status, err := bp.gqlHelper.JoinClub(clubCode)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to apply to the club", bp.logPrefix))
	}
	bp.logger.Info().Msgf("%s: Successfully applied to club [%s]. Member Status: [%s]", bp.logPrefix, clubCode, status)

	bp.clubID, err = bp.GetClubID(clubCode)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to get the club ID", bp.logPrefix))
	}
	bp.clubCode = clubCode

	return nil
}

// GetClubMemberStatus returns the club member status of this bot.
func (bp *BotPlayer) GetClubMemberStatus(clubCode string) (int, error) {
	bp.logger.Info().Msgf("%s: Querying member status for club [%s].", bp.logPrefix, clubCode)

	resp, err := bp.gqlHelper.GetClubMemberStatus(clubCode)
	if err != nil {
		return -1, errors.Wrap(err, fmt.Sprintf("%s: Unable to get club member status", bp.logPrefix))
	}
	status := int(game.ClubMemberStatus_value[resp.Status])
	bp.logger.Info().Msgf("%s: Club member Status: [%s] StatusInt: %d", bp.logPrefix, resp.Status, status)
	return status, nil
}

// CreateClubReward creates a new club reward.
func (bp *BotPlayer) CreateClubReward(clubCode string, name string, rewardType string, scheduleType string, amount float32) (uint32, error) {
	bp.logger.Info().Msgf("%s: Creating a new club reward [%s].", bp.logPrefix, name)

	var rewardID uint32
	// if the reward already exists, use the existing reward
	clubRewards, err := bp.gqlHelper.GetClubRewards(clubCode)
	if clubRewards != nil {
		for _, reward := range *clubRewards {
			if reward.Name == name {
				rewardID = uint32(reward.Id)
				break
			}
		}
	}
	if rewardID == 0 {
		rewardID, err = bp.gqlHelper.CreateClubReward(clubCode, name, rewardType, scheduleType, amount)
		if err != nil {
			return 0, errors.Wrap(err, fmt.Sprintf("%s: Unable to create a new club", bp.logPrefix))
		}
	}
	bp.RewardsNameToId[name] = rewardID
	bp.logger.Info().Msgf("%s: Successfully created a new club reward. Club Code: [%s], rewardId: %d, name: %s, type: %s",
		bp.logPrefix, clubCode, rewardID, name, rewardType)
	return rewardID, nil
}

// GetClubID queries for the numeric club ID using the club code.
func (bp *BotPlayer) GetClubID(clubCode string) (uint64, error) {
	clubID, err := bp.gqlHelper.GetClubID(clubCode)
	if err != nil {
		return 0, errors.Wrap(err, fmt.Sprintf("%s: Unable to get club ID for club code [%s]", bp.logPrefix, clubCode))
	}
	return clubID, nil
}

// ApproveClubMembers checks and approves the pending club membership applications.
func (bp *BotPlayer) ApproveClubMembers() error {
	bp.logger.Info().Msgf("%s: Checking for pending application for the club [%s].", bp.logPrefix, bp.clubCode)
	if bp.clubCode == "" {
		return fmt.Errorf("%s: clubCode is missing", bp.logPrefix)
	}

	clubMembers, err := bp.gqlHelper.GetClubMembers(bp.clubCode)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to query for club members", bp.logPrefix))
	}

	// Now go through each member and approve all pending members.
	for _, member := range clubMembers {
		if member.Status != "PENDING" {
			continue
		}
		newStatus, err := bp.gqlHelper.ApproveClubMember(bp.clubCode, member.PlayerID)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("%s: Unable to approve member [%s] player ID [%s]", bp.logPrefix, member.Name, member.PlayerID))
		}
		if newStatus != "ACTIVE" {
			return fmt.Errorf("%s: Unable to approve member [%s] player ID [%s]. Member Status is [%s]", bp.logPrefix, member.Name, member.PlayerID, newStatus)
		}
		bp.logger.Info().Msgf("%s: Successfully approved [%s] for club [%s]. Member Status: [%s]", bp.logPrefix, member.Name, bp.clubCode, newStatus)
	}
	return nil
}

// CreateGame creates a new game.
func (bp *BotPlayer) CreateGame(gameOpt game.GameCreateOpt) (string, error) {
	bp.logger.Info().Msgf("%s: Creating a new game [%s].", bp.logPrefix, gameOpt.Title)

	gameCode, err := bp.gqlHelper.CreateGame(bp.clubCode, gameOpt)
	if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("%s: Unable to create new game", bp.logPrefix))
	}
	bp.logger.Info().Msgf("%s: Successfully created a new game. Game Code: [%s]", bp.logPrefix, gameCode)
	bp.gameCode = gameCode
	return bp.gameCode, nil
}

// Subscribe makes the bot subscribe to the game's nats subjects.
func (bp *BotPlayer) Subscribe(gameToAll string, handToAll string, handToPlayer string) error {
	if bp.gameMsgSub == nil || !bp.gameMsgSub.IsValid() {
		bp.logger.Info().Msgf("%s: Subscribing to %s to receive game messages sent to players/observers", bp.logPrefix, gameToAll)
		gameToAllSub, err := bp.natsConn.Subscribe(gameToAll, bp.queueGameMsg)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("%s: Unable to subscribe to the game message subject [%s]", bp.logPrefix, gameToAll))
		}
		bp.gameMsgSub = gameToAllSub
		bp.logger.Info().Msgf("%s: Successfully subscribed to %s.", bp.logPrefix, gameToAll)
	}

	if bp.handMsgAllSub == nil || !bp.handMsgAllSub.IsValid() {
		bp.logger.Info().Msgf("%s: Subscribing to %s to receive hand messages sent to players/observers", bp.logPrefix, handToAll)
		handToAllSub, err := bp.natsConn.Subscribe(handToAll, bp.queueHandMsg)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("%s: Unable to subscribe to the hand message subject [%s]", bp.logPrefix, handToAll))
		}
		bp.handMsgAllSub = handToAllSub
		bp.logger.Info().Msgf("%s: Successfully subscribed to %s.", bp.logPrefix, handToAll)
	}

	if bp.handMsgPlayerSub == nil || !bp.handMsgPlayerSub.IsValid() {
		bp.logger.Info().Msgf("%s: Subscribing to %s to receive hand messages sent to player: %s", bp.logPrefix, handToPlayer, bp.config.Name)
		handToPlayerSub, err := bp.natsConn.Subscribe(handToPlayer, bp.queueHandMsg)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("%s: Unable to subscribe to the hand message subject [%s]", bp.logPrefix, handToPlayer))
		}
		bp.handMsgPlayerSub = handToPlayerSub
		bp.logger.Info().Msgf("%s: Successfully subscribed to %s.", bp.logPrefix, handToPlayer)
	}

	bp.event(BotEvent__SUBSCRIBE)

	return nil
}

// unsubscribe makes the bot unsubscribe from the nats subjects.
func (bp *BotPlayer) unsubscribe() error {
	var errMsg string
	if bp.gameMsgSub != nil {
		err := bp.gameMsgSub.Unsubscribe()
		if err != nil {
			errMsg = fmt.Sprintf("Error [%s] while unsubscribing from subject [%s]", err, bp.gameMsgSub.Subject)
		}
	}
	if bp.handMsgAllSub != nil {
		err := bp.handMsgAllSub.Unsubscribe()
		if err != nil {
			errMsg = fmt.Sprintf("%s Error [%s] while unsubscribing from subject [%s]", errMsg, err, bp.handMsgAllSub.Subject)
		}
	}
	if bp.handMsgPlayerSub != nil {
		err := bp.handMsgPlayerSub.Unsubscribe()
		if err != nil {
			errMsg = fmt.Sprintf("%s Error [%s] while unsubscribing from subject [%s]", errMsg, err, bp.handMsgPlayerSub.Subject)
		}
	}
	if errMsg != "" {
		return fmt.Errorf(errMsg)
	}
	return nil
}

// enterGame enters a game without taking a seat as a player.
func (bp *BotPlayer) enterGame(gameCode string) error {
	gi, err := bp.GetGameInfo(gameCode)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to get game info %s", bp.logPrefix, gameCode))
	}

	bp.game = &gameView{
		table: &tableView{
			playersBySeat: make(map[uint32]*player),
			actionsRecord: game.NewHandActionRecord(),
			playersActed:  make(map[uint32]*game.PlayerActRound),
		},
	}

	err = bp.Subscribe(gi.GameToPlayerChannel, gi.HandToAllChannel, gi.HandToPlayerChannel)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to subscribe to game %s channels", bp.logPrefix, gameCode))
	}

	bp.gameToAll = gi.GameToPlayerChannel
	bp.handToAll = gi.HandToAllChannel
	bp.handToMe = gi.HandToPlayerChannel
	bp.meToHand = gi.PlayerToHandChannel

	gameID, err := bp.GetGameID(gameCode)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to get game ID for game code [%s]", bp.logPrefix, gameCode))
	}
	bp.gameCode = gameCode
	bp.gameID = gameID
	bp.gameInfo = &gi

	return nil
}

// ObserveGame enters the game without taking a seat as a player.
// Every player must call either JoinGame or ObserveGame in order to participate in a game.
func (bp *BotPlayer) ObserveGame(gameCode string) error {
	bp.logger.Info().Msgf("%s: Observing game %s", bp.logPrefix, gameCode)
	err := bp.enterGame(gameCode)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to enter game %s", bp.logPrefix, gameCode))
	}
	return nil
}

// JoinGame enters a game and takes a seat in the game table as a player.
// Every player must call either JoinGame or ObserveGame in order to participate in a game.
func (bp *BotPlayer) JoinGame(gameCode string) error {
	scriptSeatNo := bp.config.Script.GetSeatNoByPlayerName(bp.config.Name)
	if scriptSeatNo == 0 {
		return fmt.Errorf("%s: Unable to get the scripted seat number", bp.logPrefix)
	}
	scriptBuyInAmount := bp.config.Script.GetInitialBuyInAmount(scriptSeatNo)
	if scriptBuyInAmount == 0 {
		return fmt.Errorf("%s: Unable to get the scripted buy-in amount", bp.logPrefix)
	}

	err := bp.enterGame(gameCode)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to enter game %s", bp.logPrefix, gameCode))
	}

	playerInMySeat := bp.getPlayerInSeat(scriptSeatNo)
	if playerInMySeat != nil && playerInMySeat.Name == bp.config.Name {
		// I was already sitting in this game and still have my seat. Just rejoining after a crash.

		bp.event(BotEvent__REJOIN)

		if bp.gameInfo.PlayerGameStatus == game.PlayerStatus_WAIT_FOR_BUYIN.String() {
			// I was sitting, but crashed before submitting a buy-in.
			// The game's waiting for me to buy in, so that it can start a hand. Go ahead and submit a buy-in request.
			if bp.IsHuman() {
				bp.logger.Info().Msgf("%s: Press ENTER to buy in [%f] chips...", bp.logPrefix, scriptBuyInAmount)
				bufio.NewReader(os.Stdin).ReadBytes('\n')
			}
			err := bp.BuyIn(gameCode, scriptBuyInAmount)
			if err != nil {
				return errors.Wrap(err, fmt.Sprintf("%s: Unable to buy in after rejoining game", bp.logPrefix))
			}
			bp.balance = scriptBuyInAmount

			bp.event(BotEvent__SUCCEED_BUYIN)
		} else {
			// I was playing, but crashed in the middle of an ongoing hand. What is the state of the hand now?
			err := bp.queryCurrentHandState()
			if err != nil {
				return errors.Wrap(err, fmt.Sprintf("%s: Unable to query current hand state", bp.logPrefix))
			}
		}
	} else {
		// Joining a game from fresh.
		if bp.IsHuman() {
			bp.logger.Info().Msgf("%s: Press ENTER to take seat [%d]...", bp.logPrefix, scriptSeatNo)
			bufio.NewReader(os.Stdin).ReadBytes('\n')
		}

		bp.event(BotEvent__REQUEST_SIT)

		err := bp.SitIn(gameCode, scriptSeatNo)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("%s: Unable to sit in", bp.logPrefix))
		}

		if bp.IsHuman() {
			bp.logger.Info().Msgf("%s: Press ENTER to buy in [%f] chips...", bp.logPrefix, scriptBuyInAmount)
			bufio.NewReader(os.Stdin).ReadBytes('\n')
		}
		bp.buyInAmount = uint32(scriptBuyInAmount)
		err = bp.BuyIn(gameCode, scriptBuyInAmount)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("%s: Unable to buy in", bp.logPrefix))
		}

		bp.event(BotEvent__SUCCEED_BUYIN)
	}

	bp.seatNo = scriptSeatNo
	bp.balance = scriptBuyInAmount
	bp.updateLogPrefix()

	return nil
}

func (bp *BotPlayer) reload() error {
	err := bp.BuyIn(bp.gameCode, bp.gameInfo.BuyInMax)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to buy in", bp.logPrefix))
	}
	bp.balance = bp.gameInfo.BuyInMin
	return err
}

// JoinUnscriptedGame joins a game without using the yaml script. This is used for joining
// a human-created game where you can freely grab whatever seat available.
func (bp *BotPlayer) JoinUnscriptedGame(gameCode string) error {
	if !bp.config.Script.AutoPlay {
		return fmt.Errorf("%s: JoinUnscriptedGame called with a non-autoplay script", bp.logPrefix)
	}

	err := bp.enterGame(gameCode)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to enter game %s", bp.logPrefix, gameCode))
	}
	if len(bp.gameInfo.SeatInfo.AvailableSeats) == 0 {
		return fmt.Errorf("%s: Unable to join game [%s]. Seats are full", bp.logPrefix, gameCode)
	}
	seatNo := bp.gameInfo.SeatInfo.AvailableSeats[0]

	bp.event(BotEvent__REQUEST_SIT)

	err = bp.SitIn(gameCode, seatNo)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to sit in", bp.logPrefix))
	}
	buyInAmt := bp.gameInfo.BuyInMax
	err = bp.BuyIn(gameCode, buyInAmt)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to buy in", bp.logPrefix))
	}
	bp.seatNo = seatNo

	bp.event(BotEvent__SUCCEED_BUYIN)

	return nil
}

// SitIn takes a seat in a game as a player.
func (bp *BotPlayer) SitIn(gameCode string, seatNo uint32) error {
	bp.logger.Info().Msgf("%s: Grabbing seat [%d] in game [%s].", bp.logPrefix, seatNo, gameCode)
	status, err := bp.gqlHelper.SitIn(gameCode, seatNo)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to sit in game [%s]", bp.logPrefix, gameCode))
	}

	bp.observing = false
	bp.logger.Info().Msgf("%s: Successfully took a seat in game [%s]. Status: [%s]", bp.logPrefix, gameCode, status)
	bp.isSeated = true
	return nil
}

// BuyIn is where you buy the chips once seated in a game.
func (bp *BotPlayer) BuyIn(gameCode string, amount float32) error {
	bp.logger.Info().Msgf("%s: Buying in amount [%f].", bp.logPrefix, amount)

	resp, err := bp.gqlHelper.BuyIn(gameCode, amount)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to buy in", bp.logPrefix))
	}

	if resp.Status.Approved {
		bp.logger.Info().Msgf("%s: Successfully bought in [%f] chips.", bp.logPrefix, amount)
	} else {
		bp.logger.Info().Msgf("%s: Requested to buy in [%f] chips. Needs approval.", bp.logPrefix, amount)
	}

	return nil
}

// LeaveGame makes the bot leave the game.
func (bp *BotPlayer) LeaveGame() error {
	bp.logger.Info().Msgf("%s: Leaving game [%s].", bp.logPrefix, bp.gameCode)
	err := bp.unsubscribe()
	if err != nil {
		return errors.Wrap(err, "Error while unsubscribing from NATS subjects")
	}
	if bp.isSeated {
		_, err = bp.gqlHelper.LeaveGame(bp.gameCode)
		if err != nil {
			return errors.Wrap(err, "Error while making a GQL request to leave game")
		}
	}
	bp.end <- true
	return nil
}

// GetGameInfo queries the game info from the api server.
func (bp *BotPlayer) GetGameInfo(gameCode string) (gameInfo game.GameInfo, err error) {
	return bp.gqlHelper.GetGameInfo(gameCode)
}

// GetPlayersInSeat queries for the numeric game ID using the game code.
func (bp *BotPlayer) GetPlayersInSeat(gameCode string) ([]game.SeatInfo, error) {
	gameInfo, err := bp.GetGameInfo(gameCode)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("%s: Unable to get players in seat", bp.logPrefix))
	}
	return gameInfo.SeatInfo.PlayersInSeats, nil
}

// GetGameID queries for the numeric game ID using the game code.
func (bp *BotPlayer) GetGameID(gameCode string) (uint64, error) {
	return bp.gqlHelper.GetGameID(gameCode)
}

func (bp *BotPlayer) getPlayerID() (uint64, error) {
	playerID, err := bp.gqlHelper.GetPlayerID()
	if err != nil {
		return 0, errors.Wrap(err, fmt.Sprintf("%s: Unable to get player ID", bp.logPrefix))
	}
	if playerID.Name != bp.config.Name {
		return 0, fmt.Errorf("%s: Unable to get player ID. Player name [%s] does not match the bot player's name [%s]", bp.logPrefix, playerID.Name, bp.config.Name)
	}
	return playerID.ID, nil
}

// StartGame starts the game.
func (bp *BotPlayer) StartGame(gameCode string) error {
	bp.logger.Info().Msgf("%s: Starting the game [%s].", bp.logPrefix, gameCode)

	// setup first deck if not auto play
	if bp.IsHost() && !bp.config.Script.AutoPlay {
		bp.setupNextHand()
	}

	status, err := bp.gqlHelper.StartGame(gameCode)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to start the game [%s]", bp.logPrefix, gameCode))
	}
	if status != "ACTIVE" {
		return fmt.Errorf("%s: Unable to start the game [%s]. Status is [%s]", bp.logPrefix, gameCode, status)
	}

	bp.logger.Info().Msgf("%s: Successfully started the game [%s]. Status: [%s]", bp.logPrefix, gameCode, status)
	return nil
}

// RequestEndGame schedules to end the game after the current hand is finished.
func (bp *BotPlayer) RequestEndGame(gameCode string) error {
	bp.logger.Info().Msgf("%s: Requesting to end the game [%s] after the current hand.", bp.logPrefix, gameCode)

	status, err := bp.gqlHelper.EndGame(gameCode)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Error while requesting to end the game [%s]", bp.logPrefix, gameCode))
	}

	bp.logger.Info().Msgf("%s: Successfully requested to end the game [%s] after the current hand. Status: [%s]", bp.logPrefix, gameCode, status)
	return nil
}

func (bp *BotPlayer) queryCurrentHandState() error {
	// query current hand state
	msg := game.HandMessage{
		GameId:   bp.gameID,
		PlayerId: bp.PlayerID,
		//GameToken: 	 bp.GameToken,
		MessageType: game.HandQueryCurrentHand,
		HandMessage: nil,
	}
	protoData, err := protojson.Marshal(&msg)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Could not create query hand message.", bp.logPrefix))
	}
	bp.logger.Info().Msgf("%s: Querying current hand. Msg: %s", bp.logPrefix, string(protoData))
	// Send to hand subject.
	err = bp.natsConn.Publish(bp.meToHand, protoData)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to publish to nats", bp.logPrefix))
	}
	return nil
}

func (bp *BotPlayer) act(seatAction *game.NextSeatAction) {
	availableActions := seatAction.AvailableActions
	nextAction := game.ACTION_CHECK
	nextAmt := float32(0)
	autoPlay := false

	if bp.config.Script.AutoPlay {
		autoPlay = true
	} else if len(bp.config.Script.Hands) >= int(bp.game.table.handNum) {
		handScript := bp.config.Script.Hands[bp.game.table.handNum-1]
		if handScript.Setup.Auto {
			autoPlay = true
		}
	}

	if autoPlay {
		bp.logger.Info().Msgf("%s: Seat %d Available actions: %+v", bp.logPrefix, bp.seatNo, seatAction.AvailableActions)
		canBet := false
		canRaise := false
		checkAvailable := false
		callAvailable := false
		allInAvailable := false
		allInAmount := float32(0.0)
		minBet := float32(0.0)
		maxBet := float32(0.0)
		// we are in auto play now
		for _, action := range seatAction.AvailableActions {
			if action == game.ACTION_CHECK {
				checkAvailable = true
			}
			if action == game.ACTION_CALL {
				callAvailable = true
				nextAction = game.ACTION_CALL
				nextAmt = seatAction.CallAmount
			}
			if action == game.ACTION_BET {
				canBet = true
				minBet = seatAction.MinRaiseAmount
				maxBet = seatAction.MaxRaiseAmount
			}
			if action == game.ACTION_BET {
				canRaise = true
				minBet = seatAction.MinRaiseAmount
				maxBet = seatAction.MaxRaiseAmount
			}
			if action == game.ACTION_ALLIN {
				allInAvailable = true
				allInAmount = seatAction.AllInAmount
			}
		}

		if checkAvailable {
			nextAction = game.ACTION_CHECK
			nextAmt = 0.0
		}

		// do I have a pair
		if bp.havePair {
			pairValue := (float32)(bp.pairCard / 16)
			nextAmt = pairValue * minBet
			if nextAmt > maxBet {
				nextAmt = maxBet
			}
			if nextAmt == seatAction.AllInAmount {
				nextAction = game.ACTION_ALLIN
			} else {
				if canBet {
					nextAction = game.ACTION_BET
				} else if canRaise {
					nextAction = game.ACTION_RAISE
				}
			}
		}

		if nextAmt == 0 {
			if checkAvailable {
				nextAction = game.ACTION_CHECK
			} else {
				nextAction = game.ACTION_FOLD
			}
		}

		if !checkAvailable {
			if !callAvailable && allInAvailable {
				// go all in
				nextAction = game.ACTION_ALLIN
				nextAmt = allInAmount
			} else {
				if nextAmt > bp.balance/2 {
					// more than half of the balance, fold this hand
					nextAction = game.ACTION_FOLD
				} else if callAvailable {
					// call this bet
					nextAction = game.ACTION_CALL
					nextAmt = seatAction.CallAmount
				} else {
					nextAction = game.ACTION_FOLD
				}
			}
		}
	} else {
		var err error
		nextAction, nextAmt, err = bp.decision.GetNextAction(bp, availableActions)
		if err != nil {
			bp.logger.Error().Msgf("%s: Unable to get the next action %+v", bp.logPrefix, err)
			return
		}
	}

	if bp.IsHuman() {
		bp.logger.Info().Msgf("%s: Seat %d: Your Turn. Press ENTER to continue with [%s %f] (Hand Status: %s)...", bp.logPrefix, bp.seatNo, nextAction, nextAmt, bp.game.table.handStatus)
		bufio.NewReader(os.Stdin).ReadBytes('\n')
	}

	if !bp.IsHuman() {
		// Pause to think for some time to be realistic.
		time.Sleep(time.Duration(bp.config.BotActionPauseTime) * time.Millisecond)
	}

	handAction := game.HandAction{
		SeatNo: bp.seatNo,
		Action: nextAction,
		Amount: nextAmt,
	}
	msgType := game.HandPlayerActed
	msgID := bp.lastMsgID + 1
	actionMsg := game.HandMessage{
		ClubId:      uint32(bp.clubID),
		GameId:      bp.gameID,
		HandNum:     bp.game.table.handNum,
		PlayerId:    bp.PlayerID,
		MessageType: msgType,
		MessageId:   msgID,
		HandMessage: &game.HandMessage_PlayerActed{PlayerActed: &handAction},
	}

	go bp.publishAndWaitForAck(bp.meToHand, &actionMsg)
}

func (bp *BotPlayer) publishAndWaitForAck(subj string, msg *game.HandMessage) {
	protoData, err := protojson.Marshal(msg)
	if err != nil {
		errMsg := fmt.Sprintf("%s: Could not serialize hand message [%+v]. Error: %v", bp.logPrefix, msg, err)
		bp.logger.Error().Msg(errMsg)
		bp.errorStateMsg = errMsg
		bp.sm.SetState(BotState__ERROR)
		return
	}
	published := false
	ackReceived := false
	for attempts := 1; !ackReceived; attempts++ {
		if attempts > bp.ackMaxWait {
			var errMsg string
			if !published {
				errMsg = fmt.Sprintf("%s: Retry (%d) exhausted while publishing message type: %s, ID: %d", bp.logPrefix, bp.ackMaxWait, msg.GetMessageType(), msg.GetMessageId())
			} else {
				errMsg = fmt.Sprintf("%s: Retry (%d) exhausted while waiting for game server acknowledgement for message type: %s, ID: %d", bp.logPrefix, bp.ackMaxWait, msg.GetMessageType(), msg.GetMessageId())
			}
			bp.logger.Error().Msg(errMsg)
			bp.errorStateMsg = errMsg
			bp.sm.SetState(BotState__ERROR)
			return
		}
		if attempts > 1 {
			bp.logger.Info().Msgf("%s: Attempt (%d) to publish message type: %s, message ID: %d", bp.logPrefix, attempts, msg.GetMessageType(), msg.GetMessageId())
		}
		if err := bp.natsConn.Publish(bp.meToHand, protoData); err != nil {
			bp.logger.Error().Msgf("%s: Error [%s] while publishing message %+v", bp.logPrefix, err, msg)
			time.Sleep(2 * time.Second)
			continue
		}
		if !published {
			bp.sm.Event(BotEvent__SEND_MY_ACTION)
			bp.lastMsgID = msg.GetMessageId()
			bp.lastMsgType = msg.GetMessageType()
			published = true
		}
		time.Sleep(2 * time.Second)
		if bp.sm.Current() != BotState__ACTED_WAITING_FOR_ACK {
			ackReceived = true
		} else if bp.lastMsgID != msg.GetMessageId() {
			// Bots are acting very fast. This bot is already waiting for the ack for the next action.
			ackReceived = true
		}
	}
}

func (bp *BotPlayer) rememberPlayerAction(seatNo uint32, action game.ACTION, amount float32, timedOut bool, handStatus game.HandStatus) {
	bp.game.table.actionsRecord.RecordAction(seatNo, action, amount, timedOut, handStatus)

	state := game.ActionToActionState(action)
	bp.game.table.playersActed[seatNo] = &game.PlayerActRound{
		State:  state,
		Amount: amount,
	}
}

// IsObserver returns true if this bot is an observer bot.
func (bp *BotPlayer) IsObserver() bool {
	return bp.config.IsObserver
}

// IsHost returns true if this bot is the game host.
func (bp *BotPlayer) IsHost() bool {
	return bp.config.IsHost
}

// IsHuman returns true if this bot is a human player.
func (bp *BotPlayer) IsHuman() bool {
	return bp.config.IsHuman
}

// IsSeated returns true if this bot is sitting in a table.
func (bp *BotPlayer) IsSeated() bool {
	return bp.isSeated
}

// IsGameOver returns true if the bot has finished the game.
func (bp *BotPlayer) IsGameOver() bool {
	if bp.game == nil || bp.game.table == nil {
		return false
	}
	return bp.game.status == game.GameStatus_ENDED
}

// IsErrorState returns true if the bot is in an unrecoverable error state.
func (bp *BotPlayer) IsErrorState() bool {
	return bp.sm.Current() == BotState__ERROR
}

// GetErrorMsg returns the cause of the error state (BotState__ERROR).
func (bp *BotPlayer) GetErrorMsg() string {
	return bp.errorStateMsg
}

// GetHandResult returns the hand result received from the server.
func (bp *BotPlayer) GetHandResult() *game.HandResult {
	return bp.game.table.handResult
}

// PrintHandResult prints the hand winners to console.
func (bp *BotPlayer) PrintHandResult() {
	result := bp.GetHandResult()
	data, _ := json.Marshal(result)
	bp.logger.Info().Msg(string(data))
	pots := bp.GetHandResult().GetHandLog().GetPotWinners()
	for potNum, potWinners := range pots {
		for i, winner := range potWinners.HiWinners {
			seatNo := winner.GetSeatNo()
			playerName := bp.getPlayerNameBySeatNo(seatNo)
			amount := winner.GetAmount()
			cardsStr := winner.GetWinningCardsStr()
			rankStr := winner.GetRankStr()
			winningCards := ""
			if cardsStr != "" {
				winningCards = fmt.Sprintf(" Winning Cards: %s (%s)", cardsStr, rankStr)
			}
			bp.logger.Info().Msgf("%s: Pot %d Hi-Winner %d: Seat %d (%s) Amount: %f%s", bp.logPrefix, potNum+1, i+1, seatNo, playerName, amount, winningCards)
		}
		for i, winner := range potWinners.LowWinners {
			seatNo := winner.GetSeatNo()
			playerName := bp.getPlayerNameBySeatNo(seatNo)
			amount := winner.GetAmount()
			cardsStr := winner.GetWinningCardsStr()
			rankStr := winner.GetRankStr()
			winningCards := ""
			if cardsStr != "" {
				winningCards = fmt.Sprintf(" Winning Cards: %s (%s)", cardsStr, rankStr)
			}
			bp.logger.Info().Msgf("%s: Pot %d Low-Winner %d: Seat %d (%s) Amount: %f%s", bp.logPrefix, potNum+1, i+1, seatNo, playerName, amount, winningCards)
		}
	}
}

// Login authenticates the player and stores its jwt for future api calls.
func (bp *BotPlayer) Login(playerUUID string, deviceID string) error {
	userJwt, err := bp.GetJWT(playerUUID, deviceID)
	if err != nil {
		return err
	}
	bp.apiAuthToken = fmt.Sprintf("jwt %s", userJwt)
	bp.gqlHelper.SetAuthToken(bp.apiAuthToken)
	return nil
}

// GetJWT authenticates the player and returns the jwt.
func (bp *BotPlayer) GetJWT(playerUUID string, deviceID string) (string, error) {

	// after we created the player
	// authenticate the player and get JWT
	type login struct {
		UUID     string `json:"uuid"`
		DeviceID string `json:"device-id"`
	}
	loginData := login{
		UUID:     playerUUID,
		DeviceID: deviceID,
	}

	jsonValue, _ := json.Marshal(loginData)
	loginURL := fmt.Sprintf("%s/auth/login", bp.config.APIServerURL)
	response, err := http.Post(loginURL, "application/json", bytes.NewBuffer(jsonValue))
	if err != nil {
		return "", errors.Wrap(err, "Login request failed")
	}

	type JwtResp struct {
		Jwt string
	}
	var jwtData JwtResp

	data, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", errors.Wrap(err, "Unabgle to read login response body")
	}
	body := string(data)
	if strings.Contains(body, "errors") {
		return "", fmt.Errorf("Login response for user %s contains error: %s", bp.config.Name, body)
	}

	json.Unmarshal(data, &jwtData)

	token, _, err := new(jwt.Parser).ParseUnverified(jwtData.Jwt, jwt.MapClaims{})
	if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("Error while parsing jwt response. Response body: [%s]", body))
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		bp.PlayerID = uint64(claims["id"].(float64))
	} else {
		bp.logger.Error().Msgf("%s: Error while processing jwt: %s", bp.logPrefix, err)
	}
	return jwtData.Jwt, nil
}

func (bp *BotPlayer) getPlayerInSeat(seatNo uint32) *game.SeatInfo {
	for _, p := range bp.gameInfo.SeatInfo.PlayersInSeats {
		if p.SeatNo == seatNo {
			return &p
		}
	}
	return nil
}

func (bp *BotPlayer) setupNextHand() error {

	if int(bp.deckHandNum) >= len(bp.config.Script.Hands) {
		return nil
	}

	currentHand := bp.config.Script.Hands[bp.deckHandNum]

	// setup deck
	var setupDeckMsg *SetupDeck
	if currentHand.Setup.Auto {
		setupDeckMsg = &SetupDeck{
			MessageType: BotDriverSetupDeck,
			GameCode:    bp.gameCode,
			GameID:      bp.gameID,
			Auto:        true,
			Pause:       currentHand.Setup.Pause,
		}
		if currentHand.Setup.ButtonPos != 0 {
			setupDeckMsg.ButtonPos = currentHand.Setup.ButtonPos
		}
	} else {
		setupDeckMsg = &SetupDeck{
			MessageType: BotDriverSetupDeck,
			Pause:       currentHand.Setup.Pause,
			GameCode:    bp.gameCode,
			GameID:      bp.gameID,
			ButtonPos:   currentHand.Setup.ButtonPos,
			Flop:        currentHand.Setup.Flop,
			Turn:        currentHand.Setup.Turn,
			River:       currentHand.Setup.River,
			PlayerCards: bp.getPlayerCardsFromConfig(currentHand.Setup.SeatCards),
		}
	}
	msgBytes, err := jsoniter.Marshal(setupDeckMsg)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Unable to marshal SetupDeck BotDriverSetupDeck message %+v", setupDeckMsg))
	}
	bp.natsConn.Publish(util.GetDriverToGameSubject(), msgBytes)
	bp.deckHandNum++
	return nil
}

func (bp *BotPlayer) getPlayerCardsFromConfig(seatCards []gamescript.SeatCards) []PlayerCard {
	var playerCards []PlayerCard
	for _, seatCard := range seatCards {
		cards := seatCard.Cards
		playerCards = append(playerCards, PlayerCard{
			Cards: cards,
		})
	}
	return playerCards
}

func (bp *BotPlayer) storeGameInfo() error {
	gi, err := bp.GetGameInfo(bp.gameCode)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to get game info %s", bp.logPrefix, bp.gameCode))
	}
	bp.gameInfo = &gi
	return nil
}

func (bp *BotPlayer) isGamePaused() (bool, error) {
	gi, err := bp.GetGameInfo(bp.gameCode)
	if err != nil {
		return false, errors.Wrap(err, fmt.Sprintf("%s: Unable to get game info %s", bp.logPrefix, bp.gameCode))
	}
	if gi.Status == game.GameStatus_PAUSED.String() {
		return true, nil
	}
	return false, nil
}

func (bp *BotPlayer) getPlayerNameBySeatNo(seatNo uint32) string {
	for _, p := range bp.gameInfo.SeatInfo.PlayersInSeats {
		if p.SeatNo == seatNo {
			return p.Name
		}
	}
	return "MISSING"
}

// GetName returns the player's name (e.g., tom)
func (bp *BotPlayer) GetName() string {
	return bp.config.Name
}

// GetClubCode finds club code by club name.
func (bp *BotPlayer) GetClubCode(name string) (string, error) {
	bp.logger.Info().Msgf("%s: Locating club code using name [%s].", bp.logPrefix, name)

	clubCode, err := bp.gqlHelper.GetClubCode(name)
	if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("%s: Unable to get clubs", bp.logPrefix))
	}
	if name == "" {
		bp.logger.Info().Msgf("%s: No club found with name: [%s]", bp.logPrefix, name)
		return "", nil
	}
	return clubCode, nil
}
