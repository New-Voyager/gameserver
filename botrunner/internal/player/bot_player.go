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
	"sort"
	"strconv"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
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
	"voyager.com/botrunner/internal/networkcheck"
	"voyager.com/botrunner/internal/poker"
	"voyager.com/botrunner/internal/rest"
	"voyager.com/botrunner/internal/util"
	"voyager.com/encryption"
	"voyager.com/gamescript"
	"voyager.com/logging"
)

// Config holds the configuration for a bot object.
type Config struct {
	Name            string
	DeviceID        string
	Email           string
	Password        string
	IsHuman         bool
	IsObserver      bool
	IsHost          bool
	MinActionDelay  uint32
	MaxActionDelay  uint32
	APIServerURL    string
	NatsURL         string
	GQLTimeoutSec   int
	Gps             *gamescript.GpsLocation
	IpAddress       string
	Players         *gamescript.Players
	Script          *gamescript.Script
	IsTournamentBot bool
}

type GameMessageChannelItem struct {
	ProtoGameMsg    *game.GameMessage
	NonProtoGameMsg *gamescript.NonProtoMessage
}

type TournamentMessageChannelItem struct {
	NonProtoMsg *gamescript.NonProtoTournamentMsg
}

// BotPlayer represents a bot user.
type BotPlayer struct {
	logger  *zerolog.Logger
	logFile *os.File
	config  Config

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

	// tournament flag
	tournament                  bool
	tournamentID                uint64
	tournamentInfo              game.TournamentInfo
	tournamentTableInfo         game.TournamentTableInfo
	tournamentSeatNo            uint32
	tournamentTableNo           uint32
	needsTournamentTableRefresh bool

	// initial seat information (used for determining whether bot or human)
	seatInfo map[uint32]game.SeatInfo // initial seat info (used in auto play games)

	// state of the bot
	sm *fsm.FSM

	// current status
	buyInAmount uint32
	havePair    bool
	pairCard    uint32
	balance     float64
	currentRank string

	// For message acknowledgement
	clientLastMsgID   string
	clientLastMsgType string
	maxRetry          int

	// bots in game
	bots []*BotPlayer

	// Remember the most recent message ID's for deduplicating server messages.
	serverLastMsgIDs *util.Queue

	// Message channels
	chGame       chan *GameMessageChannelItem
	chHand       chan *game.HandMessage
	chHandText   chan *gamescript.HandTextMessage
	chTournament chan *TournamentMessageChannelItem
	end          chan bool
	endPing      chan bool

	// GameInfo received from the api server.
	gameInfo *game.GameInfo

	// wait list variables
	inWaitList      bool
	confirmWaitlist bool

	// Nats subjects
	meToHandSubjectName    string
	clientAliveSubjectName string
	tournamentChannelName  string

	// Nats subscription objects
	gameMsgSubscription             *natsgo.Subscription
	handMsgAllSubscription          *natsgo.Subscription
	handMsgPlayerSubscription       *natsgo.Subscription
	handMsgPlayerTextSubscription   *natsgo.Subscription
	playerMsgPlayerSubscription     *natsgo.Subscription
	tournamentMsgSubscription       *natsgo.Subscription
	tournamentPlayerMsgSubscription *natsgo.Subscription

	game      *gameView
	seatNo    uint32
	observing bool // if a player is playing, then he is also an observer

	// Print nats messages for debugging.
	printGameMsg       bool
	printHandMsg       bool
	printStateMsg      bool
	printTournamentMsg bool

	decision ScriptBasedDecision

	hasNextHandBeenSetup bool // For host only

	// The bot wants to leave after the current hand and has sent the
	// leaveGame request to the api server.
	hasSentLeaveGameRequest bool

	// Error msg if the bot is in an error state (BotState__ERROR).
	errorStateMsg string

	// messages received in the player/private channel
	PrivateMessages     []map[string]interface{}
	PrivateTextMessages []*gamescript.HandTextMessage
	GameMessages        []*gamescript.NonProtoMessage

	// tournament messages
	TournamentMessages []map[string]interface{}

	// For periodically notifying server the client is alive
	clientAliveCheck *networkcheck.ClientAliveCheck
}

// NewBotPlayer creates an instance of BotPlayer.
func NewBotPlayer(playerConfig Config, logFile *os.File) (*BotPlayer, error) {
	logger := logging.GetZeroLogger("BotPlayer", logFile).With().
		Str(logging.PlayerNameKey, playerConfig.Name).Logger()
	logger.Info().Msgf("Bot player connecting to NATS URL: %s", playerConfig.NatsURL)
	nc, err := natsgo.Connect(playerConfig.NatsURL)
	if err != nil {
		logger.Error().Err(err).Msgf("Could not connect to NATS server at %s", playerConfig.NatsURL)
		return nil, errors.Wrap(err, fmt.Sprintf("Error connecting to NATS server [%s]", playerConfig.NatsURL))
	}

	gqlClient := graphql.NewClient(util.GetGqlURL(playerConfig.APIServerURL))
	gqlHelper := gql.NewGQLHelper(gqlClient, uint32(playerConfig.GQLTimeoutSec), "")
	restHelper := rest.NewRestClient(util.GetInternalRestURL(playerConfig.APIServerURL), uint32(playerConfig.GQLTimeoutSec), "")

	bp := BotPlayer{
		logger:             &logger,
		logFile:            logFile,
		config:             playerConfig,
		PlayerUUID:         playerConfig.DeviceID,
		gqlHelper:          gqlHelper,
		restHelper:         restHelper,
		natsConn:           nc,
		chGame:             make(chan *GameMessageChannelItem, 10),
		chHand:             make(chan *game.HandMessage, 10),
		chHandText:         make(chan *gamescript.HandTextMessage, 10),
		chTournament:       make(chan *TournamentMessageChannelItem, 10),
		end:                make(chan bool),
		endPing:            make(chan bool),
		printGameMsg:       util.Env.ShouldPrintGameMsg(),
		printHandMsg:       util.Env.ShouldPrintHandMsg(),
		printStateMsg:      util.Env.ShouldPrintStateMsg(),
		printTournamentMsg: util.Env.ShouldPrintTournamentMsg(),
		RewardsNameToID:    make(map[string]uint32),
		maxRetry:           300,
		tournament:         playerConfig.IsTournamentBot,
	}
	bp.UpdateLogger()

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
			{
				Name: BotEvent__UNSUBSCRIBE,
				Src: []string{
					BotState__OBSERVING,
					BotState__JOINING,
					BotState__REJOINING,
					BotState__WAITING_FOR_MY_TURN,
					BotState__ACTED_WAITING_FOR_ACK,
					BotState__MY_TURN,
				},
				Dst: BotState__NOT_IN_GAME,
			},
		},
		fsm.Callbacks{
			"enter_state": func(e *fsm.Event) { bp.enterState(e) },
		},
	)
	return &bp, nil
}

func (bp *BotPlayer) Reset() {
	if bp.sm.Current() != BotState__NOT_IN_GAME {
		panic("Can't reset while in game")
	}
	bp.gameCode = ""
	bp.gameID = 0
	bp.seatNo = 0
	bp.clientLastMsgID = "0"
	bp.serverLastMsgIDs = util.NewQueue(10)
	bp.seatInfo = nil
	bp.buyInAmount = 0
	bp.havePair = false
	bp.pairCard = 0
	bp.balance = 0
	bp.gameInfo = nil
	bp.inWaitList = false
	bp.confirmWaitlist = false
	bp.meToHandSubjectName = ""
	bp.clientAliveSubjectName = ""
	bp.gameMsgSubscription = nil
	bp.gameMsgSubscription = nil
	bp.handMsgAllSubscription = nil
	bp.handMsgPlayerSubscription = nil
	bp.handMsgPlayerTextSubscription = nil
	bp.playerMsgPlayerSubscription = nil
	bp.tournamentMsgSubscription = nil
	bp.tournamentPlayerMsgSubscription = nil
	bp.game = nil
	bp.observing = false
	bp.hasNextHandBeenSetup = false
	bp.hasSentLeaveGameRequest = false
	bp.errorStateMsg = ""
	bp.PrivateMessages = make([]map[string]interface{}, 0)
	bp.GameMessages = make([]*gamescript.NonProtoMessage, 0)
	bp.PrivateTextMessages = make([]*gamescript.HandTextMessage, 0)
	bp.tournamentID = 0
	bp.needsTournamentTableRefresh = false
	bp.currentRank = ""
	bp.UpdateLogger()

	go bp.messageLoop()
}

func (bp *BotPlayer) getLogPrefix() string {
	if bp.config.IsHuman {
		return fmt.Sprintf("Player [%s:%d:%d]", bp.config.Name, bp.PlayerID, bp.seatNo)
	} else {
		return fmt.Sprintf("Bot [%s:%d:%d]", bp.config.Name, bp.PlayerID, bp.seatNo)
	}
}

func (bp *BotPlayer) FormatLogMsg(msg interface{}) string {
	return fmt.Sprintf("%s: %s", bp.getLogPrefix(), msg)
}

func (bp *BotPlayer) UpdateLogger() {
	gameID := bp.gameID
	gameCode := bp.gameCode
	output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339, FormatMessage: bp.FormatLogMsg}
	newLogger := bp.logger.Output(output).With().
		Uint64(logging.GameIDKey, gameID).
		Str(logging.GameCodeKey, gameCode).
		Logger()
	bp.logger = &newLogger
}

func (bp *BotPlayer) SetBotsInGame(bots []*BotPlayer) {
	bp.bots = bots
}

func (bp *BotPlayer) enterState(e *fsm.Event) {
	if bp.printStateMsg {
		bp.logger.Info().Msgf("[%s] ===> [%s]", e.Src, e.Dst)
	}

	// if e.Dst == BotState__MY_TURN {
	// 	if bp.clientAliveCheck == nil {
	// 		bp.logger.Error().Msgf("Entered %s state, but clientAliveCheck is nil", BotState__MY_TURN)
	// 	} else {
	// 		bp.clientAliveCheck.InAction()
	// 	}
	// } else {
	// 	if bp.clientAliveCheck != nil {
	// 		bp.clientAliveCheck.NotInAction()
	// 	}
	// }
}

func (bp *BotPlayer) event(event string) error {
	if bp.tournament {
		// skip the event check temporarily for tournament bots
		return nil
	}
	err := bp.sm.Event(event)
	if err != nil {
		bp.logger.Warn().Msgf("Error from state machine: %s", err.Error())
	}
	return err
}

func (bp *BotPlayer) SetIPAddress(ipAddress string) {
	bp.config.IpAddress = ipAddress
	bp.gqlHelper.IpAddress = ipAddress
}

func (bp *BotPlayer) SetGpsLocation(gps *gamescript.GpsLocation) {
	bp.config.Gps = gps
}

func (bp *BotPlayer) handleGameMsg(msg *natsgo.Msg) {
	if bp.printGameMsg {
		bp.logger.Info().Msgf("Received game message %s", string(msg.Data))
	}

	var message game.GameMessage
	var nonProtoMsg gamescript.NonProtoMessage
	err := protojson.Unmarshal(msg.Data, &message)
	if err != nil {
		// bp.logger.Debug().Msgf("Error [%s] while unmarshalling protobuf game message [%s]. Assuming non-protobuf message", err, string(msg.Data))
		err = json.Unmarshal(msg.Data, &nonProtoMsg)
		if err != nil {
			bp.logger.Error().Msgf("Error [%s] while unmarshalling non-protobuf game message [%s]", err, string(msg.Data))
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
			bp.logger.Error().Msgf("Error [%s] while decrypting private hand message", err)
			return
		}
		data = decryptedMsg
	}

	bp.unmarshalAndQueueHandMsg(data)
}

