package player

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/google/go-cmp/cmp"
	jsoniter "github.com/json-iterator/go"
	"github.com/looplab/fsm"
	"github.com/machinebox/graphql"
	natsgo "github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"voyager.com/botrunner/internal/game"
	"voyager.com/botrunner/internal/gql"
	"voyager.com/botrunner/internal/poker"
	"voyager.com/botrunner/internal/rest"
	"voyager.com/botrunner/internal/util"
	"voyager.com/encryption"
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
	MinActionPauseTime uint32
	MaxActionPauseTime uint32
	APIServerURL       string
	NatsURL            string
	GQLTimeoutSec      int
	Players            *gamescript.Players
	Script             *gamescript.Script
}

type GameMessageChannelItem struct {
	ProtoGameMsg    *game.GameMessage
	NonProtoGameMsg *gamescript.NonProtoMessage
}

// BotPlayer represents a bot user.
type BotPlayer struct {
	logger    *zerolog.Logger
	config    Config
	IpAddress string
	Gps       *gamescript.GpsLocation

	gqlHelper       *gql.GQLHelper
	restHelper      *rest.RestClient
	natsConn        *natsgo.Conn
	apiAuthToken    string
	clubCode        string
	clubID          uint64
	gameCode        string
	gameID          uint64
	PlayerID        uint64
	PlayerUUID      string
	EncryptionKey   string
	RewardsNameToID map[string]uint32
	scriptedGame    bool

	// state of the bot
	sm *fsm.FSM

	// current status
	buyInAmount uint32
	havePair    bool
	pairCard    uint32
	balance     float32

	// For message acknowledgement
	clientLastMsgID   string
	clientLastMsgType string
	maxRetry          int

	// bots in game
	bots []*BotPlayer

	// Remember the most recent message ID's for deduplicating server messages.
	serverLastMsgIDs *util.Queue

	// Message channels
	chGame  chan *GameMessageChannelItem
	chHand  chan *game.HandMessage
	chPing  chan *game.PingPongMessage
	end     chan bool
	endPing chan bool

	// Points to the most recent messages from the game server.
	lastGameMessage *game.GameMessage
	lastHandMessage *game.HandMessage
	//playerStateMessage *game.GameTableStateMessage

	// GameInfo received from the api server.
	gameInfo *game.GameInfo

	// Seat change variables
	requestedSeatChange bool
	confirmSeatChange   bool

	// wait list variables
	inWaitList      bool
	confirmWaitlist bool

	// other config
	muckLosingHand bool

	// Nats subjects
	gameToAll       string
	handToAll       string
	handToMe        string
	meToHand        string
	pingSubjectName string
	pongSubjectName string

	// Nats subscription objects
	gameMsgSubscription         *natsgo.Subscription
	handMsgAllSubscription      *natsgo.Subscription
	handMsgPlayerSubscription   *natsgo.Subscription
	playerMsgPlayerSubscription *natsgo.Subscription
	pingSubscription            *natsgo.Subscription

	game      *gameView
	seatNo    uint32
	observing bool // if a player is playing, then he is also an observer

	logPrefix string

	// Print nats messages for debugging.
	printGameMsg  bool
	printHandMsg  bool
	printStateMsg bool

	decision ScriptBasedDecision

	isSeated             bool
	hasNextHandBeenSetup bool // For host only

	// The bot wants to leave after the current hand and has sent the
	// leaveGame request to the api server.
	hasSentLeaveGameRequest bool

	// Error msg if the bot is in an error state (BotState__ERROR).
	errorStateMsg string

	// messages received in the player/private channel
	PrivateMessages []map[string]interface{}
	GameMessages    []*gamescript.NonProtoMessage
}

// NewBotPlayer creates an instance of BotPlayer.
func NewBotPlayer(playerConfig Config, logger *zerolog.Logger) (*BotPlayer, error) {
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
	restHelper := rest.NewRestClient(util.GetInternalRestURL(playerConfig.APIServerURL), uint32(playerConfig.GQLTimeoutSec), "")

	bp := BotPlayer{
		logger:           logger,
		config:           playerConfig,
		PlayerUUID:       playerConfig.DeviceID,
		gqlHelper:        gqlHelper,
		restHelper:       restHelper,
		natsConn:         nc,
		chGame:           make(chan *GameMessageChannelItem, 10),
		chHand:           make(chan *game.HandMessage, 10),
		chPing:           make(chan *game.PingPongMessage, 10),
		end:              make(chan bool),
		endPing:          make(chan bool),
		logPrefix:        logPrefix,
		printGameMsg:     util.Env.ShouldPrintGameMsg(),
		printHandMsg:     util.Env.ShouldPrintHandMsg(),
		printStateMsg:    util.Env.ShouldPrintStateMsg(),
		RewardsNameToID:  make(map[string]uint32),
		clientLastMsgID:  "0",
		serverLastMsgIDs: util.NewQueue(10),
		maxRetry:         300,
		scriptedGame:     true,
		PrivateMessages:  make([]map[string]interface{}, 0),
		GameMessages:     make([]*gamescript.NonProtoMessage, 0),
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
				Src:  []string{BotState__ACTED_WAITING_FOR_ACK, BotState__MY_TURN},
				Dst:  BotState__WAITING_FOR_MY_TURN,
			},
		},
		fsm.Callbacks{
			"enter_state": func(e *fsm.Event) { bp.enterState(e) },
		},
	)
	go bp.messageLoop()
	go bp.pingMessageLoop()
	return &bp, nil
}

func (bp *BotPlayer) SetBotsInGame(bots []*BotPlayer) {
	bp.bots = bots
}

func (bp *BotPlayer) enterState(e *fsm.Event) {
	if bp.printStateMsg {
		bp.logger.Info().Msgf("%s: [%s] ===> [%s]", bp.logPrefix, e.Src, e.Dst)
	}
}

func (bp *BotPlayer) event(event string) error {
	err := bp.sm.Event(event)
	if err != nil {
		bp.logger.Warn().Msgf("%s: Error from state machine: %s", bp.logPrefix, err.Error())
	}
	return err
}

func (bp *BotPlayer) updateLogPrefix() {
	if bp.config.IsHuman {
		bp.logPrefix = fmt.Sprintf("Player [%s:%d:%d]", bp.config.Name, bp.PlayerID, bp.seatNo)
	} else {
		bp.logPrefix = fmt.Sprintf("Bot [%s:%d:%d]", bp.config.Name, bp.PlayerID, bp.seatNo)
	}
}

func (bp *BotPlayer) SetIPAddress(ipAddress string) {
	bp.gqlHelper.IpAddress = ipAddress
}

func (bp *BotPlayer) SetGpsLocation(gps *gamescript.GpsLocation) {
	bp.Gps = gps
}

func (bp *BotPlayer) handleGameMsg(msg *natsgo.Msg) {
	if bp.printGameMsg {
		bp.logger.Info().Msgf("%s: Received game message %s", bp.logPrefix, string(msg.Data))
	}

	var message game.GameMessage
	var nonProtoMsg gamescript.NonProtoMessage
	err := protojson.Unmarshal(msg.Data, &message)
	if err != nil {
		// bp.logger.Debug().Msgf("%s: Error [%s] while unmarshalling protobuf game message [%s]. Assuming non-protobuf message", bp.logPrefix, err, string(msg.Data))
		err = json.Unmarshal(msg.Data, &nonProtoMsg)
		if err != nil {
			bp.logger.Error().Msgf("%s: Error [%s] while unmarshalling non-protobuf game message [%s]", bp.logPrefix, err, string(msg.Data))
			return
		}
		bp.chGame <- &GameMessageChannelItem{
			ProtoGameMsg:    nil,
			NonProtoGameMsg: &nonProtoMsg,
		}
	} else {
		bp.chGame <- &GameMessageChannelItem{
			ProtoGameMsg:    &message,
			NonProtoGameMsg: nil,
		}
	}
}

func (bp *BotPlayer) handleHandMsg(msg *natsgo.Msg) {
	bp.unmarshalAndQueueHandMsg(msg.Data)
}

func (bp *BotPlayer) handlePrivateHandMsg(msg *natsgo.Msg) {
	data := msg.Data
	if util.Env.IsEncryptionEnabled() {
		decryptedMsg, err := encryption.DecryptWithUUIDStrKey(msg.Data, bp.EncryptionKey)
		if err != nil {
			bp.logger.Error().Msgf("%s: Error [%s] while decrypting private hand message", bp.logPrefix, err)
			return
		}
		data = decryptedMsg
	}

	bp.unmarshalAndQueueHandMsg(data)
}

func (bp *BotPlayer) handlePlayerPrivateMsg(msg *natsgo.Msg) {
	var jsonMessage map[string]interface{}
	err := json.Unmarshal(msg.Data, &jsonMessage)
	if err == nil {
		bp.PrivateMessages = append(bp.PrivateMessages, jsonMessage)
	}
}

func (bp *BotPlayer) unmarshalAndQueueHandMsg(data []byte) {
	var message game.HandMessage
	err := proto.Unmarshal(data, &message)
	if err != nil {
		bp.logger.Error().Msgf("%s: Error [%s] while unmarshalling protobuf hand message [%s]", bp.logPrefix, err, string(data))
		return
	}

	bp.chHand <- &message
}

func (bp *BotPlayer) handlePingMsg(msg *natsgo.Msg) {
	var message game.PingPongMessage
	err := proto.Unmarshal(msg.Data, &message)
	if err != nil {
		bp.logger.Error().Msgf("%s: Error [%s] while unmarshalling protobuf ping message [%s]", bp.logPrefix, err, string(msg.Data))
		return
	}

	bp.chPing <- &message
}