func (bp *BotPlayer) handlePrivateHandTextMsg(msg *natsgo.Msg) {
	var message gamescript.HandTextMessage
	data := msg.Data
	if util.Env.ShouldPrintHandMsg() {
		bp.logger.Debug().Msgf("Received hand msg (text): %s\n", string(data))
	}

	err := json.Unmarshal(msg.Data, &message)
	if err != nil {
		panic(fmt.Sprintf("Unable to unmarshal hand text msg. Msg: %s", string(msg.Data)))
	}

	bp.PrivateTextMessages = append(bp.PrivateTextMessages, &message)
	bp.chHandText <- &message
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
		bp.logger.Error().Msgf("Error [%s] while unmarshalling protobuf hand message [%s]", err, string(data))
		return
	}

	if util.Env.ShouldPrintHandMsg() {
		fmt.Printf("Received hand msg (proto): %s\n", message.String())
	}

	bp.chHand <- &message
}

func (bp *BotPlayer) messageLoop() {
	for {
		select {
		case <-bp.end:
			return
		case message := <-bp.chTournament:
			bp.processTournamentMessage(message)
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
		case message := <-bp.chHandText:
			bp.processHandTextMessage(message)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (bp *BotPlayer) processHandTextMessage(message *gamescript.HandTextMessage) {
	if bp.IsErrorState() {
		bp.logger.Info().Msgf("Bot is in error state. Ignoring hand message.")
		return
	}

	if message.MessageId == "" {
		bp.logger.Panic().Msgf("Hand message from server is missing message ID. Message: %s", message.MessageType)
	}

	if bp.config.Script.AutoPlay.Enabled {
		choices := message.DealerChoiceGames
		nextGameIdx := rand.Intn(len(choices))
		nextGame := game.GameType(choices[nextGameIdx])
		bp.chooseNextGame(nextGame)
		return
	}

	currentHand := bp.config.Script.GetHand(message.HandNum)
	if message.MessageType == game.HandDealerChoice {
		if currentHand.Setup.DealerChoice != nil {
			// choose a game from the list
			if bp.seatNo != currentHand.Setup.DealerChoice.Seat {
				errMsg := fmt.Sprintf("Seat No[%d] should be choosing dealer choice, but found another seat: [%d]", currentHand.Setup.DealerChoice.Seat, bp.seatNo)
				bp.logger.Error().Msg(errMsg)
				panic(errMsg)
			}
			gameType := game.GameType_UNKNOWN
			switch currentHand.Setup.DealerChoice.Choice {
			case "HOLDEM":
				gameType = game.GameType_HOLDEM
			case "PLO":
				gameType = game.GameType_PLO
			case "PLO_HILO":
				gameType = game.GameType_PLO_HILO
			case "FIVE_CARD_PLO":
				gameType = game.GameType_FIVE_CARD_PLO
			case "FIVE_CARD_PLO_HILO":
				gameType = game.GameType_FIVE_CARD_PLO_HILO
			case "SIX_CARD_PLO":
				gameType = game.GameType_SIX_CARD_PLO
			case "SIX_CARD_PLO_HILO":
				gameType = game.GameType_SIX_CARD_PLO_HILO
			}

			bp.logger.Info().
				Msgf("Submitting dealer choice for hand %d: %s", message.HandNum, gameType)
			_, err := bp.gqlHelper.DealerChoice(bp.gameCode, gameType)
			if err != nil {
				errMsg := fmt.Sprintf("Error submitting dealer choice: %s", err)
				bp.logger.Error().Msg(errMsg)
				panic(errMsg)
			}
		}
	}
}

func (bp *BotPlayer) processHandMessage(message *game.HandMessage) {
	if bp.IsErrorState() {
		bp.logger.Info().Msgf("Bot is in error state. Ignoring hand message.")
		return
	}

	if message.MessageId == "" {
		bp.logger.Panic().Msgf("Hand message from server is missing message ID. Message: %s", message.String())
	}

	if bp.serverLastMsgIDs.Contains(message.MessageId) {
		// Duplicate message potentially due to server restart. Ignore it.
		bp.logger.Info().Msgf("Ignoring duplicate hand message ID: %s", message.MessageId)
		return
	}
	bp.serverLastMsgIDs.Push(message.MessageId)

	if message.PlayerId != 0 && message.PlayerId != bp.PlayerID {
		// drop this message
		// this message was targeted for another player
		return
	}

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
		bp.logger.Debug().Msgf("Received cards: %s (%+v)", poker.CardsToString(cards), cards)

	case game.HandDealerChoice:
		dealerChoice := msgItem.GetDealerChoice()
		bp.logger.Info().Msgf("Dealer choice games: (%+v)", dealerChoice.Games)
		nextGameIdx := rand.Intn(len(dealerChoice.Games))
		nextGame := dealerChoice.Games[nextGameIdx]
		bp.chooseNextGame(nextGame)

	case game.HandNewHand:
		/* MessageType: NEW_HAND */
		bp.game.table.playersActed = make(map[uint32]*game.PlayerActRound)
		bp.game.handNum = message.HandNum
		bp.game.handStatus = message.GetHandStatus()
		newHand := msgItem.GetNewHand()
		bp.reloadBotFromGameInfo(newHand)
		bp.game.table.buttonPos = newHand.GetButtonPos()
		bp.game.table.sbPos = newHand.GetSbPos()
		bp.game.table.bbPos = newHand.GetBbPos()
		bp.game.table.nextActionSeat = newHand.GetNextActionSeat()
		bp.game.table.actionTracker = game.NewHandActionTracker()
		bp.currentRank = ""

		bp.hasNextHandBeenSetup = false // Not this hand, but the next one.

		if bp.needsTournamentTableRefresh {
			bp.refreshTournamentTableInfo()
		}
		if bp.IsHost() {
			data, _ := protojson.Marshal(message)
			bp.logger.Debug().Msgf("A new hand is started. Hand Num: %d, message: %s", message.HandNum, string(data))
			bp.logger.Info().Msgf("New Hand. Hand Num: %d", message.HandNum)
			if bp.config.Script.AutoPlay.Enabled {
				handsPerGame := bp.config.Script.AutoPlay.HandsPerGame
				if handsPerGame != 0 && message.HandNum >= handsPerGame {
					err := bp.RequestEndGame(bp.gameCode)
					if err != nil {
						bp.logger.Error().Msgf("Could not schedule to end game: %s", err.Error())
					}
				}
			} else {
				if int(message.HandNum) == len(bp.config.Script.Hands) {
					bp.logger.Info().Msgf("Last hand: %d Game will be ended in next hand", message.HandNum)

					// The host bot should schedule to end the game after this hand is over.
					err := bp.RequestEndGame(bp.gameCode)
					if err != nil {
						bp.logger.Error().Msgf("Could not schedule to end game: %s", err.Error())
					}
				}
			}
			bp.pauseGameIfNeeded()

			if !bp.config.Script.AutoPlay.Enabled {
				bp.verifyNewHand(message.HandNum, newHand)
			}
		}

		// update seat number
		for seatNo, player := range newHand.PlayersInSeats {
			if player.PlayerId == bp.PlayerID {
				bp.balance = player.Stack
				if bp.seatNo == 0 {
					bp.updateSeatNo(seatNo)
				} else if bp.seatNo != seatNo {
					bp.logger.Info().Msgf("Player: %s changed seat from %d to %d", player.Name, bp.seatNo, seatNo)
					bp.updateSeatNo(seatNo)
				}
				break
			}
		}
		if !bp.tournament {
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

			if bp.balance == 0 {
				err := bp.autoReloadBalance()
				if err != nil {
					errMsg := fmt.Sprintf("Could not reload chips when balance is 0. Current hand num: %d. Error: %v", bp.game.handNum, err)
					bp.logger.Error().Msg(errMsg)
				}
			}
		}

	case game.HandFlop:
		/* MessageType: FLOP */
		bp.game.handStatus = message.GetHandStatus()
		bp.game.table.flopCards = msgItem.GetFlop().GetBoard()
		bp.logger.Debug().Msgf("Flop cards shown: %s Rank: %v", msgItem.GetFlop().GetCardsStr(), bp.decryptRankStr(msgItem.GetFlop().PlayerCardRanks[bp.seatNo]))
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
		bp.logger.Debug().Msgf("Turn cards shown: %s Rank: %v", msgItem.GetTurn().GetCardsStr(), bp.decryptRankStr(msgItem.GetTurn().PlayerCardRanks[bp.seatNo]))
		bp.verifyBoard()
		bp.verifyCardRank(msgItem.GetTurn().GetPlayerCardRanks())
		bp.updateBalance(msgItem.GetTurn().GetPlayerBalance())
		//time.Sleep(1 * time.Second)
		bp.game.table.playersActed = make(map[uint32]*game.PlayerActRound)

	case game.HandRiver:
		/* MessageType: RIVER */
		bp.game.handStatus = message.GetHandStatus()
		bp.game.table.riverCards = msgItem.GetRiver().GetBoard()
		bp.logger.Debug().Msgf("River cards shown: %s Rank: %v", msgItem.GetRiver().GetCardsStr(), bp.decryptRankStr(msgItem.GetRiver().PlayerCardRanks[bp.seatNo]))
		bp.verifyBoard()
		bp.verifyCardRank(msgItem.GetRiver().GetPlayerCardRanks())
		bp.updateBalance(msgItem.GetRiver().GetPlayerBalance())
		//time.Sleep(1 * time.Second)
		bp.game.table.playersActed = make(map[uint32]*game.PlayerActRound)

	case game.HandYourAction:
		/* MessageType: YOUR_ACTION */
		seatAction := msgItem.GetSeatAction()
		seatNo := seatAction.GetSeatNo()
		if seatNo != bp.seatNo {
			// It's not my turn.
			bp.clientAliveCheck.NotInAction()
			break
		}
		bp.clientAliveCheck.InAction()
		err := bp.event(BotEvent__RECEIVE_YOUR_ACTION)
		if err != nil {
			// State transition failed due to unexpected YOUR_ACTION message. Possible cause is game server sent a duplicate
			// YOUR_ACTION message as part of the crash recovery. Ignore the message.
			bp.logger.Info().Msgf("Ignoring unexpected %s message.", game.HandYourAction)
			break
		}
		bp.game.handStatus = message.GetHandStatus()
		if bp.IsObserver() && bp.config.Script.IsSeatHuman(seatNo) {
			bp.logger.Info().Msgf("Waiting on seat %d (%s/human) to act.", seatNo, bp.getPlayerNameBySeatNo(seatNo))
		}
		bp.act(seatAction, message.GetHandStatus())

	case game.HandPlayerActed:
		/* MessageType: PLAYER_ACTED */
		if bp.IsHost() && !bp.config.Script.AutoPlay.Enabled && !bp.hasNextHandBeenSetup {
			// We're just using this message as a signal that the betting
			// round is in progress and we are now ready to setup the next hand.
			err := bp.setupNextHand()
			if err != nil {
				errMsg := fmt.Sprintf("Could not setup next hand. Current hand num: %d. Error: %v", bp.game.handNum, err)
				bp.logger.Error().Msg(errMsg)
				bp.errorStateMsg = errMsg
				bp.sm.SetState(BotState__ERROR)
				return
			}
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
			bp.logger.Debug().Msgf("Seat %d (%s/%s) acted%s [%s %v] Stage:%s.", seatNo, actedPlayerName, actedPlayerType, timedout, action, amount, bp.game.handStatus)
		}
		if bp.IsHuman() && seatNo != bp.seatNo {
			// I'm a human and I see another player acted.
			bp.logger.Debug().Msgf("Seat %d: %s %v%s", seatNo, action, amount, timedout)
		}
		if seatNo == bp.seatNo && isTimedOut {
			bp.event(BotEvent__ACTION_TIMEDOUT)
		}
		if seatNo == bp.seatNo && !bp.tournament && !bp.config.Script.AutoPlay.Enabled {
			// verify the player action
			verify, err := bp.decision.GetPrevActionToVerify(bp)
			if err == nil {
				if verify != nil {
					bp.logger.Info().Msg("Verify previous action")
					if verify.Stack != playerActed.Stack {
						bp.logger.Panic().Msgf("Hand %d Seat No: %d verify seat action failed. Player stack: %v expected: %v",
							bp.game.handNum, bp.seatNo, playerActed.Stack, verify.Stack)
					}

					// TODO: enable this later
					// if verify.PotUpdates != playerActed.PotUpdates {
					// 	bp.logger.Panic().Msgf("Hand %d Seat No: %d  verify seat action failed. Pot updates: %v expected: %v",
					// 		bp.game.handNum, bp.seatNo, playerActed.PotUpdates, verify.PotUpdates)
					// }
				}
			}
		}

	case game.HandMsgAck:
		/* MessageType: MSG_ACK */
		msgType := msgItem.GetMsgAck().GetMessageType()
		msgID := msgItem.GetMsgAck().GetMessageId()
		msg := fmt.Sprintf("Ignoring unexpected %s msg - %s:%s BotState: %s, CurrentMsgType: %s, CurrentMsgID: %s", game.HandMsgAck, msgType, msgID, bp.sm.Current(), bp.clientLastMsgType, bp.clientLastMsgID)
		if msgType != bp.clientLastMsgType {
			bp.logger.Info().Msg(msg)
			return
		}
		if msgID != bp.clientLastMsgID {
			bp.logger.Info().Msg(msg)
			return
		}

		// On rare occasions when running many bot games (100+) in the same botrunner server, I see that
		// BotEvent__RECEIVE_ACK results in a state error because it is processed before BotEvent__SEND_MY_ACTION.
		// "Ignoring unexpected MSG_ACK msg - PLAYER_ACTED:114 BotState: MY_TURN"
		// Adding a sleep here to yield this goroutine so that the other event gets processed first.
		time.Sleep(5 * time.Millisecond)
		err := bp.event(BotEvent__RECEIVE_ACK)
		if err != nil {
			bp.logger.Info().Msg(msg)
		}

	case game.HandResultMessage2:
		/* MessageType: RESULT */
		bp.game.handStatus = message.GetHandStatus()
		bp.game.handResult2 = msgItem.GetHandResultClient()
		if bp.IsObserver() {
			bp.PrintHandResult()
			bp.verifyResult2()
			bp.verifyAPIRespForHand()
		}

	case game.HandEnded:
		if bp.IsHost() {
			bp.logger.Info().Msgf("Hand Num: %d ended", message.HandNum)

			// process post hand steps if specified
			bp.processPostHandSteps()
		}
		if bp.hasSentLeaveGameRequest {
			bp.LeaveGameImmediately()
		}
		bp.logger.Debug().Msgf("IsHost: %v handNum: %d ended", bp.IsHost(), message.HandNum)

	case game.HandQueryCurrentHand:
		currentState := msgItem.GetCurrentHandState()
		bp.logger.Info().Msgf("Received current hand state: %+v", currentState)
		if message.HandNum == 0 {
			bp.logger.Info().Msgf("Ignoring current hand state message (handNum = 0)")
			return
		}
		if message.HandNum < bp.game.handNum {
			bp.logger.Info().Msgf("Ignoring current hand state message (message handNum = %d, hand in progress = %d)", message.HandNum, bp.game.handNum)
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
			bp.act(nextSeatAction, handStatus)
		}
	}
}

func (bp *BotPlayer) chooseNextGame(gameType game.GameType) {
	bp.gqlHelper.DealerChoice(bp.gameCode, gameType)
}

func (bp *BotPlayer) verifyBoard() {
	if bp.tournament {
		return
	}
	// if the script is configured to auto play, return
	if bp.config.Script.AutoPlay.Enabled {
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
		bp.logger.Panic().Msgf("Hand %d %s verify failed. Board does not match the expected. Current board: %v. Expected board: %v.", bp.game.handNum, bp.game.handStatus, currentBoardCards, expectedBoardCards)
	}
}

func (bp *BotPlayer) updateBalance(playerBalances map[uint32]float64) {
	balance, exists := playerBalances[bp.seatNo]
	if exists {
		bp.balance = balance
	}
}

func (bp *BotPlayer) verifyCardRank(currentRanks map[uint32]string) {
	actualRank, exists := currentRanks[bp.seatNo]
	if exists {
		if util.Env.IsEncryptionEnabled() {
			// Player rank string is encrypted and base64 encoded by the game server.
			// It first needs to be b64 decoded and then decrypted using the player's
			// encryption key.
			actualRank = bp.decryptRankStr(actualRank)
		}
		bp.currentRank = actualRank
	}

	if bp.tournament {
		return
	}

	// if the script is configured to auto play, return
	if bp.config.Script.AutoPlay.Enabled {
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

	if actualRank != expectedRank {
		bp.logger.Panic().Msgf("Hand %d %s verify failed. Seat %d rank string does not match the expected. Current rank: %s. Expected rank: %s.", bp.game.handNum, bp.game.handStatus, bp.seatNo, actualRank, expectedRank)
	}
}

func (bp *BotPlayer) decryptRankStr(rankStr string) string {
	if !util.Env.IsEncryptionEnabled() {
		return rankStr
	}

	decodedRankStr, err := encryption.B64DecodeString(rankStr)
	if err != nil {
		bp.logger.Panic().Msgf("Unable to decode player rank string %s", rankStr)
	}
	decrypted, err := encryption.DecryptWithUUIDStrKey(decodedRankStr, bp.EncryptionKey)
	if err != nil {
		bp.logger.Panic().Msgf("Error [%s] while decrypting private hand message", err)
	}
	return string(decrypted)
}

func (bp *BotPlayer) verifyNewHand(handNum uint32, newHand *game.NewHand) {
	currentHand := bp.config.Script.GetHand(handNum)
	verify := currentHand.Setup.Verify
	if len(verify.Seats) > 0 {
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
			if seat.MissedBlind != nil {
				if seatPlayer.MissedBlind != *seat.MissedBlind {
					errMsg := fmt.Sprintf("Player [%s] missed blind is not matching: Expected: %t Actual: %t",
						seatPlayer.Name, *seat.MissedBlind, seatPlayer.MissedBlind)
					bp.logger.Error().Msg(errMsg)
					panic(errMsg)
				}
			}
			if seat.Stack != nil {
				if seatPlayer.Stack != *seat.Stack {
					errMsg := fmt.Sprintf("Player [%s] stack is not matching: Expected: %v Actual: %v",
						seatPlayer.Name, *seat.Stack, seatPlayer.Stack)
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

func (bp *BotPlayer) verifyBoardWinners(scriptBoard *gamescript.BoardWinner, actualResult *game.BoardWinner) bool {
	type winner struct {
		SeatNo  uint32
		Amount  float64
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
			bp.logger.Error().Msgf("Hand %d result verify failed. Winners: %v. Expected: %v.", bp.game.handNum, actualWinnersBySeat, expectedWinnersBySeat)
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
			bp.logger.Error().Msgf("Hand %d result verify failed. Low Winners: %v. Expected: %v.", bp.game.handNum, actualWinnersBySeat, expectedWinnersBySeat)
			passed = false
		}
	}
	return passed
}

func (bp *BotPlayer) verifyResult2() {
	// don't verify result for auto play
	if bp.config.Script.AutoPlay.Enabled {
		return
	}

	if bp.game.handNum == 0 {
		return
	}

	bp.logger.Info().Msgf("Verifying result for hand %d", bp.game.handNum)
	scriptResult := bp.config.Script.GetHand(bp.game.handNum).Result

	actualResult := bp.GetHandResult2()
	if actualResult == nil {
		panic(fmt.Sprintf("Hand %d result verify failed. Unable to get the result", bp.game.handNum))
	}

	playerInfo := actualResult.PlayerInfo

	passed := true

	if scriptResult.ActionEndedAt != "" {
		expectedWonAt := scriptResult.ActionEndedAt
		wonAt := actualResult.WonAt
		if wonAt.String() != expectedWonAt {
			bp.logger.Error().Msgf("Hand %d result verify failed. Won at: %s. Expected won at: %s.", bp.game.handNum, wonAt, expectedWonAt)
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
			Amount   float64
			RankStr  string
			RakePaid float64
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
					bp.logger.Error().Msgf("Hand %d result verify failed. Winners: %v. Expected: %v.", bp.game.handNum, actualWinnerBySeat, expectedWinnerBySeat)
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
				bp.logger.Error().Msgf("Hand %d result verify failed. Low Winners: %v. Expected: %v.", bp.game.handNum, actualWinnersBySeat, expectedWinnersBySeat)
				passed = false
			}
		}
	}

	if len(scriptResult.Players) > 0 {
		resultPlayers := actualResult.GetPlayerInfo()
		for _, scriptResultPlayer := range scriptResult.Players {
			seatNo := scriptResultPlayer.Seat
			if _, exists := resultPlayers[seatNo]; !exists {
				bp.logger.Error().Msgf("Hand %d result verify failed. Expected seat# %d to be found in the result, but the result does not contain that seat.", bp.game.handNum, seatNo)
				passed = false
				continue
			}

			expectedBalanceBefore := scriptResultPlayer.Balance.Before
			if expectedBalanceBefore != nil {
				actualBalanceBefore := resultPlayers[seatNo].GetBalance().Before
				if actualBalanceBefore != *expectedBalanceBefore {
					bp.logger.Error().Msgf("Hand %d result verify failed. Starting balance for seat# %d: %v. Expected: %v.", bp.game.handNum, seatNo, actualBalanceBefore, *expectedBalanceBefore)
					passed = false
				}
			}
			expectedBalanceAfter := scriptResultPlayer.Balance.After
			if expectedBalanceAfter != nil {
				actualBalanceAfter := resultPlayers[seatNo].GetBalance().After
				if actualBalanceAfter != *expectedBalanceAfter {
					bp.logger.Error().Msgf("Hand %d result verify failed. Remaining balance for seat# %d: %v. Expected: %v.", bp.game.handNum, seatNo, actualBalanceAfter, *expectedBalanceAfter)
					passed = false
				}
			}
			expectedHhRank := scriptResultPlayer.HhRank
			if expectedHhRank != nil {
				actualRank := actualResult.Boards[0].PlayerRank[seatNo].HhRank
				//actualHhRank := resultPlayers[seatNo].GetHhRank()
				if actualRank != *expectedHhRank {
					bp.logger.Error().Msgf("Hand %d result verify failed. HhRank for seat# %d: %d. Expected: %d.", bp.game.handNum, seatNo, actualRank, *expectedHhRank)
					passed = false
				}
			}
			expectedPotContribution := scriptResultPlayer.PotContribution
			if expectedPotContribution != nil {
				actualContribution := resultPlayers[seatNo].GetPotContribution()
				if actualContribution != *expectedPotContribution {
					bp.logger.Error().Msgf("Hand %d result verify failed. PotContribution for seat# %d: %v. Expected: %v.", bp.game.handNum, seatNo, actualContribution, *expectedPotContribution)
					passed = false
				}
			}
		}
	}

	if scriptResult.TipsCollected != nil {
		var expectedTips float64 = *scriptResult.TipsCollected
		var actualTips float64 = actualResult.GetTipsCollected()
		if actualTips != expectedTips {
			bp.logger.Error().Msgf("Hand %d result verify failed. Tips collected: %v, Expected: %v", bp.game.handNum, actualTips, expectedTips)
			passed = false
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
				bp.logger.Error().Msgf("Hand %d result verify failed. Consecutive Action Timeouts for seat# %d player ID %d: %d. Expected: %d.", bp.game.handNum, seatNo, playerID, actualTimeouts, expectedTimeouts)
				passed = false
			}
			actualActedAtLeastOnce := actualPlayerStats[playerID].ActedAtLeastOnce
			expectedActedAtLeastOnce := scriptStat.ActedAtLeastOnce
			if actualActedAtLeastOnce != expectedActedAtLeastOnce {
				bp.logger.Error().Msgf("Hand %d result verify failed. ActedAtLeastOnce for seat# %d player ID %d: %v. Expected: %v.", bp.game.handNum, seatNo, playerID, actualActedAtLeastOnce, expectedActedAtLeastOnce)
				passed = false
			}
		}
	}

	if len(scriptResult.HighHands) > 0 {
		if len(scriptResult.HighHands) != len(actualResult.HighHandWinners) {
			bp.logger.Error().Msgf("Hand %d result verify failed. High hand winners expected: %d actual: %d", bp.game.handNum, len(scriptResult.HighHands), len(actualResult.HighHandWinners))
			passed = false
		} else {
			for idx, expectedHHWinner := range scriptResult.HighHands {
				actualHHWinner := actualResult.HighHandWinners[idx]

				sort.Slice(expectedHHWinner.HhCards, func(i, j int) bool { return expectedHHWinner.HhCards[i] < expectedHHWinner.HhCards[j] })
				sort.Slice(actualHHWinner.HhCards, func(i, j int) bool { return actualHHWinner.HhCards[i] < actualHHWinner.HhCards[j] })

				if expectedHHWinner.PlayerName != actualHHWinner.PlayerName ||
					!cmp.Equal(expectedHHWinner.HhCards, actualHHWinner.HhCards) ||
					!cmp.Equal(expectedHHWinner.PlayerCards, actualHHWinner.PlayerCards) {
					bp.logger.Error().Msgf("Hand %d result verify failed. High hand winners expected: %+v actual: %+v", bp.game.handNum, expectedHHWinner, actualHHWinner)
					passed = false
				}
			}
		}
	}

	if !passed {
		panic(fmt.Sprintf("Hand %d result verify failed. Please check the logs.", bp.game.handNum))
	}
}

func (bp *BotPlayer) verifyPotWinners(actualPot *game.PotWinners, expectedPot gamescript.WinnerPot, boardNum int, potNum int) bool {
	if actualPot == nil {
		return false
	}
	type winner struct {
		SeatNo  uint32
		Amount  float64
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
		bp.logger.Error().Msgf("Hand %d result verify failed. RunItTwice board %d pot %d HI Winners: %v. Expected: %v.", bp.game.handNum, boardNum, potNum, actualHiWinnersBySeat, expectedHiWinnersBySeat)
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
		bp.logger.Error().Msgf("Hand %d result verify failed. RunItTwice board %d pot %d LO Winners: %v. Expected: %v.", bp.game.handNum, boardNum, potNum, actualLoWinnersBySeat, expectedLoWinnersBySeat)
		passed = false
	}

	return passed
}

func (bp *BotPlayer) verifyAPIRespForHand() {
	if bp.config.Script.AutoPlay.Enabled {
		return
	}

	bp.logger.Info().Msgf("Verifying api responses")

	if bp.config.Script.AutoPlay.Enabled {
		return
	}
	passed := bp.VerifyAPIResponses(bp.gameCode, bp.config.Script.GetHand(bp.game.handNum).APIVerification)
	if !passed {
		panic(fmt.Sprintf("API response verify failed for hand %d. Please check the logs.", bp.game.handNum))
	}
}

func (bp *BotPlayer) VerifyAPIResponses(gameCode string, apiVerification gamescript.APIVerification) bool {
	passed := true
	if apiVerification.GameResultTable != nil {
		err := bp.verifyGameResultTable(gameCode, apiVerification.GameResultTable)
		if err != nil {
			bp.logger.Error().Msgf("Failed to verify the response from game result table API: %s", err)
			passed = false
		}
	}

	return passed
}

func (bp *BotPlayer) verifyGameResultTable(gameCode string, expectedRows []gamescript.GameResultTableRow) error {
	rows, err := bp.GetGameResultTable(gameCode)
	if err != nil {
		return errors.Wrap(err, "Unable to get game result table")
	}

	if len(rows) != len(expectedRows) {
		return fmt.Errorf("Number of rows (%d) is different from expected (%d)", len(rows), len(expectedRows))
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].PlayerName < rows[j].PlayerName
	})
	sort.Slice(expectedRows, func(i, j int) bool {
		return expectedRows[i].PlayerName < expectedRows[j].PlayerName
	})

	passed := true
	for i := 0; i < len(rows); i++ {
		if rows[i].PlayerName != expectedRows[i].PlayerName {
			bp.logger.Error().Msgf("Game result table row %d player name: %s, expected: %s", i, rows[i].PlayerName, expectedRows[i].PlayerName)
			passed = false
		}
	}

	if !passed {
		return fmt.Errorf("Player names do not match the expected")
	}

	for i := 0; i < len(rows); i++ {
		if rows[i].HandsPlayed != expectedRows[i].HandsPlayed {
			bp.logger.Error().Msgf("Game result table player %s hands played: %d, expected: %d", rows[i].PlayerName, rows[i].HandsPlayed, expectedRows[i].HandsPlayed)
			passed = false
		}
		if rows[i].BuyIn != expectedRows[i].BuyIn {
			bp.logger.Error().Msgf("Game result table player %s buy-in: %v, expected: %v", rows[i].PlayerName, rows[i].BuyIn, expectedRows[i].BuyIn)
			passed = false
		}
		if rows[i].Stack != expectedRows[i].Stack {
			bp.logger.Error().Msgf("Game result table player %s stack: %v, expected: %v", rows[i].PlayerName, rows[i].Stack, expectedRows[i].Stack)
			passed = false
		}
		if rows[i].Profit != expectedRows[i].Profit {
			bp.logger.Error().Msgf("Game result table player %s profit: %v, expected: %v", rows[i].PlayerName, rows[i].Profit, expectedRows[i].Profit)
			passed = false
		}
		if rows[i].RakePaid != expectedRows[i].RakePaid {
			bp.logger.Error().Msgf("Game result table player %s rake paid: %v, expected: %v", rows[i].PlayerName, rows[i].RakePaid, expectedRows[i].RakePaid)
			passed = false
		}
	}

	if !passed {
		return fmt.Errorf("Row data does not match the expected")
	}

	bp.logger.Info().Msgf("Successfully verified %d rows from game result table response", len(rows))
	return nil
}

func (bp *BotPlayer) SetClubCode(clubCode string) {
	bp.clubCode = clubCode
}

func (bp *BotPlayer) GetSeatNo() uint32 {
	return bp.seatNo
}

func (bp *BotPlayer) SetBalance(balance float64) {
	bp.balance = balance
}

func (bp *BotPlayer) SetSeatInfo(seatInfo map[uint32]game.SeatInfo) {
	bp.seatInfo = seatInfo
}

func (bp *BotPlayer) SignUp() error {
	bp.logger.Info().Msgf("Signing up as a user.")

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
		return errors.Wrap(err, "Unable to get the player encryption key")
	}

	bp.EncryptionKey = encryptionKey
	bp.logger.Info().Msgf("Successfully signed up as a user. Player UUID: [%s] Player ID: [%d].", bp.PlayerUUID, bp.PlayerID)
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
		return errors.Wrap(err, "Unable to get the player encryption key")
	}

	bp.EncryptionKey = encryptionKey
	bp.logger.Info().Msgf("Successfully logged in.")
	return nil
}

// CreateClub creates a new club.
func (bp *BotPlayer) CreateClub(name string, description string) (string, error) {
	bp.logger.Info().Msgf("Creating a new club [%s].", name)

	clubCode, err := bp.gqlHelper.CreateClub(name, description)
	if err != nil {
		return "", errors.Wrap(err, "Unable to create a new club")
	}

	bp.logger.Info().Msgf("Successfully created a new club. Club Code: [%s]", clubCode)
	bp.clubCode = clubCode
	return bp.clubCode, nil
}

// JoinClub joins the bot to a club.
func (bp *BotPlayer) JoinClub(clubCode string) error {
	bp.logger.Info().Msgf("Applying to club [%s].", clubCode)

	status, err := bp.gqlHelper.JoinClub(clubCode)
	if err != nil {
		return errors.Wrap(err, "Unable to apply to the club")
	}
	bp.logger.Info().Msgf("Successfully applied to club [%s]. Member Status: [%s]", clubCode, status)

	bp.clubID, err = bp.GetClubID(clubCode)
	if err != nil {
		return errors.Wrap(err, "Unable to get the club ID")
	}
	bp.clubCode = clubCode

	return nil
}

// GetClubMemberStatus returns the club member status of this bot.
func (bp *BotPlayer) GetClubMemberStatus(clubCode string) (int, error) {
	bp.logger.Info().Msgf("Querying member status for club [%s].", clubCode)

	resp, err := bp.gqlHelper.GetClubMemberStatus(clubCode)
	if err != nil {
		return -1, errors.Wrap(err, "Unable to get club member status")
	}
	status := int(game.ClubMemberStatus_value[resp.Status])
	bp.logger.Info().Msgf("Club member Status: [%s] StatusInt: %d", resp.Status, status)
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
func (bp *BotPlayer) CreateClubReward(clubCode string, name string, rewardType string, scheduleType string, amount float64) (uint32, error) {
	bp.logger.Info().Msgf("Creating a new club reward [%s].", name)
	rewardID, err := bp.GetRewardID(clubCode, name)

	if rewardID == 0 {
		rewardID, err = bp.gqlHelper.CreateClubReward(clubCode, name, rewardType, scheduleType, amount)
		if err != nil {
			return 0, errors.Wrap(err, "Unable to create a new club")
		}
	}
	bp.RewardsNameToID[name] = rewardID
	bp.logger.Info().Msgf("Successfully created a new club reward. Club Code: [%s], rewardId: %d, name: %s, type: %s",
		clubCode, rewardID, name, rewardType)
	return rewardID, nil
}

// GetClubID queries for the numeric club ID using the club code.
func (bp *BotPlayer) GetClubID(clubCode string) (uint64, error) {
	clubID, err := bp.gqlHelper.GetClubID(clubCode)
	if err != nil {
		return 0, errors.Wrap(err, fmt.Sprintf("Unable to get club ID for club code [%s]", clubCode))
	}
	return clubID, nil
}

// ApproveClubMembers checks and approves the pending club membership applications.
func (bp *BotPlayer) ApproveClubMembers() error {
	bp.logger.Info().Msgf("Checking for pending application for the club [%s].", bp.clubCode)
	if bp.clubCode == "" {
		return fmt.Errorf("clubCode is missing")
	}

	clubMembers, err := bp.gqlHelper.GetClubMembers(bp.clubCode)
	if err != nil {
		return errors.Wrap(err, "Unable to query for club members")
	}

	// Now go through each member and approve all pending members.
	for _, member := range clubMembers {
		if member.Status != "PENDING" {
			continue
		}
		newStatus, err := bp.gqlHelper.ApproveClubMember(bp.clubCode, member.PlayerID)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("Unable to approve member [%s] player ID [%s]", member.Name, member.PlayerID))
		}
		if newStatus != "ACTIVE" {
			return fmt.Errorf("Unable to approve member [%s] player ID [%s]. Member Status is [%s]", member.Name, member.PlayerID, newStatus)
		}
		bp.logger.Info().Msgf("Successfully approved [%s] for club [%s]. Member Status: [%s]", member.Name, bp.clubCode, newStatus)
	}
	return nil
}