func (bp *BotPlayer) messageLoop() {
	for {
		select {
		case <-bp.end:
			return
		case chItem := <-bp.chGame:
			if chItem.ProtoGameMsg != nil {
				if chItem.ProtoGameMsg.MessageType == "PLAYER_CONNECTIVITY_LOST" ||
					chItem.ProtoGameMsg.MessageType == "PLAYER_CONNECTIVITY_RESTORED" {

				} else {
					panic("We should not get any messages from game server")
				}
			} else if chItem.NonProtoGameMsg != nil {
				bp.processNonProtoGameMessage(chItem.NonProtoGameMsg)
			}
		case message := <-bp.chHand:
			bp.processHandMessage(message)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (bp *BotPlayer) pingMessageLoop() {
	for {
		select {
		case <-bp.endPing:
			return
		case message := <-bp.chPing:
			bp.respondToPing(message)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (bp *BotPlayer) processHandMessage(message *game.HandMessage) {
	if bp.IsErrorState() {
		bp.logger.Info().Msgf("%s: Bot is in error state. Ignoring hand message.", bp.logPrefix)
		return
	}

	if message.MessageId == "" {
		bp.logger.Panic().Msgf("%s: Hand message from server is missing message ID. Message: %s", bp.logPrefix, message.String())
	}

	if bp.serverLastMsgIDs.Contains(message.MessageId) {
		// Duplicate message potentially due to server restart. Ignore it.
		bp.logger.Info().Msgf("%s: Ignoring duplicate hand message ID: %s", bp.logPrefix, message.MessageId)
		return
	}
	bp.serverLastMsgIDs.Push(message.MessageId)

	if message.PlayerId != 0 && message.PlayerId != bp.PlayerID {
		// drop this message
		// this message was targeted for another player
		return
	}

	bp.lastHandMessage = message

	for i, msgItem := range message.GetMessages() {
		bp.processMsgItem(message, msgItem, i)
	}
}

func (bp *BotPlayer) processMsgItem(message *game.HandMessage, msgItem *game.HandMessageItem, msgItemIdx int) {
	switch msgItem.MessageType {
	case game.HandDeal:
		deal := msgItem.GetDealCards()
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

	case game.HandDealerChoice:
		dealerChoice := msgItem.GetDealerChoice()
		bp.logger.Info().Msgf("%s: Dealer choice games: (%+v)", bp.logPrefix, dealerChoice.Games)
		nextGameIdx := rand.Intn(len(dealerChoice.Games))
		nextGame := dealerChoice.Games[nextGameIdx]
		bp.chooseNextGame(nextGame)

	case game.HandNewHand:
		/* MessageType: NEW_HAND */
		bp.game.table.playersActed = make(map[uint32]*game.PlayerActRound)
		bp.reloadBotFromGameInfo()
		bp.game.handNum = message.HandNum
		bp.game.handStatus = message.GetHandStatus()
		newHand := msgItem.GetNewHand()
		bp.game.table.buttonPos = newHand.GetButtonPos()
		bp.game.table.sbPos = newHand.GetSbPos()
		bp.game.table.bbPos = newHand.GetBbPos()
		bp.game.table.nextActionSeat = newHand.GetNextActionSeat()
		bp.game.table.actionTracker = game.NewHandActionTracker()

		bp.hasNextHandBeenSetup = false // Not this hand, but the next one.

		if bp.IsHost() {
			data, _ := proto.Marshal(message)
			bp.logger.Info().Msgf("A new hand is started. Hand Num: %d, message: %s", message.HandNum, string(data))
			if !bp.config.Script.AutoPlay {
				if int(message.HandNum) == len(bp.config.Script.Hands) {
					bp.logger.Info().Msgf("%s: Last hand: %d Game will be ended in next hand", bp.logPrefix, message.HandNum)

					// The host bot should schedule to end the game after this hand is over.
					go func() {
						bp.RequestEndGame(bp.gameCode)
					}()
				}
			}
			bp.pauseGameIfNeeded()

			if !bp.config.Script.AutoPlay {
				bp.verifyNewHand(message.HandNum, newHand)
			}
		}

		// update seat number
		for seatNo, player := range newHand.PlayersInSeats {
			if player.PlayerId == bp.PlayerID {
				if bp.seatNo != seatNo {
					bp.logger.Info().Msgf("%s: Player: %s changed seat from %d to %d", bp.logPrefix, player.Name, bp.seatNo, seatNo)
					bp.seatNo = seatNo
					bp.updateLogPrefix()
				}
				break
			}
		}
		// update player config
		bp.updatePlayersConfig()

		// setup seat change requests
		bp.setupSeatChange()

		// setup run-it-twice prompt/response
		bp.setupRunItTwice()

		// setup take break request
		bp.setupTakeBreak()

		// setup sit back request
		bp.setupSitBack()

		// process any leave game requests
		// the player will after this hand
		bp.setupLeaveGame()

		// setup switch seats
		bp.setupSwitchSeats()

		// setup reload chips
		bp.setupReloadChips()

	case game.HandFlop:
		/* MessageType: FLOP */
		bp.game.handStatus = message.GetHandStatus()
		bp.game.table.flopCards = msgItem.GetFlop().GetBoard()
		if bp.IsHuman() || bp.IsObserver() {
			bp.logger.Info().Msgf("%s: Flop cards shown: %s Rank: %v", bp.logPrefix, msgItem.GetFlop().GetCardsStr(), msgItem.GetFlop().PlayerCardRanks)
		}
		if bp.IsHost() {
			bp.verifyBoard()
		}

		// Game server evaluates player cards at every betting round and sends the rank string for each seat number.
		// Verify they match the script.
		bp.verifyCardRank(msgItem.GetFlop().GetPlayerCardRanks())

		bp.updateBalance(msgItem.GetFlop().GetPlayerBalance())

		//time.Sleep(1 * time.Second)
		bp.game.table.playersActed = make(map[uint32]*game.PlayerActRound)

	case game.HandTurn:
		/* MessageType: TURN */
		bp.game.handStatus = message.GetHandStatus()
		bp.game.table.turnCards = msgItem.GetTurn().GetBoard()
		if bp.IsHuman() || bp.IsObserver() {
			bp.logger.Info().Msgf("%s: Turn cards shown: %s Rank: %v", bp.logPrefix, msgItem.GetTurn().GetCardsStr(), msgItem.GetTurn().PlayerCardRanks)
		}
		bp.verifyBoard()
		bp.verifyCardRank(msgItem.GetTurn().GetPlayerCardRanks())
		bp.updateBalance(msgItem.GetTurn().GetPlayerBalance())
		//time.Sleep(1 * time.Second)
		bp.game.table.playersActed = make(map[uint32]*game.PlayerActRound)

	case game.HandRiver:
		/* MessageType: RIVER */
		bp.game.handStatus = message.GetHandStatus()
		bp.game.table.riverCards = msgItem.GetRiver().GetBoard()
		if bp.IsHuman() || bp.IsObserver() {
			bp.logger.Info().Msgf("%s: River cards shown: %s Rank: %v", bp.logPrefix, msgItem.GetRiver().GetCardsStr(), msgItem.GetRiver().PlayerCardRanks)
		}
		bp.verifyBoard()
		bp.verifyCardRank(msgItem.GetRiver().GetPlayerCardRanks())
		bp.updateBalance(msgItem.GetRiver().GetPlayerBalance())
		//time.Sleep(1 * time.Second)
		bp.game.table.playersActed = make(map[uint32]*game.PlayerActRound)

	case game.HandPlayerAction:
		/* MessageType: YOUR_ACTION */
		seatAction := msgItem.GetSeatAction()
		seatNo := seatAction.GetSeatNo()
		if seatNo != bp.seatNo {
			// It's not my turn.
			break
		}
		err := bp.event(BotEvent__RECEIVE_YOUR_ACTION)
		if err != nil {
			// State transition failed due to unexpected YOUR_ACTION message. Possible cause is game server sent a duplicate
			// YOUR_ACTION message as part of the crash recovery. Ignore the message.
			bp.logger.Info().Msgf("%s: Ignoring unexpected %s message.", bp.logPrefix, game.HandPlayerAction)
			break
		}
		bp.game.handStatus = message.GetHandStatus()
		if bp.IsObserver() && bp.config.Script.IsSeatHuman(seatNo) {
			bp.logger.Info().Msgf("%s: Waiting on seat %d (%s/human) to act.", bp.logPrefix, seatNo, bp.getPlayerNameBySeatNo(seatNo))
		}
		bp.act(seatAction)

	case game.HandPlayerActed:
		/* MessageType: PLAYER_ACTED */
		if bp.IsHost() && !bp.config.Script.AutoPlay && !bp.hasNextHandBeenSetup {
			// We're just using this message as a signal that the betting
			// round is in progress and we are now ready to setup the next hand.
			bp.setupNextHand()
			bp.hasNextHandBeenSetup = true
		}

		playerActed := msgItem.GetPlayerActed()
		seatNo := playerActed.GetSeatNo()
		action := playerActed.GetAction()
		amount := playerActed.GetAmount()
		isTimedOut := playerActed.GetTimedOut()
		var timedout string
		if isTimedOut {
			timedout = " (TIMED OUT)"
		}
		actedPlayerName := bp.getPlayerNameBySeatNo(seatNo)
		lastActedSeat := bp.getLastActedSeatFromTracker()
		if lastActedSeat == seatNo {
			// This is a duplicate PLAYER_ACTED message (possibly from the game server crash & resume).
			// Don't add to the tracker.
		} else {
			bp.rememberPlayerAction(seatNo, action, amount, isTimedOut, bp.game.handStatus)
		}
		if bp.IsObserver() {
			actedPlayerType := "bot"
			if bp.config.Script.IsSeatHuman(seatNo) {
				actedPlayerType = "human"
			}
			bp.logger.Info().Msgf("%s: Seat %d (%s/%s) acted%s [%s %f] Stage:%s.", bp.logPrefix, seatNo, actedPlayerName, actedPlayerType, timedout, action, amount, bp.game.handStatus)
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
		msgType := msgItem.GetMsgAck().GetMessageType()
		msgID := msgItem.GetMsgAck().GetMessageId()
		msg := fmt.Sprintf("%s: Ignoring unexpected %s msg - %s:%s BotState: %s, CurrentMsgType: %s, CurrentMsgID: %s", bp.logPrefix, game.HandMsgAck, msgType, msgID, bp.sm.Current(), bp.clientLastMsgType, bp.clientLastMsgID)
		if msgType != bp.clientLastMsgType {
			bp.logger.Info().Msg(msg)
			return
		}
		if msgID != bp.clientLastMsgID {
			bp.logger.Info().Msg(msg)
			return
		}
		err := bp.event(BotEvent__RECEIVE_ACK)
		if err != nil {
			bp.logger.Info().Msg(msg)
		}

	// case game.HandResultMessage:
	// 	/* MessageType: RESULT */
	// 	bp.game.handStatus = message.GetHandStatus()
	// 	bp.game.handResult = msgItem.GetHandResult()
	// 	if bp.IsObserver() {
	// 		bp.PrintHandResult()
	// 		bp.verifyResult()
	// 	}

	// 	result := bp.game.handResult
	// 	for seatNo, player := range result.Players {
	// 		if seatNo == bp.seatNo {
	// 			if player.Balance.After == 0.0 {
	// 				// reload chips
	// 				bp.reload()
	// 			}
	// 			break
	// 		}
	// 	}

	case game.HandResultMessage2:
		/* MessageType: RESULT */
		bp.game.handStatus = message.GetHandStatus()
		bp.game.handResult2 = msgItem.GetHandResultClient()
		if bp.IsObserver() {
			bp.PrintHandResult()
			bp.verifyResult2()
		}

		//result := bp.game.handResult2
		// for seatNo, player := range result.PlayerInfo {
		// 	if seatNo == 0 {
		// 		continue
		// 	}
		// 	if seatNo == bp.seatNo {
		// 		if player.Balance.After == 0.0 {
		// 			// reload chips
		// 			bp.reload()
		// 		}
		// 		break
		// 	}
		// }

	case game.HandEnded:
		bp.logger.Info().Msgf("%s: IsHost: %v handNum: %d ended", bp.logPrefix, bp.IsHost(), message.HandNum)
		if bp.IsHost() {
			// process post hand steps if specified
			bp.processPostHandSteps()
		}
		if bp.hasSentLeaveGameRequest {
			bp.LeaveGameImmediately()
		}

	case game.HandQueryCurrentHand:
		currentState := msgItem.GetCurrentHandState()
		bp.logger.Info().Msgf("%s: Received current hand state: %+v", bp.logPrefix, currentState)
		if message.HandNum == 0 {
			bp.logger.Info().Msgf("%s: Ignoring current hand state message (handNum = 0)", bp.logPrefix)
			return
		}
		if message.HandNum < bp.game.handNum {
			bp.logger.Info().Msgf("%s: Ignoring current hand state message (message handNum = %d, hand in progress = %d)", bp.logPrefix, message.HandNum, bp.game.handNum)
			return
		}
		handStatus := currentState.GetCurrentRound()
		playersActed := currentState.GetPlayersActed()
		nextSeatAction := currentState.GetNextSeatAction()
		actionSeatNo := nextSeatAction.GetSeatNo()
		bp.game.handStatus = handStatus
		bp.game.table.nextActionSeat = actionSeatNo
		bp.game.table.playersActed = playersActed
		if bp.game.table.playersActed == nil {
			bp.game.table.playersActed = make(map[uint32]*game.PlayerActRound)
		}
		bp.game.handNum = message.HandNum

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

func (bp *BotPlayer) respondToPing(pingMsg *game.PingPongMessage) error {
	msg := game.PingPongMessage{
		GameId:   bp.gameID,
		PlayerId: bp.PlayerID,
		Seq:      pingMsg.GetSeq(),
	}

	protoData, err := proto.Marshal(&msg)
	if err != nil {
		return errors.Wrap(err, "Could not proto-marshal pong message.")
	}
	// bp.logger.Debug().Msgf("%s: Sending PONG. Msg: %s", bp.logPrefix, string(protoData))
	// Send to hand subject.
	err = bp.natsConn.Publish(bp.pongSubjectName, protoData)
	if err != nil {
		return errors.Wrapf(err, "Unable to publish pong message to nats channel %s", bp.pongSubjectName)
	}

	return nil
}

func (bp *BotPlayer) chooseNextGame(gameType game.GameType) {
	bp.gqlHelper.DealerChoice(bp.gameCode, gameType)
}

func (bp *BotPlayer) verifyBoard() {
	// if the script is configured to auto play, return
	if bp.config.Script.AutoPlay {
		return
	}

	if bp.game.handNum == 0 {
		return
	}

	var expectedBoard []string
	var currentBoard []uint32
	scriptCurrentHand := bp.config.Script.GetHand(bp.game.handNum)
	switch bp.game.handStatus {
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
		bp.logger.Panic().Msgf("%s: Hand %d %s verify failed. Board does not match the expected. Current board: %v. Expected board: %v.", bp.logPrefix, bp.game.handNum, bp.game.handStatus, currentBoardCards, expectedBoardCards)
	}
}

func (bp *BotPlayer) updateBalance(playerBalances map[uint32]float32) {
	balance, exists := playerBalances[bp.seatNo]
	if exists {
		bp.balance = balance
	}
}

func (bp *BotPlayer) verifyCardRank(currentRanks map[uint32]string) {
	// if the script is configured to auto play, return
	if bp.config.Script.AutoPlay {
		return
	}

	if bp.game.handNum == 0 {
		return
	}

	if bp.observing {
		return
	}

	var expectedRanks []gamescript.SeatRank
	scriptCurrentHand := bp.config.Script.GetHand(bp.game.handNum)
	switch bp.game.handStatus {
	case game.HandStatus_FLOP:
		expectedRanks = scriptCurrentHand.Flop.Verify.Ranks
	case game.HandStatus_TURN:
		expectedRanks = scriptCurrentHand.Turn.Verify.Ranks
	case game.HandStatus_RIVER:
		expectedRanks = scriptCurrentHand.River.Verify.Ranks
	}
	if len(expectedRanks) == 0 {
		// No verify in yaml.
		return
	}

	var expectedRank string
	for _, r := range expectedRanks {
		if r.Seat == bp.seatNo {
			expectedRank = r.RankStr
			break
		}
	}
	if expectedRank == "" {
		return
	}

	var actualRank = currentRanks[bp.seatNo]
	if util.Env.IsEncryptionEnabled() {
		// Player rank string is encrypted and base64 encoded by the game server.
		// It first needs to be b64 decoded and then decrypted using the player's
		// encryption key.
		decodedRankStr, err := encryption.B64DecodeString(actualRank)
		if err != nil {
			bp.logger.Panic().Msgf("%s: Unable to decode player rank string %s", bp.logPrefix, actualRank)
		}
		decrypted, err := encryption.DecryptWithUUIDStrKey(decodedRankStr, bp.EncryptionKey)
		if err != nil {
			bp.logger.Error().Msgf("%s: Error [%s] while decrypting private hand message", bp.logPrefix, err)
			return
		}
		actualRank = string(decrypted)
	}

	if actualRank != expectedRank {
		bp.logger.Panic().Msgf("%s: Hand %d %s verify failed. Player rank string does not match the expected. Current rank: %s. Expected rank: %s.", bp.logPrefix, bp.game.handNum, bp.game.handStatus, actualRank, expectedRank)
	}
}

func (bp *BotPlayer) verifyNewHand(handNum uint32, newHand *game.NewHand) {
	currentHand := bp.config.Script.GetHand(handNum)
	verify := currentHand.Setup.Verify
	if len(verify.Seats) > 0 {
		// if len(verify.Seats) != len(newHand.PlayersInSeats) {
		// 	errMsg := fmt.Sprintf("Number of players in the table is not matching. Expected: %v Actual: %v", verify.Seats, newHand.PlayersInSeats)
		// 	bp.logger.Error().Msg(errMsg)
		// 	panic(errMsg)
		// }
		for _, seat := range currentHand.Setup.Verify.Seats {
			seatPlayer := newHand.PlayersInSeats[seat.Seat]
			if seat.Player != "" && seatPlayer.Name != seat.Player {
				errMsg := fmt.Sprintf("Player [%s] should be in seat %d, but found another player: [%s]", seat.Player, seat.Seat, seatPlayer.Name)
				bp.logger.Error().Msg(errMsg)
				panic(errMsg)
			}
			playerStatus := seatPlayer.Status.String()
			if seat.Status != "" && playerStatus != seat.Status {
				errMsg := fmt.Sprintf("Player [%s] Status: %s. Expected Status: %s", seat.Player, seat.Status, seatPlayer.Status)
				bp.logger.Error().Msg(errMsg)
				panic(errMsg)
			}

			if seat.InHand != nil {
				if seatPlayer.Inhand != *seat.InHand {
					errMsg := fmt.Sprintf("Player [%s] inhand is not matching: Expected: %t Actual: %t",
						seat.Player, *seat.InHand, seatPlayer.Inhand)
					bp.logger.Error().Msg(errMsg)
					panic(errMsg)
				}
			}

			if seat.Button != nil {
				if seat.Seat != newHand.ButtonPos {
					errMsg := fmt.Sprintf("Player [%s] button position is not matching: Expected: %d Actual: %d",
						seat.Player, seat.Seat, newHand.ButtonPos)
					bp.logger.Error().Msg(errMsg)
					panic(errMsg)
				}
			}

			if seat.Sb != nil {
				if seat.Seat != newHand.SbPos {
					errMsg := fmt.Sprintf("Player [%s] small blind position is not matching: Expected: %d Actual: %d",
						seat.Player, seat.Seat, newHand.SbPos)
					bp.logger.Error().Msg(errMsg)
					panic(errMsg)
				}
			}

			if seat.Bb != nil {
				if seat.Seat != newHand.BbPos {
					errMsg := fmt.Sprintf("Player [%s] big blind position is not matching: Expected: %d Actual: %d",
						seat.Player, seat.Seat, newHand.BbPos)
					bp.logger.Error().Msg(errMsg)
					panic(errMsg)
				}
			}
		}
	}

	if verify.GameType != "" {
		if newHand.GameType.String() != verify.GameType {
			errMsg := fmt.Sprintf("Game type does not match for hand num: %d. Expected: %s, Actual: %s", newHand.HandNum, verify.GameType, newHand.GameType.String())
			bp.logger.Error().Msg(errMsg)
			panic(errMsg)
		}
	}

	if verify.ButtonPos != nil {
		if newHand.ButtonPos != *verify.ButtonPos {
			errMsg := fmt.Sprintf("Button position does not match for hand num: %d. Expected: %d, Actual %d", newHand.HandNum, *verify.ButtonPos, newHand.ButtonPos)
			bp.logger.Error().Msg(errMsg)
			panic(errMsg)
		}
	}
	if verify.SBPos != nil {
		if newHand.SbPos != *verify.SBPos {
			errMsg := fmt.Sprintf("SB position does not match for hand num: %d. Expected: %d, Actual %d", newHand.HandNum, *verify.SBPos, newHand.SbPos)
			bp.logger.Error().Msg(errMsg)
			panic(errMsg)
		}
	}
	if verify.BBPos != nil {
		if newHand.BbPos != *verify.BBPos {
			errMsg := fmt.Sprintf("BB position does not match for hand num: %d. Expected: %d, Actual %d", newHand.HandNum, *verify.BBPos, newHand.BbPos)
			bp.logger.Error().Msg(errMsg)
			panic(errMsg)
		}
	}
	if verify.NextActionPos != nil {
		if newHand.NextActionSeat != *verify.NextActionPos {
			errMsg := fmt.Sprintf("Next action seat does not match for hand num: %d. Expected: %d, Actual %d", newHand.HandNum, *verify.NextActionPos, newHand.NextActionSeat)
			bp.logger.Error().Msg(errMsg)
			panic(errMsg)
		}
	}
}

/*
func (bp *BotPlayer) verifyResult() {
	// don't verify result for auto play
	if bp.config.Script.AutoPlay {
		return
	}

	if bp.game.handNum == 0 {
		return
	}

	bp.logger.Info().Msgf("%s: Verifying result for hand %d", bp.logPrefix, bp.game.handNum)
	scriptResult := bp.config.Script.GetHand(bp.game.handNum).Result

	actualResult := bp.GetHandResult()
	if actualResult == nil {
		panic(fmt.Sprintf("%s: Hand %d result verify failed. Unable to get the result", bp.logPrefix, bp.game.handNum))
	}

	passed := true

	if scriptResult.ActionEndedAt != "" {
		expectedWonAt := scriptResult.ActionEndedAt
		wonAt := actualResult.GetHandLog().GetWonAt()
		if wonAt.String() != expectedWonAt {
			bp.logger.Error().Msgf("%s: Hand %d result verify failed. Won at: %s. Expected won at: %s.", bp.logPrefix, bp.game.handNum, wonAt, expectedWonAt)
		}
	}

	type winner struct {
		SeatNo  uint32
		Amount  float32
		RankStr string
	}

	if len(scriptResult.Winners) > 0 {
		expectedWinnersBySeat := make(map[uint32]winner)
		for _, expectedWinner := range scriptResult.Winners {
			expectedWinnersBySeat[expectedWinner.Seat] = winner{
				SeatNo:  expectedWinner.Seat,
				Amount:  expectedWinner.Receive,
				RankStr: expectedWinner.RankStr,
			}
		}
		actualWinnersBySeat := make(map[uint32]winner)
		pots := actualResult.GetHandLog().GetPotWinners()
		for _, w := range pots[0].HiWinners {
			actualWinnersBySeat[w.GetSeatNo()] = winner{
				SeatNo:  w.GetSeatNo(),
				Amount:  w.GetAmount(),
				RankStr: w.GetRankStr(),
			}
		}
		if !cmp.Equal(expectedWinnersBySeat, actualWinnersBySeat) {
			bp.logger.Error().Msgf("%s: Hand %d result verify failed. Winners: %v. Expected: %v.", bp.logPrefix, bp.game.handNum, actualWinnersBySeat, expectedWinnersBySeat)
			passed = false
		}
	}

	if len(scriptResult.LoWinners) > 0 {
		expectedLoWinnersBySeat := make(map[uint32]winner)
		for _, expectedWinner := range scriptResult.LoWinners {
			expectedLoWinnersBySeat[expectedWinner.Seat] = winner{
				SeatNo:  expectedWinner.Seat,
				Amount:  expectedWinner.Receive,
				RankStr: expectedWinner.RankStr,
			}
		}
		actualLoWinnersBySeat := make(map[uint32]winner)
		pots := actualResult.GetHandLog().GetPotWinners()
		for _, w := range pots[0].LowWinners {
			actualLoWinnersBySeat[w.GetSeatNo()] = winner{
				SeatNo:  w.GetSeatNo(),
				Amount:  w.GetAmount(),
				RankStr: w.GetRankStr(),
			}
		}

		if !cmp.Equal(expectedLoWinnersBySeat, actualLoWinnersBySeat) {
			bp.logger.Error().Msgf("%s: Hand %d result verify failed. Low Winners: %v. Expected: %v.", bp.logPrefix, bp.game.handNum, actualLoWinnersBySeat, expectedLoWinnersBySeat)
			passed = false
		}
	}

	if len(scriptResult.Players) > 0 {
		resultPlayers := actualResult.GetPlayers()
		for _, scriptResultPlayer := range scriptResult.Players {
			seatNo := scriptResultPlayer.Seat
			if _, exists := resultPlayers[seatNo]; !exists {
				bp.logger.Error().Msgf("%s: Hand %d result verify failed. Expected seat# %d to be found in the result, but the result does not contain that seat.", bp.logPrefix, bp.game.handNum, seatNo)
				passed = false
				continue
			}

			expectedBalanceBefore := scriptResultPlayer.Balance.Before
			if expectedBalanceBefore != nil {
				actualBalanceBefore := resultPlayers[seatNo].GetBalance().Before
				if actualBalanceBefore != *expectedBalanceBefore {
					bp.logger.Error().Msgf("%s: Hand %d result verify failed. Starting balance for seat# %d: %f. Expected: %f.", bp.logPrefix, bp.game.handNum, seatNo, actualBalanceBefore, *expectedBalanceBefore)
					passed = false
				}
			}
			expectedBalanceAfter := scriptResultPlayer.Balance.After
			if expectedBalanceAfter != nil {
				actualBalanceAfter := resultPlayers[seatNo].GetBalance().After
				if actualBalanceAfter != *expectedBalanceAfter {
					bp.logger.Error().Msgf("%s: Hand %d result verify failed. Remaining balance for seat# %d: %f. Expected: %f.", bp.logPrefix, bp.game.handNum, seatNo, actualBalanceAfter, *expectedBalanceAfter)
					passed = false
				}
			}
			expectedHhRank := scriptResultPlayer.HhRank
			if expectedHhRank != nil {
				actualHhRank := resultPlayers[seatNo].GetHhRank()
				if actualHhRank != *expectedHhRank {
					bp.logger.Error().Msgf("%s: Hand %d result verify failed. HhRank for seat# %d: %d. Expected: %d.", bp.logPrefix, bp.game.handNum, seatNo, actualHhRank, *expectedHhRank)
					passed = false
				}
			}
		}
	}

	if len(scriptResult.PlayerStats) > 0 {
		actualPlayerStats := actualResult.GetPlayerStats()
		for _, scriptStat := range scriptResult.PlayerStats {
			seatNo := scriptStat.Seat
			playerID := bp.getPlayerIDBySeatNo(seatNo)
			actualTimeouts := actualPlayerStats[playerID].ConsecutiveActionTimeouts
			expectedTimeouts := scriptStat.ConsecutiveActionTimeouts
			if actualTimeouts != expectedTimeouts {
				bp.logger.Error().Msgf("%s: Hand %d result verify failed. Consecutive Action Timeouts for seat# %d player ID %d: %d. Expected: %d.", bp.logPrefix, bp.game.handNum, seatNo, playerID, actualTimeouts, expectedTimeouts)
				passed = false
			}
			actualActedAtLeastOnce := actualPlayerStats[playerID].ActedAtLeastOnce
			expectedActedAtLeastOnce := scriptStat.ActedAtLeastOnce
			if actualActedAtLeastOnce != expectedActedAtLeastOnce {
				bp.logger.Error().Msgf("%s: Hand %d result verify failed. ActedAtLeastOnce for seat# %d player ID %d: %v. Expected: %v.", bp.logPrefix, bp.game.handNum, seatNo, playerID, actualActedAtLeastOnce, expectedActedAtLeastOnce)
				passed = false
			}
		}
	}

	if len(scriptResult.HighHand) > 0 {
		actualHighHand := actualResult.GetHighHand()
		if actualHighHand == nil {
			bp.logger.Error().Msgf("%s: Hand %d result verify failed. Expected high-hand in result, but got null.")
			passed = false
		}
		hhWinners := actualHighHand.GetWinners()
		if len(hhWinners) != len(scriptResult.HighHand) {
			bp.logger.Error().Msgf("%s: Hand %d result verify failed. Number of high-hand winners: %d. Expected: %d.", bp.logPrefix, bp.game.handNum, len(hhWinners), len(scriptResult.HighHand))
			passed = false
		}
		for _, expectedWinner := range scriptResult.HighHand {
			expectedSeatNo := expectedWinner.Seat
			seatFound := false
			for _, winner := range hhWinners {
				if winner.SeatNo == expectedSeatNo {
					seatFound = true
					break
				}
			}
			if !seatFound {
				bp.logger.Error().Msgf("%s: Hand %d result verify failed. Expected high-hand winner seat# %d was not found in the result high-hand winners.", bp.logPrefix, bp.game.handNum, expectedSeatNo)
				passed = false
			}
		}
	}

	if scriptResult.RunItTwice != nil {
		resultShouldBeNull := scriptResult.RunItTwice.ShouldBeNull
		expectedRunItTwice := !resultShouldBeNull

		if actualResult.GetRunItTwice() != expectedRunItTwice {
			bp.logger.Error().Msgf("%s: Hand %d result verify failed. Run-it-twice in hand result is expected to to be %v, but is %v.", bp.logPrefix, bp.game.handNum, expectedRunItTwice, actualResult.GetRunItTwice())
			passed = false
		}
		if actualResult.GetHandLog().GetRunItTwice() != expectedRunItTwice {
			bp.logger.Error().Msgf("%s: Hand %d result verify failed. Run-it-twice in result hand log is expected to be %v, but is %v.", bp.logPrefix, bp.game.handNum, expectedRunItTwice, actualResult.GetHandLog().GetRunItTwice())
			passed = false
		}
		actualRunItTwiceResult := actualResult.GetHandLog().GetRunItTwiceResult()
		if resultShouldBeNull {
			if actualRunItTwiceResult != nil {
				bp.logger.Error().Msgf("%s: Hand %d result verify failed. The result is not expected to containe run-it-twice result, but it contains not-null run-it-twice section.", bp.logPrefix, bp.game.handNum)
				passed = false
			}
		} else {
			if actualRunItTwiceResult == nil {
				bp.logger.Error().Msgf("%s: Hand %d result verify failed. The result is expected to contain run-it-twice result, but it is null.", bp.logPrefix, bp.game.handNum)
				passed = false
			} else {
				if actualRunItTwiceResult.GetRunItTwiceStartedAt().String() != scriptResult.RunItTwice.StartedAt {
					bp.logger.Error().Msgf("%s: Hand %d result verify failed. runItTwiceStartedAt: %s, expected: %s.", bp.logPrefix, bp.game.handNum, actualRunItTwiceResult.GetRunItTwiceStartedAt().String(), scriptResult.RunItTwice.StartedAt)
					passed = false
				}

				expectedBoard1Pots := scriptResult.RunItTwice.Board1Winners
				actualBoard1Pots := actualRunItTwiceResult.GetBoard_1Winners()
				for potNum, expectedPot := range expectedBoard1Pots {
					actualPot := actualBoard1Pots[uint32(potNum)]
					if actualPot.Amount != expectedPot.Amount {
						bp.logger.Error().Msgf("%s: Hand %d result verify failed. RunItTwice board 1 pot %d amount: %f, expected: %f.", bp.logPrefix, bp.game.handNum, potNum, actualPot.Amount, expectedPot.Amount)
						passed = false
					}
					if !bp.verifyPotWinners(actualPot, expectedPot, 1, potNum) {
						passed = false
					}
				}

				expectedBoard2Pots := scriptResult.RunItTwice.Board2Winners
				actualBoard2Pots := actualRunItTwiceResult.GetBoard_2Winners()
				for potNum, expectedPot := range expectedBoard2Pots {
					actualPot := actualBoard2Pots[uint32(potNum)]
					if actualPot.Amount != expectedPot.Amount {
						bp.logger.Error().Msgf("%s: Hand %d result verify failed. RunItTwice board 2 pot %d amount: %f, expected: %f.", bp.logPrefix, bp.game.handNum, potNum, actualPot.Amount, expectedPot.Amount)
						passed = false
					}
					if !bp.verifyPotWinners(actualPot, expectedPot, 2, potNum) {
						passed = false
					}
				}
			}
		}
	}

	if !passed {
		panic(fmt.Sprintf("Hand %d result verify failed. Please check the logs.", bp.game.handNum))
	}
}
*/

func (bp *BotPlayer) verifyBoardWinners(scriptBoard *gamescript.BoardWinner, actualResult *game.BoardWinner) bool {
	type winner struct {
		SeatNo  uint32
		Amount  float32
		RankStr string
	}
	passed := true
	if len(scriptBoard.BoardWinners.Winners) > 0 {
		expectedWinnersBySeat := make(map[uint32]winner)
		for _, expectedWinner := range scriptBoard.BoardWinners.Winners {
			expectedWinnersBySeat[expectedWinner.Seat] = winner{
				SeatNo:  expectedWinner.Seat,
				Amount:  expectedWinner.Receive,
				RankStr: expectedWinner.RankStr,
			}
		}
		actualWinnersBySeat := make(map[uint32]winner)
		for _, w := range actualResult.HiWinners {
			actualWinnersBySeat[w.GetSeatNo()] = winner{
				SeatNo:  w.GetSeatNo(),
				Amount:  w.GetAmount(),
				RankStr: actualResult.HiRankText,
			}
		}
		if !cmp.Equal(expectedWinnersBySeat, actualWinnersBySeat) {
			bp.logger.Error().Msgf("%s: Hand %d result verify failed. Winners: %v. Expected: %v.", bp.logPrefix, bp.game.handNum, actualWinnersBySeat, expectedWinnersBySeat)
			passed = false
		}
	}
	if len(scriptBoard.BoardWinners.LoWinners) > 0 {
		expectedWinnersBySeat := make(map[uint32]winner)
		for _, expectedWinner := range scriptBoard.BoardWinners.LoWinners {
			expectedWinnersBySeat[expectedWinner.Seat] = winner{
				SeatNo:  expectedWinner.Seat,
				Amount:  expectedWinner.Receive,
				RankStr: "",
			}
		}
		actualWinnersBySeat := make(map[uint32]winner)
		for _, w := range actualResult.LowWinners {
			actualWinnersBySeat[w.GetSeatNo()] = winner{
				SeatNo:  w.GetSeatNo(),
				Amount:  w.GetAmount(),
				RankStr: "",
			}
		}
		if !cmp.Equal(expectedWinnersBySeat, actualWinnersBySeat) {
			bp.logger.Error().Msgf("%s: Hand %d result verify failed. Low Winners: %v. Expected: %v.", bp.logPrefix, bp.game.handNum, actualWinnersBySeat, expectedWinnersBySeat)
			passed = false
		}
	}
	return passed
}

func (bp *BotPlayer) verifyResult2() {
	// don't verify result for auto play
	if bp.config.Script.AutoPlay {
		return
	}

	if bp.game.handNum == 0 {
		return
	}

	bp.logger.Info().Msgf("%s: Verifying result for hand %d", bp.logPrefix, bp.game.handNum)
	scriptResult := bp.config.Script.GetHand(bp.game.handNum).Result

	actualResult := bp.GetHandResult2()
	if actualResult == nil {
		panic(fmt.Sprintf("%s: Hand %d result verify failed. Unable to get the result", bp.logPrefix, bp.game.handNum))
	}

	playerInfo := actualResult.PlayerInfo

	passed := true

	if scriptResult.ActionEndedAt != "" {
		expectedWonAt := scriptResult.ActionEndedAt
		wonAt := actualResult.WonAt
		if wonAt.String() != expectedWonAt {
			bp.logger.Error().Msgf("%s: Hand %d result verify failed. Won at: %s. Expected won at: %s.", bp.logPrefix, bp.game.handNum, wonAt, expectedWonAt)
		}
	}

	if scriptResult.Boards != nil {
		// verify board winners
		// we support only main pot
		for i, testBoardResult := range scriptResult.Boards {
			actualBoard := actualResult.PotWinners[0].BoardWinners[i]
			passed = bp.verifyBoardWinners(&testBoardResult, actualBoard)
		}
	} else {
		type winner struct {
			SeatNo   uint32
			Amount   float32
			RankStr  string
			RakePaid float32
		}
		if len(scriptResult.Winners) > 0 {
			expectedWinnersBySeat := make(map[uint32]winner)
			includeRakePaid := false
			for _, expectedWinner := range scriptResult.Winners {
				expectedWinnersBySeat[expectedWinner.Seat] = winner{
					SeatNo:   expectedWinner.Seat,
					Amount:   expectedWinner.Receive,
					RankStr:  expectedWinner.RankStr,
					RakePaid: 0,
				}
				if expectedWinner.RakePaid != nil {
					includeRakePaid = true
					expectedWinnersBySeat[expectedWinner.Seat] = winner{
						SeatNo:   expectedWinner.Seat,
						Amount:   expectedWinner.Receive,
						RankStr:  expectedWinner.RankStr,
						RakePaid: *expectedWinner.RakePaid,
					}
				}
			}
			actualWinnersBySeat := make(map[uint32]winner)
			pots := actualResult.PotWinners
			pot := pots[0]
			board1 := pot.BoardWinners[0]
			for _, w := range board1.HiWinners {
				player := playerInfo[w.SeatNo]
				actualWinnersBySeat[w.GetSeatNo()] = winner{
					SeatNo:  w.GetSeatNo(),
					Amount:  w.GetAmount(),
					RankStr: board1.HiRankText,
				}
				if includeRakePaid {
					actualWinnersBySeat[w.GetSeatNo()] = winner{
						SeatNo:   w.GetSeatNo(),
						Amount:   w.GetAmount(),
						RankStr:  board1.HiRankText,
						RakePaid: player.RakePaid,
					}
				}
			}
			for seatNo, expectedWinnerBySeat := range expectedWinnersBySeat {
				actualWinnerBySeat := actualWinnersBySeat[seatNo]
				if !cmp.Equal(expectedWinnerBySeat, actualWinnerBySeat) {
					bp.logger.Error().Msgf("%s: Hand %d result verify failed. Winners: %v. Expected: %v.", bp.logPrefix, bp.game.handNum, actualWinnerBySeat, expectedWinnerBySeat)
					passed = false
				}
			}
		}
		if len(scriptResult.LoWinners) > 0 {
			expectedWinnersBySeat := make(map[uint32]winner)
			includeRakePaid := false
			for _, expectedWinner := range scriptResult.LoWinners {
				expectedWinnersBySeat[expectedWinner.Seat] = winner{
					SeatNo:  expectedWinner.Seat,
					Amount:  expectedWinner.Receive,
					RankStr: expectedWinner.RankStr,
				}
				if expectedWinner.RakePaid != nil {
					includeRakePaid = true
					expectedWinnersBySeat[expectedWinner.Seat] = winner{
						SeatNo:   expectedWinner.Seat,
						Amount:   expectedWinner.Receive,
						RankStr:  expectedWinner.RankStr,
						RakePaid: *expectedWinner.RakePaid,
					}
				}
			}
			actualWinnersBySeat := make(map[uint32]winner)
			pots := actualResult.PotWinners
			pot := pots[0]
			board1 := pot.BoardWinners[0]
			for _, w := range board1.LowWinners {
				player := playerInfo[w.SeatNo]
				actualWinnersBySeat[w.GetSeatNo()] = winner{
					SeatNo:  w.GetSeatNo(),
					Amount:  w.GetAmount(),
					RankStr: "",
				}
				if includeRakePaid {
					actualWinnersBySeat[w.GetSeatNo()] = winner{
						SeatNo:   w.GetSeatNo(),
						Amount:   w.GetAmount(),
						RankStr:  "",
						RakePaid: player.RakePaid,
					}
				}
			}
			if !cmp.Equal(expectedWinnersBySeat, actualWinnersBySeat) {
				bp.logger.Error().Msgf("%s: Hand %d result verify failed. Low Winners: %v. Expected: %v.", bp.logPrefix, bp.game.handNum, actualWinnersBySeat, expectedWinnersBySeat)
				passed = false
			}
		}
	}

	if len(scriptResult.Players) > 0 {
		resultPlayers := actualResult.GetPlayerInfo()
		for _, scriptResultPlayer := range scriptResult.Players {
			seatNo := scriptResultPlayer.Seat
			if _, exists := resultPlayers[seatNo]; !exists {
				bp.logger.Error().Msgf("%s: Hand %d result verify failed. Expected seat# %d to be found in the result, but the result does not contain that seat.", bp.logPrefix, bp.game.handNum, seatNo)
				passed = false
				continue
			}

			expectedBalanceBefore := scriptResultPlayer.Balance.Before
			if expectedBalanceBefore != nil {
				actualBalanceBefore := resultPlayers[seatNo].GetBalance().Before
				if actualBalanceBefore != *expectedBalanceBefore {
					bp.logger.Error().Msgf("%s: Hand %d result verify failed. Starting balance for seat# %d: %f. Expected: %f.", bp.logPrefix, bp.game.handNum, seatNo, actualBalanceBefore, *expectedBalanceBefore)
					passed = false
				}
			}
			expectedBalanceAfter := scriptResultPlayer.Balance.After
			if expectedBalanceAfter != nil {
				actualBalanceAfter := resultPlayers[seatNo].GetBalance().After
				if actualBalanceAfter != *expectedBalanceAfter {
					bp.logger.Error().Msgf("%s: Hand %d result verify failed. Remaining balance for seat# %d: %f. Expected: %f.", bp.logPrefix, bp.game.handNum, seatNo, actualBalanceAfter, *expectedBalanceAfter)
					passed = false
				}
			}
			expectedHhRank := scriptResultPlayer.HhRank
			if expectedHhRank != nil {
				actualRank := actualResult.Boards[0].PlayerRank[seatNo].HiRank
				//actualHhRank := resultPlayers[seatNo].GetHhRank()
				if actualRank != *expectedHhRank {
					bp.logger.Error().Msgf("%s: Hand %d result verify failed. HhRank for seat# %d: %d. Expected: %d.", bp.logPrefix, bp.game.handNum, seatNo, actualRank, *expectedHhRank)
					passed = false
				}
			}
		}
	}

	if len(scriptResult.TimeoutStats) > 0 {
		actualPlayerStats := actualResult.GetTimeoutStats()
		for _, scriptStat := range scriptResult.TimeoutStats {
			seatNo := scriptStat.Seat
			playerID := bp.getPlayerIDBySeatNo(seatNo)
			actualTimeouts := actualPlayerStats[playerID].ConsecutiveActionTimeouts
			expectedTimeouts := scriptStat.ConsecutiveActionTimeouts
			if actualTimeouts != expectedTimeouts {
				bp.logger.Error().Msgf("%s: Hand %d result verify failed. Consecutive Action Timeouts for seat# %d player ID %d: %d. Expected: %d.", bp.logPrefix, bp.game.handNum, seatNo, playerID, actualTimeouts, expectedTimeouts)
				passed = false
			}
			actualActedAtLeastOnce := actualPlayerStats[playerID].ActedAtLeastOnce
			expectedActedAtLeastOnce := scriptStat.ActedAtLeastOnce
			if actualActedAtLeastOnce != expectedActedAtLeastOnce {
				bp.logger.Error().Msgf("%s: Hand %d result verify failed. ActedAtLeastOnce for seat# %d player ID %d: %v. Expected: %v.", bp.logPrefix, bp.game.handNum, seatNo, playerID, actualActedAtLeastOnce, expectedActedAtLeastOnce)
				passed = false
			}
		}
	}

	// if len(scriptResult.HighHand) > 0 {
	// 	actualHighHand := actualResult.GetHighHand()
	// 	if actualHighHand == nil {
	// 		bp.logger.Error().Msgf("%s: Hand %d result verify failed. Expected high-hand in result, but got null.")
	// 		passed = false
	// 	}
	// 	hhWinners := actualHighHand.GetWinners()
	// 	if len(hhWinners) != len(scriptResult.HighHand) {
	// 		bp.logger.Error().Msgf("%s: Hand %d result verify failed. Number of high-hand winners: %d. Expected: %d.", bp.logPrefix, bp.game.handNum, len(hhWinners), len(scriptResult.HighHand))
	// 		passed = false
	// 	}
	// 	for _, expectedWinner := range scriptResult.HighHand {
	// 		expectedSeatNo := expectedWinner.Seat
	// 		seatFound := false
	// 		for _, winner := range hhWinners {
	// 			if winner.SeatNo == expectedSeatNo {
	// 				seatFound = true
	// 				break
	// 			}
	// 		}
	// 		if !seatFound {
	// 			bp.logger.Error().Msgf("%s: Hand %d result verify failed. Expected high-hand winner seat# %d was not found in the result high-hand winners.", bp.logPrefix, bp.game.handNum, expectedSeatNo)
	// 			passed = false
	// 		}
	// 	}
	// }

	// if scriptResult.RunItTwice != nil {
	// 	resultShouldBeNull := scriptResult.RunItTwice.ShouldBeNull
	// 	expectedRunItTwice := !resultShouldBeNull

	// 	if actualResult.GetRunItTwice() != expectedRunItTwice {
	// 		bp.logger.Error().Msgf("%s: Hand %d result verify failed. Run-it-twice in hand result is expected to to be %v, but is %v.", bp.logPrefix, bp.game.handNum, expectedRunItTwice, actualResult.GetRunItTwice())
	// 		passed = false
	// 	}
	// 	if actualResult.GetHandLog().GetRunItTwice() != expectedRunItTwice {
	// 		bp.logger.Error().Msgf("%s: Hand %d result verify failed. Run-it-twice in result hand log is expected to be %v, but is %v.", bp.logPrefix, bp.game.handNum, expectedRunItTwice, actualResult.GetHandLog().GetRunItTwice())
	// 		passed = false
	// 	}
	// 	actualRunItTwiceResult := actualResult.GetHandLog().GetRunItTwiceResult()
	// 	if resultShouldBeNull {
	// 		if actualRunItTwiceResult != nil {
	// 			bp.logger.Error().Msgf("%s: Hand %d result verify failed. The result is not expected to containe run-it-twice result, but it contains not-null run-it-twice section.", bp.logPrefix, bp.game.handNum)
	// 			passed = false
	// 		}
	// 	} else {
	// 		if actualRunItTwiceResult == nil {
	// 			bp.logger.Error().Msgf("%s: Hand %d result verify failed. The result is expected to contain run-it-twice result, but it is null.", bp.logPrefix, bp.game.handNum)
	// 			passed = false
	// 		} else {
	// 			if actualRunItTwiceResult.GetRunItTwiceStartedAt().String() != scriptResult.RunItTwice.StartedAt {
	// 				bp.logger.Error().Msgf("%s: Hand %d result verify failed. runItTwiceStartedAt: %s, expected: %s.", bp.logPrefix, bp.game.handNum, actualRunItTwiceResult.GetRunItTwiceStartedAt().String(), scriptResult.RunItTwice.StartedAt)
	// 				passed = false
	// 			}

	// 			expectedBoard1Pots := scriptResult.RunItTwice.Board1Winners
	// 			actualBoard1Pots := actualRunItTwiceResult.GetBoard_1Winners()
	// 			for potNum, expectedPot := range expectedBoard1Pots {
	// 				actualPot := actualBoard1Pots[uint32(potNum)]
	// 				if actualPot.Amount != expectedPot.Amount {
	// 					bp.logger.Error().Msgf("%s: Hand %d result verify failed. RunItTwice board 1 pot %d amount: %f, expected: %f.", bp.logPrefix, bp.game.handNum, potNum, actualPot.Amount, expectedPot.Amount)
	// 					passed = false
	// 				}
	// 				if !bp.verifyPotWinners(actualPot, expectedPot, 1, potNum) {
	// 					passed = false
	// 				}
	// 			}

	// 			expectedBoard2Pots := scriptResult.RunItTwice.Board2Winners
	// 			actualBoard2Pots := actualRunItTwiceResult.GetBoard_2Winners()
	// 			for potNum, expectedPot := range expectedBoard2Pots {
	// 				actualPot := actualBoard2Pots[uint32(potNum)]
	// 				if actualPot.Amount != expectedPot.Amount {
	// 					bp.logger.Error().Msgf("%s: Hand %d result verify failed. RunItTwice board 2 pot %d amount: %f, expected: %f.", bp.logPrefix, bp.game.handNum, potNum, actualPot.Amount, expectedPot.Amount)
	// 					passed = false
	// 				}
	// 				if !bp.verifyPotWinners(actualPot, expectedPot, 2, potNum) {
	// 					passed = false
	// 				}
	// 			}
	// 		}
	// 	}
	// }

	_ = passed
	// if !passed {
	// 	panic(fmt.Sprintf("Hand %d result verify failed. Please check the logs.", bp.game.handNum))
	// }
}

func (bp *BotPlayer) verifyPotWinners(actualPot *game.PotWinners, expectedPot gamescript.WinnerPot, boardNum int, potNum int) bool {
	if actualPot == nil {
		return false
	}
	type winner struct {
		SeatNo  uint32
		Amount  float32
		RankStr string
	}
	passed := true

	expectedHiWinnersBySeat := make(map[uint32]winner)
	for _, expectedWinner := range expectedPot.Winners {
		expectedHiWinnersBySeat[expectedWinner.Seat] = winner{
			SeatNo:  expectedWinner.Seat,
			Amount:  expectedWinner.Receive,
			RankStr: expectedWinner.RankStr,
		}
	}
	actualHiWinnersBySeat := make(map[uint32]winner)
	for _, w := range actualPot.HiWinners {
		actualHiWinnersBySeat[w.GetSeatNo()] = winner{
			SeatNo:  w.GetSeatNo(),
			Amount:  w.GetAmount(),
			RankStr: w.GetRankStr(),
		}
	}
	if !cmp.Equal(expectedHiWinnersBySeat, actualHiWinnersBySeat) {
		bp.logger.Error().Msgf("%s: Hand %d result verify failed. RunItTwice board %d pot %d HI Winners: %v. Expected: %v.", bp.logPrefix, bp.game.handNum, boardNum, potNum, actualHiWinnersBySeat, expectedHiWinnersBySeat)
		passed = false
	}

	expectedLoWinnersBySeat := make(map[uint32]winner)
	for _, expectedWinner := range expectedPot.LoWinners {
		expectedLoWinnersBySeat[expectedWinner.Seat] = winner{
			SeatNo:  expectedWinner.Seat,
			Amount:  expectedWinner.Receive,
			RankStr: expectedWinner.RankStr,
		}
	}
	actualLoWinnersBySeat := make(map[uint32]winner)
	for _, w := range actualPot.LowWinners {
		actualLoWinnersBySeat[w.GetSeatNo()] = winner{
			SeatNo:  w.GetSeatNo(),
			Amount:  w.GetAmount(),
			RankStr: w.GetRankStr(),
		}
	}
	if !cmp.Equal(expectedLoWinnersBySeat, actualLoWinnersBySeat) {
		bp.logger.Error().Msgf("%s: Hand %d result verify failed. RunItTwice board %d pot %d LO Winners: %v. Expected: %v.", bp.logPrefix, bp.game.handNum, boardNum, potNum, actualLoWinnersBySeat, expectedLoWinnersBySeat)
		passed = false
	}

	return passed
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

func (bp *BotPlayer) SignUp() error {
	bp.logger.Info().Msgf("%s: Signing up as a user.", bp.logPrefix)

	type reqData struct {
		ScreenName  string `json:"screen-name"`
		DeviceID    string `json:"device-id"`
		Email       string `json:"email"`
		DisplayName string `json:"display-name"`
		Bot         bool   `json:"bot"`
	}
	reqBody := reqData{
		ScreenName:  bp.config.Name,
		DeviceID:    bp.config.DeviceID,
		Email:       bp.config.Email,
		DisplayName: bp.config.Name,
		Bot:         true,
	}
	jsonValue, _ := json.Marshal(reqBody)
	url := fmt.Sprintf("%s/auth/signup", bp.config.APIServerURL)
	response, err := http.Post(url, "application/json", bytes.NewBuffer(jsonValue))
	if err != nil {
		return errors.Wrap(err, "Login request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("Signup returned %d", response.StatusCode)
	}
	type respData struct {
		DeviceSecret string `json:"device-secret"`
		Jwt          string `json:"jwt"`
		Name         string `json:"name"`
		UUID         string `json:"uuid"`
		ID           uint64 `json:"id"`
	}
	var respBody respData
	data, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return errors.Wrap(err, "Unabgle to read signup response body")
	}
	json.Unmarshal(data, &respBody)
	bp.PlayerID = respBody.ID
	bp.PlayerUUID = respBody.UUID
	bp.apiAuthToken = fmt.Sprintf("jwt %s", respBody.Jwt)
	bp.gqlHelper.SetAuthToken(bp.apiAuthToken)

	encryptionKey, err := bp.getEncryptionKey()
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to get the player encryption key", bp.logPrefix))
	}

	bp.EncryptionKey = encryptionKey
	bp.logger.Info().Msgf("%s: Successfully signed up as a user. Player UUID: [%s] Player ID: [%d].", bp.logPrefix, bp.PlayerUUID, bp.PlayerID)
	return nil
}

// Login authenticates the player and stores its jwt for future api calls.
func (bp *BotPlayer) Login() error {
	type reqData struct {
		DeviceID     string `json:"device-id"`
		DeviceSecret string `json:"device-secret"`
	}
	reqBody := reqData{
		DeviceID:     bp.config.DeviceID,
		DeviceSecret: bp.config.DeviceID,
	}
	jsonValue, _ := json.Marshal(reqBody)
	url := fmt.Sprintf("%s/auth/new-login", bp.config.APIServerURL)
	response, err := http.Post(url, "application/json", bytes.NewBuffer(jsonValue))
	if err != nil {
		return errors.Wrap(err, "Login request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("Login returned %d", response.StatusCode)
	}
	type respData struct {
		Jwt  string `json:"jwt"`
		Name string `json:"name"`
		UUID string `json:"uuid"`
		ID   uint64 `json:"id"`
	}
	var respBody respData
	data, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return errors.Wrap(err, "Unabgle to read login response body")
	}
	json.Unmarshal(data, &respBody)
	bp.PlayerID = respBody.ID
	bp.PlayerUUID = respBody.UUID
	bp.apiAuthToken = fmt.Sprintf("jwt %s", respBody.Jwt)
	bp.gqlHelper.SetAuthToken(bp.apiAuthToken)

	encryptionKey, err := bp.getEncryptionKey()
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to get the player encryption key", bp.logPrefix))
	}

	bp.EncryptionKey = encryptionKey
	bp.logger.Info().Msgf("%s: Successfully logged in.", bp.logPrefix)
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

func (bp *BotPlayer) GetRewardID(clubCode string, name string) (uint32, error) {
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
	return rewardID, err
}

// CreateClubReward creates a new club reward.
func (bp *BotPlayer) CreateClubReward(clubCode string, name string, rewardType string, scheduleType string, amount float32) (uint32, error) {
	bp.logger.Info().Msgf("%s: Creating a new club reward [%s].", bp.logPrefix, name)
	rewardID, err := bp.GetRewardID(clubCode, name)

	if rewardID == 0 {
		rewardID, err = bp.gqlHelper.CreateClubReward(clubCode, name, rewardType, scheduleType, amount)
		if err != nil {
			return 0, errors.Wrap(err, fmt.Sprintf("%s: Unable to create a new club", bp.logPrefix))
		}
	}
	bp.RewardsNameToID[name] = rewardID
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
func (bp *BotPlayer) Subscribe(gameToAllSubjectName string, handToAllSubjectName string, handToPlayerSubjectName string, playerChannelName string, pingSubjectName string) error {
	if bp.gameMsgSubscription == nil || !bp.gameMsgSubscription.IsValid() {
		bp.logger.Info().Msgf("%s: Subscribing to %s to receive game messages sent to players/observers", bp.logPrefix, gameToAllSubjectName)
		gameToAllSub, err := bp.natsConn.Subscribe(gameToAllSubjectName, bp.handleGameMsg)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("%s: Unable to subscribe to the game message subject [%s]", bp.logPrefix, gameToAllSubjectName))
		}
		bp.gameMsgSubscription = gameToAllSub
		bp.logger.Info().Msgf("%s: Successfully subscribed to %s.", bp.logPrefix, gameToAllSubjectName)
	}

	if bp.handMsgAllSubscription == nil || !bp.handMsgAllSubscription.IsValid() {
		bp.logger.Info().Msgf("%s: Subscribing to %s to receive hand messages sent to players/observers", bp.logPrefix, handToAllSubjectName)
		handToAllSub, err := bp.natsConn.Subscribe(handToAllSubjectName, bp.handleHandMsg)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("%s: Unable to subscribe to the hand message subject [%s]", bp.logPrefix, handToAllSubjectName))
		}
		bp.handMsgAllSubscription = handToAllSub
		bp.logger.Info().Msgf("%s: Successfully subscribed to %s.", bp.logPrefix, handToAllSubjectName)
	}

	if bp.handMsgPlayerSubscription == nil || !bp.handMsgPlayerSubscription.IsValid() {
		bp.logger.Info().Msgf("%s: Subscribing to %s to receive hand messages sent to player: %s", bp.logPrefix, handToPlayerSubjectName, bp.config.Name)
		handToPlayerSub, err := bp.natsConn.Subscribe(handToPlayerSubjectName, bp.handlePrivateHandMsg)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("%s: Unable to subscribe to the hand message subject [%s]", bp.logPrefix, handToPlayerSubjectName))
		}
		bp.handMsgPlayerSubscription = handToPlayerSub
		bp.logger.Info().Msgf("%s: Successfully subscribed to %s.", bp.logPrefix, handToPlayerSubjectName)
	}

	if bp.playerMsgPlayerSubscription == nil || !bp.playerMsgPlayerSubscription.IsValid() {
		bp.logger.Info().Msgf("%s: Subscribing to %s to receive hand messages sent to player: %s", bp.logPrefix, handToPlayerSubjectName, bp.config.Name)
		sub, err := bp.natsConn.Subscribe(playerChannelName, bp.handlePlayerPrivateMsg)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("%s: Unable to subscribe to the hand message subject [%s]", bp.logPrefix, handToPlayerSubjectName))
		}
		bp.playerMsgPlayerSubscription = sub
		bp.logger.Info().Msgf("%s: Successfully subscribed to %s.", bp.logPrefix, handToPlayerSubjectName)
	}

	if bp.pingSubscription == nil || !bp.pingSubscription.IsValid() {
		bp.logger.Info().Msgf("%s: Subscribing to %s to receive ping messages sent to player: %s", bp.logPrefix, pingSubjectName, bp.config.Name)
		pingToPlayerSub, err := bp.natsConn.Subscribe(pingSubjectName, bp.handlePingMsg)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("%s: Unable to subscribe to the ping message subject [%s]", bp.logPrefix, pingSubjectName))
		}
		bp.pingSubscription = pingToPlayerSub
		bp.logger.Info().Msgf("%s: Successfully subscribed to %s.", bp.logPrefix, pingSubjectName)
	}

	bp.event(BotEvent__SUBSCRIBE)

	return nil
}