// CreateGame creates a new game.
func (bp *BotPlayer) CreateGame(gameOpt game.GameCreateOpt) (uint64, string, error) {
	bp.logger.Info().Msgf("Creating a new game [%s].", gameOpt.Title)

	created, err := bp.gqlHelper.CreateGame(bp.clubCode, gameOpt)
	if err != nil {
		return 0, "", errors.Wrap(err, "Unable to create new game")
	}
	gameID := created.ConfiguredGame.GameID
	gameCode := created.ConfiguredGame.GameCode
	bp.logger.Info().Msgf("Successfully created a new game. Game Code: [%s]", gameCode)
	bp.gameID = gameID
	bp.gameCode = gameCode
	bp.UpdateLogger()
	return bp.gameID, bp.gameCode, nil
}

// Subscribe makes the bot subscribe to the game's nats subjects.
func (bp *BotPlayer) Subscribe(gameToAllSubjectName string,
	handToAllSubjectName string,
	handToPlayerSubjectName string,
	handToPlayerTextSubjectName string,
	playerChannelName string) error {

	if bp.gameMsgSubscription == nil || !bp.gameMsgSubscription.IsValid() {
		bp.logger.Info().Msgf("Subscribing to %s to receive game messages sent to players/observers", gameToAllSubjectName)
		gameToAllSub, err := bp.natsConn.Subscribe(gameToAllSubjectName, bp.handleGameMsg)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("Unable to subscribe to the game message subject [%s]", gameToAllSubjectName))
		}
		bp.gameMsgSubscription = gameToAllSub
		bp.logger.Info().Msgf("Successfully subscribed to %s.", gameToAllSubjectName)
	}

	if bp.handMsgAllSubscription == nil || !bp.handMsgAllSubscription.IsValid() {
		bp.logger.Info().Msgf("Subscribing to %s to receive hand messages sent to players/observers", handToAllSubjectName)
		handToAllSub, err := bp.natsConn.Subscribe(handToAllSubjectName, bp.handleHandMsg)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("Unable to subscribe to the hand message subject [%s]", handToAllSubjectName))
		}
		bp.handMsgAllSubscription = handToAllSub
		bp.logger.Info().Msgf("Successfully subscribed to %s.", handToAllSubjectName)
	}

	if bp.handMsgPlayerSubscription == nil || !bp.handMsgPlayerSubscription.IsValid() {
		bp.logger.Info().Msgf("Subscribing to %s to receive hand messages sent to player: %s", handToPlayerSubjectName, bp.config.Name)
		handToPlayerSub, err := bp.natsConn.Subscribe(handToPlayerSubjectName, bp.handlePrivateHandMsg)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("Unable to subscribe to the hand message subject [%s]", handToPlayerSubjectName))
		}
		bp.handMsgPlayerSubscription = handToPlayerSub
		bp.logger.Info().Msgf("Successfully subscribed to %s.", handToPlayerSubjectName)
	}

	if bp.handMsgPlayerTextSubscription == nil || !bp.handMsgPlayerTextSubscription.IsValid() {
		bp.logger.Info().Msgf("Subscribing to %s to receive hand text messages sent to player: %s", handToPlayerTextSubjectName, bp.config.Name)
		handToPlayerTextSub, err := bp.natsConn.Subscribe(handToPlayerTextSubjectName, bp.handlePrivateHandTextMsg)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("Unable to subscribe to the hand (text channel) message subject [%s]", handToPlayerTextSubjectName))
		}
		bp.handMsgPlayerTextSubscription = handToPlayerTextSub
		bp.logger.Info().Msgf("Successfully subscribed to %s.", handToPlayerTextSubjectName)
	}

	if bp.playerMsgPlayerSubscription == nil || !bp.playerMsgPlayerSubscription.IsValid() {
		bp.logger.Info().Msgf("Subscribing to %s to receive hand messages sent to player: %s", handToPlayerSubjectName, bp.config.Name)
		sub, err := bp.natsConn.Subscribe(playerChannelName, bp.handlePlayerPrivateMsg)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("Unable to subscribe to the hand message subject [%s]", handToPlayerSubjectName))
		}
		bp.playerMsgPlayerSubscription = sub
		bp.logger.Info().Msgf("Successfully subscribed to %s.", handToPlayerSubjectName)
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
	if bp.tournamentMsgSubscription != nil {
		err := bp.tournamentMsgSubscription.Unsubscribe()
		if err != nil {
			errMsg = fmt.Sprintf("%s Error [%s] while unsubscribing from subject [%s]", errMsg, err, bp.tournamentMsgSubscription.Subject)
		}
		bp.tournamentMsgSubscription = nil
	}
	if bp.tournamentPlayerMsgSubscription != nil {
		err := bp.tournamentPlayerMsgSubscription.Unsubscribe()
		if err != nil {
			errMsg = fmt.Sprintf("%s Error [%s] while unsubscribing from subject [%s]", errMsg, err, bp.tournamentPlayerMsgSubscription.Subject)
		}
		bp.tournamentPlayerMsgSubscription = nil
	}
	bp.event(BotEvent__UNSUBSCRIBE)
	if errMsg != "" {
		return fmt.Errorf(errMsg)
	}
	return nil
}

// enterGame enters a game without taking a seat as a player.
func (bp *BotPlayer) enterGame(gameCode string) error {
	gi, err := bp.GetGameInfo(gameCode)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Unable to get game info %s", gameCode))
	}

	bp.game = &gameView{
		table: &tableView{
			playersBySeat: make(map[uint32]*player),
			actionTracker: game.NewHandActionTracker(),
			playersActed:  make(map[uint32]*game.PlayerActRound),
		},
	}

	bp.gameCode = gameCode
	bp.gameID = gi.GameID
	bp.UpdateLogger()
	bp.gameInfo = &gi

	playerChannelName := fmt.Sprintf("player.%d", bp.PlayerID)
	err = bp.Subscribe(gi.GameToPlayerChannel, gi.HandToAllChannel, gi.HandToPlayerChannel, gi.HandToPlayerTextChannel, playerChannelName)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Unable to subscribe to game %s channels", gameCode))
	}

	bp.meToHandSubjectName = gi.PlayerToHandChannel
	bp.clientAliveSubjectName = gi.ClientAliveChannel

	bp.logger.Info().Msgf("Starting network check client")
	bp.clientAliveCheck = networkcheck.NewClientAliveCheck(bp.logger, gi.GameID, gameCode, bp.sendAliveMsg)
	bp.clientAliveCheck.Run()

	return nil
}

// ObserveGame enters the game without taking a seat as a player.
// Every player must call either JoinGame or ObserveGame in order to participate in a game.
func (bp *BotPlayer) ObserveGame(gameCode string) error {
	bp.logger.Info().Msgf("Observing game %s", gameCode)
	err := bp.enterGame(gameCode)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Unable to enter game %s", gameCode))
	}
	return nil
}

// JoinGame enters a game and takes a seat in the game table as a player.
// Every player must call either JoinGame or ObserveGame in order to participate in a game.
func (bp *BotPlayer) JoinGame(gameCode string, gps *gamescript.GpsLocation) error {
	scriptSeatNo := bp.config.Script.GetSeatNoByPlayerName(bp.config.Name)
	if scriptSeatNo == 0 {
		return fmt.Errorf("Unable to get the scripted seat number")
	}
	scriptBuyInAmount := bp.config.Script.GetInitialBuyInAmount(scriptSeatNo)
	if scriptBuyInAmount == 0 {
		return fmt.Errorf("Unable to get the scripted buy-in amount")
	}

	err := bp.enterGame(gameCode)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Unable to enter game %s", gameCode))
	}

	playerInMySeat := bp.getPlayerInSeat(scriptSeatNo)
	if playerInMySeat != nil && playerInMySeat.Name == bp.config.Name {
		// I was already sitting in this game and still have my seat. Just rejoining after a crash.

		bp.event(BotEvent__REJOIN)

		if bp.gameInfo.PlayerGameStatus == game.PlayerStatus_WAIT_FOR_BUYIN.String() {
			// I was sitting, but crashed before submitting a buy-in.
			// The game's waiting for me to buy in, so that it can start a hand. Go ahead and submit a buy-in request.
			if bp.IsHuman() {
				bp.logger.Info().Msgf("Press ENTER to buy in [%v] chips...", scriptBuyInAmount)
				bufio.NewReader(os.Stdin).ReadBytes('\n')
			}
			err := bp.BuyIn(gameCode, scriptBuyInAmount)
			if err != nil {
				return errors.Wrap(err, "Unable to buy in after rejoining game")
			}
			bp.balance = scriptBuyInAmount

			bp.event(BotEvent__SUCCEED_BUYIN)
		} else {
			// I was playing, but crashed in the middle of an ongoing hand. What is the state of the hand now?
			err := bp.queryCurrentHandState()
			if err != nil {
				return errors.Wrap(err, "Unable to query current hand state")
			}
		}
	} else {
		// Joining a game from fresh.
		if bp.IsHuman() {
			bp.logger.Info().Msgf("Press ENTER to take seat [%d]...", scriptSeatNo)
			bufio.NewReader(os.Stdin).ReadBytes('\n')
		} else {

		}

		bp.event(BotEvent__REQUEST_SIT)
		if gps == nil {
			gps = bp.config.Gps
		}
		err := bp.SitIn(gameCode, scriptSeatNo, gps)
		if err != nil {
			return errors.Wrap(err, "Unable to sit in")
		}

		if bp.IsHuman() {
			bp.logger.Info().Msgf("Press ENTER to buy in [%v] chips...", scriptBuyInAmount)
			bufio.NewReader(os.Stdin).ReadBytes('\n')
		} else {
			// update player config
			scriptSeatConfig := bp.config.Script.GetSeatConfigByPlayerName(bp.config.Name)
			runItTwice := scriptSeatConfig.RunItTwice
			if scriptSeatConfig != nil {
				bp.UpdateGamePlayerSettings(gameCode, nil, nil, nil, nil, runItTwice, &scriptSeatConfig.MuckLosingHand)
			}

			if bp.config.Script.AutoPlay.Enabled {
				runItTwice := true
				muckLosingHand := true
				bp.UpdateGamePlayerSettings(gameCode, nil, nil, nil, nil, &runItTwice, &muckLosingHand)
			}
		}
		bp.buyInAmount = uint32(scriptBuyInAmount)
		err = bp.BuyIn(gameCode, scriptBuyInAmount)
		if err != nil {
			return errors.Wrap(err, "Unable to buy in")
		}

		bp.event(BotEvent__SUCCEED_BUYIN)
	}

	bp.updateSeatNo(scriptSeatNo)
	bp.balance = scriptBuyInAmount

	return nil
}