// unsubscribe makes the bot unsubscribe from the nats subjects.
func (bp *BotPlayer) unsubscribe() error {
	var errMsg string
	if bp.gameMsgSubscription != nil {
		err := bp.gameMsgSubscription.Unsubscribe()
		if err != nil {
			errMsg = fmt.Sprintf("Error [%s] while unsubscribing from subject [%s]", err, bp.gameMsgSubscription.Subject)
		}
		bp.gameMsgSubscription = nil
	}
	if bp.handMsgAllSubscription != nil {
		err := bp.handMsgAllSubscription.Unsubscribe()
		if err != nil {
			errMsg = fmt.Sprintf("%s Error [%s] while unsubscribing from subject [%s]", errMsg, err, bp.handMsgAllSubscription.Subject)
		}
		bp.handMsgAllSubscription = nil
	}
	if bp.handMsgPlayerSubscription != nil {
		err := bp.handMsgPlayerSubscription.Unsubscribe()
		if err != nil {
			errMsg = fmt.Sprintf("%s Error [%s] while unsubscribing from subject [%s]", errMsg, err, bp.handMsgPlayerSubscription.Subject)
		}
		bp.handMsgPlayerSubscription = nil
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
			actionTracker: game.NewHandActionTracker(),
			playersActed:  make(map[uint32]*game.PlayerActRound),
		},
	}
	playerChannelName := fmt.Sprintf("player.%d", bp.PlayerID)
	err = bp.Subscribe(gi.GameToPlayerChannel, gi.HandToAllChannel, gi.HandToPlayerChannel, playerChannelName, gi.PingChannel)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to subscribe to game %s channels", bp.logPrefix, gameCode))
	}

	bp.gameToAll = gi.GameToPlayerChannel
	bp.handToAll = gi.HandToAllChannel
	bp.handToMe = gi.HandToPlayerChannel
	bp.meToHand = gi.PlayerToHandChannel
	bp.pingSubjectName = gi.PingChannel
	bp.pongSubjectName = gi.PongChannel

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
func (bp *BotPlayer) JoinGame(gameCode string, gps *gamescript.GpsLocation) error {
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
		} else {

		}

		bp.event(BotEvent__REQUEST_SIT)

		err := bp.SitIn(gameCode, scriptSeatNo, gps)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("%s: Unable to sit in", bp.logPrefix))
		}

		if bp.IsHuman() {
			bp.logger.Info().Msgf("%s: Press ENTER to buy in [%f] chips...", bp.logPrefix, scriptBuyInAmount)
			bufio.NewReader(os.Stdin).ReadBytes('\n')
		} else {
			// update player config
			scriptSeatConfig := bp.config.Script.GetSeatConfigByPlayerName(bp.config.Name)
			if scriptSeatConfig != nil {
				bp.UpdatePlayerGameConfig(gameCode, scriptSeatConfig.RunItTwice, &scriptSeatConfig.MuckLosingHand)
			}
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

func (bp *BotPlayer) NewPlayer(gameCode string, startingSeat *gamescript.StartingSeat) error {
	bp.logger.Info().Msgf("%s: Player joining the game next hand.", bp.logPrefix)
	bp.observing = true
	scriptSeatNo := startingSeat.Seat
	if scriptSeatNo == 0 {
		return fmt.Errorf("%s: Unable to get the scripted seat number", bp.logPrefix)
	}
	scriptBuyInAmount := startingSeat.BuyIn
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
		} else {

		}

		bp.event(BotEvent__REQUEST_SIT)
		// set ip address if found
		if startingSeat.IpAddress != nil {
			bp.gqlHelper.IpAddress = *startingSeat.IpAddress
		}
		err := bp.SitIn(gameCode, scriptSeatNo, startingSeat.Gps)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("%s: Unable to sit in", bp.logPrefix))
		}

		if bp.IsHuman() {
			bp.logger.Info().Msgf("%s: Press ENTER to buy in [%f] chips...", bp.logPrefix, scriptBuyInAmount)
			bufio.NewReader(os.Stdin).ReadBytes('\n')
		} else {
			// update player config
			scriptSeatConfig := bp.config.Script.GetSeatConfigByPlayerName(bp.config.Name)
			if scriptSeatConfig != nil {
				bp.UpdatePlayerGameConfig(gameCode, nil, &scriptSeatConfig.MuckLosingHand)
			}
		}
		bp.buyInAmount = uint32(scriptBuyInAmount)
		err = bp.BuyIn(gameCode, scriptBuyInAmount)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("%s: Unable to buy in", bp.logPrefix))
		}

		bp.event(BotEvent__SUCCEED_BUYIN)

		// post blind
		if startingSeat.PostBlind {
			bp.logger.Info().Msgf("%s: Posted blind", bp.logPrefix)
			bp.gqlHelper.PostBlind(bp.gameCode)
		}
	}

	bp.seatNo = scriptSeatNo
	bp.balance = scriptBuyInAmount
	bp.updateLogPrefix()

	return nil
}