func (bp *BotPlayer) NewPlayer(gameCode string, startingSeat *gamescript.StartingSeat) error {
	bp.logger.Info().Msgf("Player joining the game next hand.")
	bp.observing = true
	scriptSeatNo := startingSeat.Seat
	if scriptSeatNo == 0 {
		return fmt.Errorf("Unable to get the scripted seat number")
	}
	scriptBuyInAmount := startingSeat.BuyIn
	if scriptBuyInAmount == 0 {
		return fmt.Errorf("Unable to get the scripted buy-in amount")
	}

	err := bp.enterGame(gameCode)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Unable to enter game %s", gameCode))
	}

	playerInMySeat := bp.getPlayerInSeat(scriptSeatNo)
	if playerInMySeat != nil && playerInMySeat.Name == bp.config.Name {
		// I was already sitting in this game and still have my seat. Just rejoining after a crash.

		bp.event(BotEvent__REJOIN)

		if bp.gameInfo.PlayerGameStatus == game.PlayerStatus_WAIT_FOR_BUYIN.String() {
			// I was sitting, but crashed before submitting a buy-in.
			// The game's waiting for me to buy in, so that it can start a hand. Go ahead and submit a buy-in request.
			if bp.IsHuman() {
				bp.logger.Info().Msgf("Press ENTER to buy in [%v] chips...", scriptBuyInAmount)
				bufio.NewReader(os.Stdin).ReadBytes('\n')
			}
			err := bp.BuyIn(gameCode, scriptBuyInAmount)
			if err != nil {
				return errors.Wrap(err, "Unable to buy in after rejoining game")
			}
			bp.balance = scriptBuyInAmount

			bp.event(BotEvent__SUCCEED_BUYIN)
		} else {
			// I was playing, but crashed in the middle of an ongoing hand. What is the state of the hand now?
			err := bp.queryCurrentHandState()
			if err != nil {
				return errors.Wrap(err, "Unable to query current hand state")
			}
		}
	} else {
		// Joining a game from fresh.
		if bp.IsHuman() {
			bp.logger.Info().Msgf("Press ENTER to take seat [%d]...", scriptSeatNo)
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
			return errors.Wrap(err, "Unable to sit in")
		}

		if bp.IsHuman() {
			bp.logger.Info().Msgf("Press ENTER to buy in [%v] chips...", scriptBuyInAmount)
			bufio.NewReader(os.Stdin).ReadBytes('\n')
		} else {
			// update player config
			scriptSeatConfig := bp.config.Script.GetSeatConfigByPlayerName(bp.config.Name)
			if scriptSeatConfig != nil {
				bp.UpdateGamePlayerSettings(gameCode, nil, nil, nil, nil, nil, &scriptSeatConfig.MuckLosingHand)
			}
		}
		bp.buyInAmount = uint32(scriptBuyInAmount)
		err = bp.BuyIn(gameCode, scriptBuyInAmount)
		if err != nil {
			return errors.Wrap(err, "Unable to buy in")
		}

		bp.event(BotEvent__SUCCEED_BUYIN)

		// post blind
		if startingSeat.PostBlind {
			bp.logger.Info().Msgf("Posted blind")
			bp.gqlHelper.PostBlind(bp.gameCode)
		}
	}

	bp.updateSeatNo(scriptSeatNo)
	bp.balance = scriptBuyInAmount

	return nil
}

func (bp *BotPlayer) autoReloadBalance() error {
	seatConfig := bp.config.Script.GetSeatConfigByPlayerName(bp.config.Name)
	if seatConfig == nil {
		return nil
	}
	if seatConfig.AutoReload != nil && *seatConfig.AutoReload == false {
		// This player is explicitly set to not reload.
		return nil
	}
	bp.logger.Info().Msgf("[%s] Buyin %v.", bp.gameCode, bp.gameInfo.BuyInMax)
	err := bp.BuyIn(bp.gameCode, bp.gameInfo.BuyInMax)
	if err != nil {
		return errors.Wrap(err, "Unable to buy in")
	}

	// automaticaly post blind
	if bp.config.Script.BotConfig.AutoPostBlind {
		_, err = bp.gqlHelper.PostBlind(bp.gameCode)
		if err != nil {
			return errors.Wrap(err, "Unable to post blind")
		}
	}
	bp.balance = bp.gameInfo.BuyInMax
	return err
}

// JoinUnscriptedGame joins a game without using the yaml script. This is used for joining
// a human-created game where you can freely grab whatever seat available.
func (bp *BotPlayer) JoinUnscriptedGame(gameCode string, demoGame bool) error {
	if !bp.config.Script.AutoPlay.Enabled {
		return fmt.Errorf("JoinUnscriptedGame called with a non-autoplay script")
	}

	err := bp.enterGame(gameCode)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Unable to enter game %s", gameCode))
	}
	if len(bp.gameInfo.SeatInfo.AvailableSeats) == 0 {
		return fmt.Errorf("Unable to join game [%s]. Seats are full", gameCode)
	}
	seatNo := bp.gameInfo.SeatInfo.AvailableSeats[0]
	if demoGame {
		seatNo = bp.gameInfo.SeatInfo.AvailableSeats[1]
	}

	bp.event(BotEvent__REQUEST_SIT)

	err = bp.SitIn(gameCode, seatNo, bp.config.Gps)
	if err != nil {
		return errors.Wrap(err, "Unable to sit in")
	}
	buyInAmt := bp.gameInfo.BuyInMax
	err = bp.BuyIn(gameCode, buyInAmt)
	if err != nil {
		return errors.Wrap(err, "Unable to buy in")
	}
	bp.updateSeatNo(seatNo)

	// unscripted game, bots will run it twice
	if bp.gameInfo.RunItTwiceAllowed {
		t := true
		bp.UpdateGamePlayerSettings(gameCode, nil, nil, nil, nil, &t, &t)
	}

	bp.event(BotEvent__SUCCEED_BUYIN)

	return nil
}

// SitIn takes a seat in a game as a player.
func (bp *BotPlayer) SitIn(gameCode string, seatNo uint32, gps *gamescript.GpsLocation) error {
	if seatNo == 0 {
		panic("seatNo == 0 in SitIn")
	}
	bp.logger.Info().Msgf("Grabbing seat [%d] in game [%s].", seatNo, gameCode)
	bp.gqlHelper.IpAddress = bp.config.IpAddress
	status, err := bp.gqlHelper.SitIn(gameCode, seatNo, gps)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Unable to sit in game [%s]", gameCode))
	}

	bp.observing = false
	bp.inWaitList = false
	bp.updateSeatNo(seatNo)
	bp.logger.Info().Msgf("Successfully took a seat %d in game [%s]. Status: [%s]", status.Seat.SeatNo, gameCode, status.Seat.Status)
	return nil
}

// BuyIn is where you buy the chips once seated in a game.
func (bp *BotPlayer) BuyIn(gameCode string, amount float64) error {
	//bp.logger.Info().Msgf("Buying in amount [%v].", amount)

	resp, err := bp.gqlHelper.BuyIn(gameCode, amount)
	if err != nil {
		return errors.Wrapf(err, "Error from GQL helper while trying to buy in %v chips", amount)
	}

	if resp.Status.Approved {
		bp.logger.Info().Msgf("Successfully bought in [%v] chips.", amount)
	} else {
		bp.logger.Info().Msgf("Requested to buy in [%v] chips. Needs approval.", amount)
	}

	return nil
}

// LeaveGameImmediately makes the bot leave the game.
func (bp *BotPlayer) LeaveGameImmediately() error {
	bp.logger.Info().Msgf("Leaving game [%s].", bp.gameCode)
	err := bp.unsubscribe()
	if err != nil {
		return errors.Wrap(err, "Error while unsubscribing from NATS subjects")
	}
	if !bp.tournament {
		if bp.IsSeated() && !bp.hasSentLeaveGameRequest {
			_, err = bp.gqlHelper.LeaveGame(bp.gameCode)
			if err != nil {
				return errors.Wrap(err, "Error while making a GQL request to leave game")
			}
		}
	}
	go func() {
		bp.end <- true
		bp.endPing <- true
	}()
	bp.updateSeatNo(0)
	bp.gameCode = ""
	bp.gameID = 0
	bp.UpdateLogger()
	if bp.clientAliveCheck != nil {
		bp.clientAliveCheck.Destroy()
	}
	bp.clientAliveCheck = nil
	return nil
}

// GetGameInfo queries the game info from the api server.
func (bp *BotPlayer) GetGameInfo(gameCode string) (gameInfo game.GameInfo, err error) {
	return bp.gqlHelper.GetGameInfo(gameCode)
}

// GetGameResultTable queries the game info from the api server.
func (bp *BotPlayer) GetGameResultTable(gameCode string) (gameInfo []game.GameResultTableRow, err error) {
	return bp.gqlHelper.GetGameResultTable(gameCode)
}

// GetPlayersInSeat queries for the numeric game ID using the game code.
func (bp *BotPlayer) GetPlayersInSeat(gameCode string) ([]game.SeatInfo, error) {
	gameInfo, err := bp.GetGameInfo(gameCode)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to get players in seat")
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
		return 0, errors.Wrap(err, "Unable to get player ID")
	}
	if playerID.Name != bp.config.Name {
		return 0, fmt.Errorf("Unable to get player ID. Player name [%s] does not match the bot player's name [%s]", playerID.Name, bp.config.Name)
	}
	return playerID.ID, nil
}

func (bp *BotPlayer) getEncryptionKey() (string, error) {
	encryptionKey, err := bp.gqlHelper.GetEncryptionKey()
	if err != nil {
		return "", errors.Wrapf(err, "Unable to get encryption key")
	}
	return encryptionKey, nil
}

// StartGame starts the game.
func (bp *BotPlayer) StartGame(gameCode string) error {
	bp.logger.Info().Msgf("Starting the game [%s].", gameCode)

	// setup first deck if not auto play
	if bp.IsHost() && !bp.config.Script.AutoPlay.Enabled {
		err := bp.setupNextHand()
		if err != nil {
			return errors.Wrapf(err, "Unable to setup next hand")
		}
		// Wait for the first hand to be setup before starting the game.
		time.Sleep(500 * time.Millisecond)
	}

	status, err := bp.gqlHelper.StartGame(gameCode)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Unable to start the game [%s]", gameCode))
	}
	if status != "ACTIVE" {
		return fmt.Errorf("Unable to start the game [%s]. Status is [%s]", gameCode, status)
	}

	bp.logger.Info().Msgf("Successfully started the game [%s]. Status: [%s]", gameCode, status)
	return nil
}

// RequestEndGame schedules to end the game after the current hand is finished.
func (bp *BotPlayer) RequestEndGame(gameCode string) error {
	bp.logger.Info().Msgf("Requesting to end the game [%s].", gameCode)

	status, err := bp.gqlHelper.EndGame(gameCode)
	if err != nil {
		return errors.Wrapf(err, "Error while requesting to end the game")
	}

	bp.logger.Info().Msgf("Successfully requested to end the game [%s]. Status: [%s]", gameCode, status)
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
		return errors.Wrap(err, "Could not create query hand message.")
	}
	bp.logger.Info().Msgf("Querying current hand. Msg: %s", string(protoData))
	// Send to hand subject.
	err = bp.natsConn.Publish(bp.meToHandSubjectName, protoData)
	if err != nil {
		return errors.Wrap(err, "Unable to publish to nats")
	}
	return nil
}

func (bp *BotPlayer) sendAliveMsg() {
	msg := game.ClientAliveMessage{
		GameId:   bp.gameID,
		GameCode: bp.gameCode,
		PlayerId: bp.PlayerID,
	}

	protoData, err := proto.Marshal(&msg)
	if err != nil {
		bp.logger.Error().Err(err).Msgf("Could not proto-marshal client-alive message")
		return
	}
	bp.logger.Debug().Msgf("Sending client-alive message to %s", bp.clientAliveSubjectName)
	err = bp.natsConn.Publish(bp.clientAliveSubjectName, protoData)
	if err != nil {
		bp.logger.Error().Err(err).Msgf("Unable to publish client-alive message to nats channel %s", bp.clientAliveSubjectName)
	}
}