func (bp *BotPlayer) reload() error {
	seatConfig := bp.config.Script.GetSeatConfigByPlayerName(bp.config.Name)
	if seatConfig == nil {
		return nil
	}
	if seatConfig.Reload != nil && *seatConfig.Reload == false {
		// This player is explicitly set to not reload.
		return nil
	}

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
	bp.scriptedGame = false
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

	err = bp.SitIn(gameCode, seatNo, nil)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to sit in", bp.logPrefix))
	}
	buyInAmt := bp.gameInfo.BuyInMax
	err = bp.BuyIn(gameCode, buyInAmt)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to buy in", bp.logPrefix))
	}
	bp.seatNo = seatNo

	// unscripted game, bots will run it twice
	if bp.gameInfo.RunItTwiceAllowed {
		t := true
		bp.UpdatePlayerGameConfig(gameCode, &t, &t)
	}

	bp.event(BotEvent__SUCCEED_BUYIN)

	return nil
}

// SitIn takes a seat in a game as a player.
func (bp *BotPlayer) SitIn(gameCode string, seatNo uint32, gps *gamescript.GpsLocation) error {
	bp.logger.Info().Msgf("%s: Grabbing seat [%d] in game [%s].", bp.logPrefix, seatNo, gameCode)
	status, err := bp.gqlHelper.SitIn(gameCode, seatNo, gps)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to sit in game [%s]", bp.logPrefix, gameCode))
	}

	bp.observing = false
	bp.inWaitList = false
	bp.seatNo = seatNo
	bp.isSeated = true
	bp.updateLogPrefix()
	bp.logger.Info().Msgf("%s: Successfully took a seat in game [%s]. Status: [%s]", bp.logPrefix, gameCode, status)
	return nil
}