// UpdateGamePlayerSettings updates player's configuration for this game.
func (bp *BotPlayer) UpdateGamePlayerSettings(
	gameCode string,
	autoStraddle *bool,
	straddle *bool,
	buttonStraddle *bool,
	bombPotEnabled *bool,
	runItTwiceAllowed *bool,
	muckLosingHand *bool,
) error {
	ritStr := "<nil>"
	if runItTwiceAllowed != nil {
		ritStr = fmt.Sprintf("%v", *runItTwiceAllowed)
	}
	mlhStr := "<nil>"
	if muckLosingHand != nil {
		mlhStr = fmt.Sprintf("%v", *muckLosingHand)
	}
	bp.logger.Info().Msgf("Updating player configuration [runItTwiceAllowed: %s, muckLosingHand: %s] game [%s].",
		ritStr, mlhStr, gameCode)
	settings := gql.GamePlayerSettingsUpdateInput{
		AutoStraddle:      autoStraddle,
		Straddle:          straddle,
		ButtonStraddle:    buttonStraddle,
		BombPotEnabled:    bombPotEnabled,
		MuckLosingHand:    muckLosingHand,
		RunItTwiceEnabled: runItTwiceAllowed,
	}
	err := bp.gqlHelper.UpdateGamePlayerSettings(gameCode, settings)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Unable to update game config [%s]", gameCode))
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

func (bp *BotPlayer) act(seatAction *game.NextSeatAction, handStatus game.HandStatus) {
	availableActions := seatAction.AvailableActions
	nextAction := game.ACTION_CHECK
	nextAmt := float64(0)
	autoPlay := false
	if bp.tournament {
		autoPlay = true
	} else {
		if bp.config.Script.AutoPlay.Enabled {
			autoPlay = true
		} else if len(bp.config.Script.Hands) >= int(bp.game.handNum) {
			handScript := bp.config.Script.GetHand(bp.game.handNum)
			if !bp.doesScriptActionExists(handScript, bp.game.handStatus) {
				autoPlay = true
			}
		}
	}
	runItTwiceActionPrompt := false
	timeout := false
	var actionDelayOverride uint32
	var extendActionTimeoutBySec uint32
	var resetActionTimerToSec uint32
	if autoPlay {
		bp.logger.Debug().Msgf("Seat %d Available actions: %+v", bp.seatNo, seatAction.AvailableActions)
		canBet := false
		canRaise := false
		checkAvailable := false
		callAvailable := false
		allInAvailable := false
		minBet := seatAction.MinRaiseAmount
		maxBet := seatAction.MaxRaiseAmount
		allInAmount := seatAction.AllInAmount

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
			}
			if action == game.ACTION_RAISE {
				canRaise = true
			}
			if action == game.ACTION_ALLIN {
				allInAvailable = true
			}
			if action == game.ACTION_RUN_IT_TWICE_PROMPT {
				runItTwiceActionPrompt = true
			}
		}

		if checkAvailable {
			nextAction = game.ACTION_CHECK
			nextAmt = 0.0
		}
		/*
			1: "Straight Flush",
			2: "Four of a Kind",
			3: "Full House",
			4: "Flush",				// 100% (minBet*15)
			5: "Straight",			// 100% (minBet*10)
			6: "Three of a Kind",	// 80% (minBet*5)
			7: "Two Pair",			// 50% (minBet*3)
			8: "Pair",
			9: "High Card",
		*/

		if bp.tournament && bp.game.handStatus != game.HandStatus_PREFLOP {
			// do I have a pair
			minBetMultiply := 0
			if bp.currentRank == "Two Pair" {
				minBetMultiply = 3
			} else if bp.currentRank == "Three of a Kind" {
				minBetMultiply = 5
			} else if bp.currentRank == "Straight" {
				minBetMultiply = 10
			} else if bp.currentRank == "Flush" {
				minBetMultiply = 15
			} else if bp.currentRank == "Full House" {
				minBetMultiply = 25
			} else if bp.currentRank == "Four of a Kind" {
				minBetMultiply = 50
			} else if bp.currentRank == "Straight Flush" {
				minBetMultiply = 100
			}
			nextAmt = minBet * float64(minBetMultiply)
			if nextAmt > 0 {
				nextAmt = nextAmt * 1.0
			}
		} else {
			if bp.havePair {
				pairValue := (float64)(bp.pairCard / 16)
				nextAmt = pairValue * minBet
			}
		}

		if nextAmt > maxBet {
			nextAmt = maxBet
		}

		if nextAmt == seatAction.AllInAmount {
			nextAction = game.ACTION_ALLIN
		} else {
			if nextAmt > 0.0 {
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
				// TODO: Bring all-in back after the load test.
				// go all innextAmt
				// nextAction = game.ACTION_ALLIN
				nextAction = game.ACTION_FOLD
				nextAmt = 0

				// TODO: Bring all-in back after the load test.
				// nextAmt = allInAmount
			} else {
				// TODO: Bring back after the load test.
				// if nextAmt > bp.balance/2 {
				if nextAmt > bp.balance/5 {
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
		if !bp.tournament && bp.gameInfo.RunItTwiceAllowed {
			players := bp.humanPlayers()
			if len(players) == 1 {
				player := players[0]
				// human player action
				action := bp.game.table.playersActed[player.seatNo]
				if action != nil && action.Action == game.ACTION_ALLIN {
					activePlayers := bp.activePlayers()
					if len(activePlayers) <= 2 {
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

		if nextAction == game.ACTION_FOLD {
			nextAmt = 0
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
				bp.logger.Panic().Msgf("Bot seatNo (%d) != script seatNo (%d)", bp.seatNo, scriptAction.Action.Seat)
			}
			if err != nil {
				bp.logger.Error().Msgf("Unable to get the next action %+v", err)
				return
			}
			nextAction = game.ActionStringToAction(scriptAction.Action.Action)
			nextAmt = scriptAction.Action.Amount
			timeout = scriptAction.Timeout
			actionDelayOverride = scriptAction.ActionDelay
			extendActionTimeoutBySec = scriptAction.ExtendTimeoutBySec
			resetActionTimerToSec = scriptAction.ResetTimerToSec
			preActions := scriptAction.PreActions
			bp.processPreActions(seatAction, preActions)
		}
	}

	playerName := bp.getPlayerNameBySeatNo(bp.seatNo)
	if resetActionTimerToSec > 0 {
		bp.logger.Info().Msgf("Seat %d (%s) requesting to restart action timer at %d seconds", bp.seatNo, playerName, resetActionTimerToSec)
		resetTimer := game.ResetTimer{
			SeatNo:       bp.seatNo,
			RemainingSec: resetActionTimerToSec,
			ActionId:     seatAction.ActionId,
		}
		resetTimerMsg := game.HandMessage{
			GameCode:   bp.gameCode,
			HandNum:    bp.game.handNum,
			PlayerId:   bp.PlayerID,
			SeatNo:     bp.seatNo,
			MessageId:  uuid.NewString(),
			HandStatus: handStatus,
			Messages: []*game.HandMessageItem{
				{
					MessageType: game.HandResetTimer,
					Content: &game.HandMessageItem_ResetTimer{
						ResetTimer: &resetTimer,
					},
				},
			},
		}
		bp.publishHandMsg(bp.meToHandSubjectName, &resetTimerMsg)
	}
	if extendActionTimeoutBySec > 0 {
		bp.logger.Info().Msgf("Seat %d (%s) requesting to extend action timeout by %d seconds", bp.seatNo, playerName, extendActionTimeoutBySec)
		extendTimer := game.ExtendTimer{
			SeatNo:      bp.seatNo,
			ExtendBySec: extendActionTimeoutBySec,
			ActionId:    seatAction.ActionId,
		}
		extendTimerMsg := game.HandMessage{
			GameCode:   bp.gameCode,
			HandNum:    bp.game.handNum,
			PlayerId:   bp.PlayerID,
			SeatNo:     bp.seatNo,
			MessageId:  uuid.NewString(),
			HandStatus: handStatus,
			Messages: []*game.HandMessageItem{
				{
					MessageType: game.HandExtendTimer,
					Content: &game.HandMessageItem_ExtendTimer{
						ExtendTimer: &extendTimer,
					},
				},
			},
		}
		bp.publishHandMsg(bp.meToHandSubjectName, &extendTimerMsg)
	}

	if bp.IsHuman() {
		bp.logger.Info().Msgf("Seat %d: Your Turn. Press ENTER to continue with [%s %v] (Hand Status: %s)...", bp.seatNo, nextAction, nextAmt, bp.game.handStatus)
		bufio.NewReader(os.Stdin).ReadBytes('\n')
	}

	var handAction game.HandAction
	runItTwiceTimeout := false
	if runItTwiceActionPrompt {
		runItTwiceConf := bp.getRunItTwiceConfig()
		if runItTwiceConf != nil {
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
				Action: game.ACTION_RUN_IT_TWICE_YES,
			}
		}
	} else {
		handAction = game.HandAction{
			SeatNo:   bp.seatNo,
			Action:   nextAction,
			Amount:   nextAmt,
			ActionId: seatAction.ActionId,
		}
	}
	msgType := game.HandPlayerActed
	lastMsgIDInt, err := strconv.Atoi(bp.clientLastMsgID)
	if err != nil {
		panic(fmt.Sprintf("Unable to convert message ID to int: %v", err))
	}
	msgID := strconv.Itoa(lastMsgIDInt + 1)
	actionMsg := game.HandMessage{
		GameCode:   bp.gameCode,
		HandNum:    bp.game.handNum,
		PlayerId:   bp.PlayerID,
		SeatNo:     bp.seatNo,
		MessageId:  msgID,
		HandStatus: handStatus,
		Messages: []*game.HandMessageItem{
			{
				MessageType: msgType,
				Content:     &game.HandMessageItem_PlayerActed{PlayerActed: &handAction},
			},
		},
	}

	if timeout || runItTwiceTimeout {
		go func() {
			if runItTwiceTimeout {
				bp.logger.Info().Msgf("Seat %d (%s) is going to time out the run-it-twice prompt. Stage: %s.", bp.seatNo, playerName, bp.game.handStatus)
			} else {
				bp.logger.Info().Msgf("Seat %d (%s) is going to time out. Stage: %s.", bp.seatNo, playerName, bp.game.handStatus)
			}
			// sleep more than action time
			time.Sleep(time.Duration(bp.config.Script.Game.ActionTime) * time.Second)
			time.Sleep(2 * time.Second)
		}()
	} else {
		bp.logger.Debug().Msgf("Seat %d (%s) is about to act [%s %v]. Stage: %s.", bp.seatNo, playerName, handAction.Action, handAction.Amount, bp.game.handStatus)
		if actionDelayOverride > 0 {
			bp.logger.Info().Msgf("Seat %d (%s) sleeping for %d milliseconds", bp.seatNo, playerName, actionDelayOverride)
		}
		if bp.tournament {
			start := uint32(2)
			end := uint32(3)
			if bp.game.handStatus == game.HandStatus_TURN {
				end = 2
			}
			if bp.game.handStatus == game.HandStatus_RIVER ||
				bp.game.handStatus == game.HandStatus_SHOW_DOWN {
				end = 3
			}

			actionTimeDelay := util.GetRandomUint32(start, end)
			time.Sleep(time.Duration(actionTimeDelay) * time.Second)
		} else {
			time.Sleep(bp.getActionDelay(actionDelayOverride))
		}

		go bp.publishAndWaitForAck(bp.meToHandSubjectName, &actionMsg)
	}
}

func (bp *BotPlayer) getActionDelay(override uint32) time.Duration {
	var actionTimeMillis uint32
	if override > 0 {
		actionTimeMillis = override
	} else {
		//actionTimeMillis = 1000
		actionTimeMillis = util.GetRandomUint32(bp.config.MinActionDelay, bp.config.MaxActionDelay)
	}
	return time.Duration(actionTimeMillis) * time.Millisecond
}

func (bp *BotPlayer) publishHandMsg(subj string, msg *game.HandMessage) {
	protoData, err := proto.Marshal(msg)
	if err != nil {
		errMsg := fmt.Sprintf("Could not serialize hand message [%+v]. Error: %v", msg, err)
		bp.logger.Error().Msg(errMsg)
		bp.errorStateMsg = errMsg
		bp.sm.SetState(BotState__ERROR)
		return
	}
	err = bp.natsConn.Publish(bp.meToHandSubjectName, protoData)
	if err != nil {
		errMsg := fmt.Sprintf("Could not publish hand message [%+v]. Error: %v", msg, err)
		bp.logger.Error().Msg(errMsg)
		bp.errorStateMsg = errMsg
		bp.sm.SetState(BotState__ERROR)
		return
	}
}

func (bp *BotPlayer) publishAndWaitForAck(subj string, msg *game.HandMessage) {
	protoData, err := proto.Marshal(msg)
	if err != nil {
		errMsg := fmt.Sprintf("Could not serialize hand message [%+v]. Error: %v", msg, err)
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
				errMsg = fmt.Sprintf("Retry (%d) exhausted while publishing message type: %s, message ID: %s", bp.maxRetry, game.HandPlayerActed, msg.GetMessageId())
			} else {
				errMsg = fmt.Sprintf("Retry (%d) exhausted while waiting for game server acknowledgement for message type: %s, message ID: %s", bp.maxRetry, game.HandPlayerActed, msg.GetMessageId())
			}
			bp.logger.Error().Msg(errMsg)
			bp.errorStateMsg = errMsg
			bp.sm.SetState(BotState__ERROR)
			return
		}
		if attempts > 1 {
			bp.logger.Info().Msgf("Attempt (%d) to publish message type: %s, message ID: %s", attempts, game.HandPlayerActed, msg.GetMessageId())
		}
		if err := bp.natsConn.Publish(bp.meToHandSubjectName, protoData); err != nil {
			bp.logger.Error().Msgf("Error [%s] while publishing message %+v", err, msg)
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

func (bp *BotPlayer) rememberPlayerAction(seatNo uint32, action game.ACTION, amount float64, timedOut bool, handStatus game.HandStatus) {
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
	return bp.seatNo != 0
}

func (bp *BotPlayer) updateSeatNo(seatNo uint32) {
	bp.seatNo = seatNo
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

// GetHandResult2 returns the hand result received from the server.
func (bp *BotPlayer) GetHandResult2() *game.HandResultClient {
	if bp.game == nil {
		panic("GetHandResult2 called with nil game")
	}
	return bp.game.handResult2
}

// PrintHandResult prints the hand winners to console.
func (bp *BotPlayer) PrintHandResult() {
	result := bp.game.handResult2
	//data, _ := json.Marshal(result)
	// bp.logger.Info().Msg(string(data))

	// player winning cards

	pots := result.PotWinners
	for potNum, potWinners := range pots {
		for _, board := range potWinners.BoardWinners {
			for i, winner := range board.HiWinners {
				seatNo := winner.GetSeatNo()
				playerName := bp.getPlayerNameBySeatNo(seatNo)
				amount := winner.GetAmount()
				cardsStr := "N/A"
				rankStr := "N/A"
				//cardsStr := winner.GetWinningCardsStr()
				//rankStr := winner.GetRankStr()
				winningCards := ""
				if cardsStr != "" {
					winningCards = fmt.Sprintf(" Winning Cards: %s (%s)", cardsStr, rankStr)
				}
				bp.logger.Info().Msgf("Pot %d Hi-Winner %d: Seat %d (%s) Amount: %v%s", potNum+1, i+1, seatNo, playerName, amount, winningCards)
			}
			for i, winner := range board.LowWinners {
				seatNo := winner.GetSeatNo()
				playerName := bp.getPlayerNameBySeatNo(seatNo)
				amount := winner.GetAmount()
				cardsStr := "N/A"
				//cardsStr := winner.GetWinningCardsStr()
				//rankStr := winner.GetRankStr()
				winningCards := ""
				if cardsStr != "" {
					winningCards = fmt.Sprintf(" Lo Winning Cards: %s", cardsStr)
				}
				bp.logger.Info().Msgf("Pot %d Low-Winner %d: Seat %d (%s) Amount: %v%s", potNum+1, i+1, seatNo, playerName, amount, winningCards)
			}
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

	bp.logger.Info().Msgf("Setting up game server crash. URL: %s, Payload: %s", url, jsonBytes)
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
	bp.logger.Info().Msgf("Successfully setup game server crash.")
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

	err := bp.AddNewPlayers(&nextHand.Setup)
	if err != nil {
		return errors.Wrap(err, "Unable to add new players.")
	}

	bp.processPreDealItems(nextHand.Setup.PreDeal)

	if nextHand.Setup.ButtonPos != 0 {
		err := bp.setupButtonPos(nextHand.Setup.ButtonPos)
		if err != nil {
			return errors.Wrap(err, "Unable to set button position")
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

func (bp *BotPlayer) reloadBotFromGameInfo(newHand *game.NewHand) error {
	if bp.PlayerID == 0 {
		panic("bp.PlayerID == 0 in reloadBotFromGameInfo")
	}
	bp.game.table.playersBySeat = make(map[uint32]*player)
	if newHand.HandNum == 1 {
		gameInfo, err := bp.GetGameInfo(bp.gameCode)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("Unable to get game info %s", bp.gameCode))
		}
		bp.gameInfo = &gameInfo
	}
	var seatNo uint32
	var isPlaying bool
	for sn, p := range newHand.PlayersInSeats { //gameInfo.SeatInfo.PlayersInSeats {
		if p.OpenSeat {
			continue
		}
		isBot := true
		if playerInSeat, ok := bp.seatInfo[sn]; ok {
			isBot = playerInSeat.IsBot
		}
		pl := &player{
			playerID: p.PlayerId,
			seatNo:   sn,
			status:   game.PlayerStatus(game.PlayerStatus_value[p.Status.String()]),
			stack:    p.Stack,
			isBot:    isBot,
		}
		bp.game.table.playersBySeat[p.SeatNo] = pl
		if p.PlayerId == bp.PlayerID {
			seatNo = p.SeatNo
			if pl.status == game.PlayerStatus_PLAYING {
				isPlaying = true
			}
		}
	}
	bp.updateSeatNo(seatNo)

	bp.observing = true
	if isPlaying {
		bp.observing = false
	}

	return nil
}

func (bp *BotPlayer) isGamePaused() (bool, error) {
	gi, err := bp.GetGameInfo(bp.gameCode)
	if err != nil {
		return false, errors.Wrap(err, fmt.Sprintf("Unable to get game info %s", bp.gameCode))
	}
	if gi.Status == game.GameStatus_ENDED.String() {
		return false, fmt.Errorf("Game ended %s. Unexpected", bp.gameCode)
	}

	if gi.Status == game.GameStatus_PAUSED.String() {
		return true, nil
	}
	return false, nil
}

func (bp *BotPlayer) getPlayerNameBySeatNo(seatNo uint32) string {
	if bp.tournament {
		for _, p := range bp.tournamentTableInfo.Players {
			if p.SeatNo == seatNo {
				return p.Name
			}
		}
		return "MISSING"
	}
	for _, p := range bp.gameInfo.SeatInfo.PlayersInSeats {
		if p.SeatNo == seatNo {
			return p.Name
		}
	}
	return "MISSING"
}

func (bp *BotPlayer) getPlayerIDBySeatNo(seatNo uint32) uint64 {
	if bp.tournament {
		for _, p := range bp.tournamentTableInfo.Players {
			if p.SeatNo == seatNo {
				return p.PlayerId
			}
		}
	}
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
	bp.logger.Info().Msgf("Locating club code using name [%s].", name)

	clubCode, err := bp.gqlHelper.GetClubCode(name)
	if err != nil {
		return "", errors.Wrap(err, "Unable to get clubs")
	}
	if name == "" {
		bp.logger.Info().Msgf("No club found with name: [%s]", name)
		return "", nil
	}
	return clubCode, nil
}

// HostRequestSeatChange schedules to end the game after the current hand is finished.
func (bp *BotPlayer) HostRequestSeatChange(gameCode string) error {
	bp.logger.Info().Msgf("Host is requesting to make seat changes in game [%s].", gameCode)

	status, err := bp.gqlHelper.HostRequestSeatChange(gameCode)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Error while host is requesting to make seat changes  [%s]", gameCode))
	}

	bp.logger.Info().Msgf("Successfully requested to make seat changes. Status: [%s]", gameCode, status)
	return nil
}

// CreateClub creates a new club.
func (bp *BotPlayer) ResetDB() error {
	bp.logger.Info().Msgf("Resetting database for testing [%s].")

	err := bp.gqlHelper.ResetDB()
	if err != nil {
		return errors.Wrap(err, "Resetting database failed")
	}

	return nil
}

func (bp *BotPlayer) SetBuyinAmount(amount uint32) {
	bp.buyInAmount = amount
}