// BuyIn is where you buy the chips once seated in a game.
func (bp *BotPlayer) BuyIn(gameCode string, amount float32) error {
	//bp.logger.Info().Msgf("%s: Buying in amount [%f].", bp.logPrefix, amount)

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

// LeaveGameImmediately makes the bot leave the game.
func (bp *BotPlayer) LeaveGameImmediately() error {
	bp.logger.Info().Msgf("%s: Leaving game [%s].", bp.logPrefix, bp.gameCode)
	err := bp.unsubscribe()
	if err != nil {
		return errors.Wrap(err, "Error while unsubscribing from NATS subjects")
	}
	if bp.isSeated && !bp.hasSentLeaveGameRequest {
		_, err = bp.gqlHelper.LeaveGame(bp.gameCode)
		if err != nil {
			return errors.Wrap(err, "Error while making a GQL request to leave game")
		}
	}
	go func() {
		bp.end <- true
		bp.endPing <- true
	}()
	bp.isSeated = false
	bp.gameCode = ""
	bp.gameID = 0
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

func (bp *BotPlayer) getEncryptionKey() (string, error) {
	encryptionKey, err := bp.gqlHelper.GetEncryptionKey()
	if err != nil {
		return "", errors.Wrapf(err, "%s: Unable to get encryption key", bp.logPrefix)
	}
	return encryptionKey, nil
}

// StartGame starts the game.
func (bp *BotPlayer) StartGame(gameCode string) error {
	bp.logger.Info().Msgf("%s: Starting the game [%s].", bp.logPrefix, gameCode)

	// setup first deck if not auto play
	if bp.IsHost() && !bp.config.Script.AutoPlay {
		err := bp.setupNextHand()
		if err != nil {
			return errors.Wrapf(err, "%s: Unable to setup next hand", bp.logPrefix)
		}
		// Wait for the first hand to be setup before starting the game.
		time.Sleep(500 * time.Millisecond)
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
	messageId := fmt.Sprintf("%d:QUERY_CURRENT_HAND:%d", bp.PlayerID, time.Now())
	// query current hand state
	msg := game.HandMessage{
		GameCode:  bp.gameCode,
		PlayerId:  bp.PlayerID,
		MessageId: messageId,
		//GameToken: 	 bp.GameToken,
		Messages: []*game.HandMessageItem{
			{
				MessageType: game.HandQueryCurrentHand,
			},
		},
	}
	protoData, err := proto.Marshal(&msg)
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

// UpdatePlayerGameConfig updates player's configuration for this game.
func (bp *BotPlayer) UpdatePlayerGameConfig(gameCode string, runItTwiceAllowed *bool, muckLosingHand *bool) error {
	bp.logger.Info().Msgf("%s: Updating player configuration [runItTwiceAllowed: %v, muckLosingHand: %v] game [%s].",
		bp.logPrefix, runItTwiceAllowed, muckLosingHand, gameCode)
	err := bp.gqlHelper.UpdatePlayerGameConfig(gameCode, runItTwiceAllowed, muckLosingHand)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to update game config [%s]", bp.logPrefix, gameCode))
	}
	if muckLosingHand != nil {
		bp.muckLosingHand = *muckLosingHand
	}
	return nil
}

func (bp *BotPlayer) doesScriptActionExists(scriptHand gamescript.Hand, handStatus game.HandStatus) bool {
	var scriptActions []gamescript.SeatAction
	switch handStatus {
	case game.HandStatus_PREFLOP:
		scriptActions = scriptHand.Preflop.SeatActions
	case game.HandStatus_FLOP:
		scriptActions = scriptHand.Flop.SeatActions
	case game.HandStatus_TURN:
		scriptActions = scriptHand.Turn.SeatActions
	case game.HandStatus_RIVER:
		scriptActions = scriptHand.River.SeatActions
	default:
		panic(fmt.Sprintf("Invalid hand status [%s] in doesScriptActionExists", handStatus))
	}
	return scriptActions != nil && len(scriptActions) > 0
}

func (bp *BotPlayer) act(seatAction *game.NextSeatAction) {
	availableActions := seatAction.AvailableActions
	nextAction := game.ACTION_CHECK
	nextAmt := float32(0)
	autoPlay := false

	if bp.config.Script.AutoPlay {
		autoPlay = true
	} else if len(bp.config.Script.Hands) >= int(bp.game.handNum) {
		handScript := bp.config.Script.GetHand(bp.game.handNum)
		if !bp.doesScriptActionExists(handScript, bp.game.handStatus) {
			autoPlay = true
		}
	}
	runItTwiceActionPrompt := false
	timeout := false
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
			if action == game.ACTION_RUN_IT_TWICE_PROMPT {
				runItTwiceActionPrompt = true
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

		// this section is to simulate run it twice scenario
		// if the game is configured to run it twice and the current bot is the last one to act
		// go all in
		// get human players (there should be only one)
		// get the human player action
		// if the human player action is ALLIN, check how many players are active
		// if more than 2 players are active, then fold
		// if only two players are active, then the remaining bot will go all in
		if bp.gameInfo.RunItTwiceAllowed {
			players := bp.humanPlayers()
			if len(players) == 1 {
				player := players[0]
				// human player action
				action := bp.game.table.playersActed[player.seatNo]
				if action != nil && action.Action == game.ACTION_ALLIN {
					activePlayers := bp.activePlayers()
					if len(activePlayers) == 2 {
						// last remaining bot
						nextAction = game.ACTION_ALLIN
						nextAmt = allInAmount
					} else {
						// these bots should go all in
						nextAction = game.ACTION_FOLD
						nextAmt = 0
					}

				}
			}
			for _, action := range availableActions {
				if action == game.ACTION_RUN_IT_TWICE_PROMPT {
					runItTwiceActionPrompt = true
				}
			}
		}
	} else {

		for _, action := range availableActions {
			if action == game.ACTION_RUN_IT_TWICE_PROMPT {
				runItTwiceActionPrompt = true
			}
		}
		if !runItTwiceActionPrompt {
			var err error
			scriptAction, err := bp.decision.GetNextAction(bp, availableActions)
			if scriptAction.Action.Seat != bp.seatNo {
				bp.logger.Panic().Msgf("%s: Bot seatNo (%d) != script seatNo (%d)", bp.logPrefix, bp.seatNo, scriptAction.Action.Seat)
			}
			if err != nil {
				bp.logger.Error().Msgf("%s: Unable to get the next action %+v", bp.logPrefix, err)
				return
			}
			nextAction = game.ActionStringToAction(scriptAction.Action.Action)
			nextAmt = scriptAction.Action.Amount
			timeout = scriptAction.Timeout
			preActions := scriptAction.PreActions
			bp.processPreActions(preActions)
		}
	}

	if bp.IsHuman() {
		bp.logger.Info().Msgf("%s: Seat %d: Your Turn. Press ENTER to continue with [%s %f] (Hand Status: %s)...", bp.logPrefix, bp.seatNo, nextAction, nextAmt, bp.game.handStatus)
		bufio.NewReader(os.Stdin).ReadBytes('\n')
	}

	if !bp.IsHuman() && !util.Env.ShouldDisableDelays() {
		// Pause to think for some time to be realistic.
		time.Sleep(bp.getActionTime())
	}
	var handAction game.HandAction
	runItTwiceTimeout := false
	if runItTwiceActionPrompt {
		runItTwiceConf := bp.getRunItTwiceConfig()
		if runItTwiceConf.Confirm {
			handAction = game.HandAction{
				SeatNo: bp.seatNo,
				Action: game.ACTION_RUN_IT_TWICE_YES,
			}
		} else {
			handAction = game.HandAction{
				SeatNo: bp.seatNo,
				Action: game.ACTION_RUN_IT_TWICE_NO,
			}
		}
		if runItTwiceConf.Timeout {
			runItTwiceTimeout = true
		}
	} else {
		handAction = game.HandAction{
			SeatNo: bp.seatNo,
			Action: nextAction,
			Amount: nextAmt,
		}
	}
	msgType := game.HandPlayerActed
	lastMsgIDInt, err := strconv.Atoi(bp.clientLastMsgID)
	if err != nil {
		panic(fmt.Sprintf("Unable to convert message ID to int: %v", err))
	}
	msgID := strconv.Itoa(lastMsgIDInt + 1)
	actionMsg := game.HandMessage{
		GameCode:  bp.gameCode,
		HandNum:   bp.game.handNum,
		PlayerId:  bp.PlayerID,
		SeatNo:    bp.seatNo,
		MessageId: msgID,
		Messages: []*game.HandMessageItem{
			{
				MessageType: msgType,
				Content:     &game.HandMessageItem_PlayerActed{PlayerActed: &handAction},
			},
		},
	}

	playerName := bp.getPlayerNameBySeatNo(bp.seatNo)
	if timeout || runItTwiceTimeout {
		go func() {
			if runItTwiceTimeout {
				bp.logger.Info().Msgf("%s: Seat %d (%s) is going to time out the run-it-twice prompt. Stage: %s.", bp.logPrefix, bp.seatNo, playerName, bp.game.handStatus)
			} else {
				bp.logger.Info().Msgf("%s: Seat %d (%s) is going to time out. Stage: %s.", bp.logPrefix, bp.seatNo, playerName, bp.game.handStatus)
			}
			// sleep more than action time
			time.Sleep(time.Duration(bp.config.Script.Game.ActionTime) * time.Second)
			time.Sleep(2 * time.Second)
		}()
	} else {
		bp.logger.Info().Msgf("%s: Seat %d (%s) is about to act [%s %f]. Stage: %s.", bp.logPrefix, bp.seatNo, playerName, handAction.Action, handAction.Amount, bp.game.handStatus)
		go bp.publishAndWaitForAck(bp.meToHand, &actionMsg)
	}
}

func (bp *BotPlayer) getActionTime() time.Duration {
	randomMilli := util.GetRandomUint32(bp.config.MinActionPauseTime, bp.config.MaxActionPauseTime)
	return time.Duration(randomMilli) * time.Millisecond
}

func (bp *BotPlayer) publishAndWaitForAck(subj string, msg *game.HandMessage) {
	protoData, err := proto.Marshal(msg)
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
		if attempts > bp.maxRetry {
			var errMsg string
			if !published {
				errMsg = fmt.Sprintf("%s: Retry (%d) exhausted while publishing message type: %s, message ID: %s", bp.logPrefix, bp.maxRetry, game.HandPlayerActed, msg.GetMessageId())
			} else {
				errMsg = fmt.Sprintf("%s: Retry (%d) exhausted while waiting for game server acknowledgement for message type: %s, message ID: %s", bp.logPrefix, bp.maxRetry, game.HandPlayerActed, msg.GetMessageId())
			}
			bp.logger.Error().Msg(errMsg)
			bp.errorStateMsg = errMsg
			bp.sm.SetState(BotState__ERROR)
			return
		}
		if attempts > 1 {
			bp.logger.Info().Msgf("%s: Attempt (%d) to publish message type: %s, message ID: %s", bp.logPrefix, attempts, game.HandPlayerActed, msg.GetMessageId())
		}
		if err := bp.natsConn.Publish(bp.meToHand, protoData); err != nil {
			bp.logger.Error().Msgf("%s: Error [%s] while publishing message %+v", bp.logPrefix, err, msg)
			time.Sleep(2 * time.Second)
			continue
		}
		if !published {
			bp.sm.Event(BotEvent__SEND_MY_ACTION)
			bp.clientLastMsgID = msg.GetMessageId()
			bp.clientLastMsgType = game.HandPlayerActed
			published = true
		}
		time.Sleep(2 * time.Second)
		if bp.sm.Current() != BotState__ACTED_WAITING_FOR_ACK {
			ackReceived = true
		} else if bp.clientLastMsgID != msg.GetMessageId() {
			// Bots are acting very fast. This bot is already waiting for the ack for the next action.
			ackReceived = true
		}
	}
}

func (bp *BotPlayer) rememberPlayerAction(seatNo uint32, action game.ACTION, amount float32, timedOut bool, handStatus game.HandStatus) {
	bp.game.table.actionTracker.RecordAction(seatNo, action, amount, timedOut, handStatus)
	bp.game.table.playersActed[seatNo] = &game.PlayerActRound{
		Action: action,
		Amount: amount,
	}
}

func (bp *BotPlayer) getLastActedSeatFromTracker() uint32 {
	actionHistory := bp.game.table.actionTracker.GetActions(bp.game.handStatus)
	if len(actionHistory) == 0 {
		return 0
	}
	return actionHistory[len(actionHistory)-1].SeatNo
}

func (bp *BotPlayer) activePlayers() []uint32 {
	activePlayers := make([]uint32, 0)
	for seatNo, act := range bp.game.table.playersActed {
		if act.Action != game.ACTION_FOLD {
			activePlayers = append(activePlayers, seatNo)
		}
	}
	return activePlayers
}

func (bp *BotPlayer) humanPlayers() []*player {
	players := make([]*player, 0)
	for _, player := range bp.game.table.playersBySeat {
		if !player.isBot {
			players = append(players, player)
		}
	}
	return players
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
	return bp.game.handResult
}

// GetHandResult2 returns the hand result received from the server.
func (bp *BotPlayer) GetHandResult2() *game.HandResultClient {
	return bp.game.handResult2
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

func (bp *BotPlayer) setupServerCrash(crashPoint string, playerID uint64) error {
	type payload struct {
		GameCode   string `json:"gameCode"`
		CrashPoint string `json:"crashPoint"`
		PlayerID   uint64 `json:"playerId"`
	}
	data := payload{
		GameCode:   bp.gameCode,
		CrashPoint: crashPoint,
		PlayerID:   playerID,
	}
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return errors.Wrap(err, "Unable to marshal payload")
	}
	url := fmt.Sprintf("%s/setup-crash", util.Env.GetGameServerURL(bp.gameCode))

	bp.logger.Info().Msgf("%s: Setting up game server crash. URL: %s, Payload: %s", bp.logPrefix, url, jsonBytes)
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonBytes))
	if err != nil {
		return errors.Wrap(err, "Post failed")
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Game server returned http %d: %s", resp.StatusCode, string(body))
	}
	bp.logger.Info().Msgf("%s: Successfully setup game server crash.", bp.logPrefix)
	return nil
}

func (bp *BotPlayer) getPlayerInSeat(seatNo uint32) *game.SeatInfo {
	for _, p := range bp.gameInfo.SeatInfo.PlayersInSeats {
		if p.SeatNo == seatNo {
			return &p
		}
	}
	return nil
}

func (bp *BotPlayer) AddNewPlayers(setup *gamescript.HandSetup) error {
	var err error
	for _, startingSeat := range setup.NewPlayers {
		playerName := startingSeat.Player
		var botByName *BotPlayer
		for _, bot := range bp.bots {
			if bot.config.Name == playerName {
				botByName = bot
				break
			}
		}
		if botByName != nil {
			// new player joining the table
			err = botByName.NewPlayer(bp.gameCode, &startingSeat)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (bp *BotPlayer) setupNextHand() error {
	nextHandNum := bp.game.handNum + 1

	if int(nextHandNum) > len(bp.config.Script.Hands) {
		return nil
	}

	nextHand := bp.config.Script.GetHand(nextHandNum)

	bp.AddNewPlayers(&nextHand.Setup)

	bp.processPreDealItems(nextHand.Setup.PreDeal)

	if nextHand.Setup.ButtonPos != 0 {
		err := bp.setupButtonPos(nextHand.Setup.ButtonPos)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("Unable to set button position"))
		}
	}

	// setup deck
	var setupDeckMsg *SetupDeck
	if nextHand.Setup.Auto {
		setupDeckMsg = &SetupDeck{
			MessageType: BotDriverSetupDeck,
			GameCode:    bp.gameCode,
			GameID:      bp.gameID,
			Auto:        true,
			Pause:       nextHand.Setup.Pause,
		}
		if nextHand.Setup.ButtonPos != 0 {
			setupDeckMsg.ButtonPos = nextHand.Setup.ButtonPos
		}
	} else {
		setupDeckMsg = &SetupDeck{
			MessageType:     BotDriverSetupDeck,
			Pause:           nextHand.Setup.Pause,
			GameCode:        bp.gameCode,
			GameID:          bp.gameID,
			ButtonPos:       nextHand.Setup.ButtonPos,
			Board:           nextHand.Setup.Board,
			Board2:          nextHand.Setup.Board2,
			Flop:            nextHand.Setup.Flop,
			Turn:            nextHand.Setup.Turn,
			River:           nextHand.Setup.River,
			PlayerCards:     bp.getPlayerCardsFromConfig(nextHand.Setup.SeatCards),
			ResultPauseTime: nextHand.Setup.ResultPauseTime,
		}

		if nextHand.Setup.BombPot {
			setupDeckMsg.BombPot = true
			setupDeckMsg.BombPotBet = uint32(nextHand.Setup.BombPotBet)
			setupDeckMsg.DoubleBoard = nextHand.Setup.DoubleBoard
		}

	}
	setupDeckMsg.IncludeStatsInResult = true
	msgBytes, err := jsoniter.Marshal(setupDeckMsg)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Unable to marshal SetupDeck BotDriverSetupDeck message %+v", setupDeckMsg))
	}
	bp.natsConn.Publish(util.GetDriverToGameSubject(), msgBytes)
	return nil
}

func (bp *BotPlayer) getPlayerCardsFromConfig(seatCards []gamescript.SeatCards) []PlayerCard {
	var playerCards []PlayerCard
	for _, seatCard := range seatCards {
		playerCards = append(playerCards, PlayerCard{
			Seat:  seatCard.Seat,
			Cards: seatCard.Cards,
		})
	}
	return playerCards
}

func (bp *BotPlayer) reloadBotFromGameInfo() error {
	bp.game.table.playersBySeat = make(map[uint32]*player)
	gameInfo, err := bp.GetGameInfo(bp.gameCode)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Unable to get game info %s", bp.logPrefix, bp.gameCode))
	}
	bp.gameInfo = &gameInfo
	var seatNo uint32
	var isSeated bool
	var isPlaying bool
	for _, p := range gameInfo.SeatInfo.PlayersInSeats {
		pl := &player{
			playerID: p.PlayerId,
			seatNo:   p.SeatNo,
			status:   game.PlayerStatus(game.PlayerStatus_value[p.Status]),
			stack:    p.Stack,
			buyIn:    p.BuyIn,
			isBot:    p.IsBot,
		}
		bp.game.table.playersBySeat[p.SeatNo] = pl
		if p.PlayerUUID == bp.PlayerUUID {
			isSeated = true
			seatNo = p.SeatNo
			if pl.status == game.PlayerStatus_PLAYING {
				isPlaying = true
			}
		}
	}
	if isSeated {
		bp.isSeated = true
		bp.seatNo = seatNo
	} else {
		bp.isSeated = false
		bp.seatNo = 0
	}

	bp.observing = true
	if isPlaying {
		bp.observing = false
	}

	bp.updateLogPrefix()

	return nil
}

func (bp *BotPlayer) isGamePaused() (bool, error) {
	gi, err := bp.GetGameInfo(bp.gameCode)
	if err != nil {
		return false, errors.Wrap(err, fmt.Sprintf("%s: Unable to get game info %s", bp.logPrefix, bp.gameCode))
	}
	if gi.Status == game.GameStatus_ENDED.String() {
		return false, fmt.Errorf("%s: Game ended %s. Unexpected", bp.logPrefix, bp.gameCode)
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

func (bp *BotPlayer) getPlayerIDBySeatNo(seatNo uint32) uint64 {
	for _, p := range bp.gameInfo.SeatInfo.PlayersInSeats {
		if p.SeatNo == seatNo {
			return p.PlayerId
		}
	}
	return 0
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

// HostRequestSeatChange schedules to end the game after the current hand is finished.
func (bp *BotPlayer) HostRequestSeatChange(gameCode string) error {
	bp.logger.Info().Msgf("%s: Host is requesting to make seat changes in game [%s].", bp.logPrefix, gameCode)

	status, err := bp.gqlHelper.HostRequestSeatChange(gameCode)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Error while host is requesting to make seat changes  [%s]", bp.logPrefix, gameCode))
	}

	bp.logger.Info().Msgf("%s: Successfully requested to make seat changes. Status: [%s]", bp.logPrefix, gameCode, status)
	return nil
}

// CreateClub creates a new club.
func (bp *BotPlayer) ResetDB() error {
	bp.logger.Info().Msgf("%s: Resetting database for testing [%s].", bp.logPrefix)

	err := bp.gqlHelper.ResetDB()
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s: Resetting database failed", bp.logPrefix))
	}

	return nil
}
