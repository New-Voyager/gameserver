package bot

import (
	"fmt"
	"io/ioutil"
	"reflect"
	"strconv"
	"strings"

	"voyager.com/server/game"
	"voyager.com/server/poker"
	"voyager.com/server/util"

	jsoniter "github.com/json-iterator/go"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
)

var driverBotLogger = log.With().Str("logger_name", "server::driverbot").Logger()

var NatsURL = util.GameServerEnvironment.GetNatsClientConnURL()

const BotDriverToGame = "driverbot.game"
const GameToBotDriver = "game.driverpot"
const botPlayerID = 0xFFFFFFFF

// bot driver messages to game
const (
	BotDriverInitializeGame = "B2GInitializeGame"
	BotDriverStartGame      = "B2GStartGame"
	BotDriverSetupDeck      = "B2GSetupDeck"
)

// game to bot driver messages
const (
	GameInitialized = "G2BGameInitialized"
)

type DriverBotMessage struct {
	BotId       string           `json:"bot-id"`
	MessageType string           `json:"message-type"`
	GameConfig  *game.GameConfig `json:"game-config"`
	ClubId      uint32           `json:"club-id"`
	GameId      uint64           `json:"game-id"`
	GameCode    string           `json:"game-code"`
}

type DriverBot struct {
	botId          string
	stopped        chan bool
	players        map[uint64]*PlayerBot
	playersInSeats map[uint32]uint64 // seatNo to player id
	currentHand    *game.Hand
	noMoreActions  bool
	observer       *PlayerBot // driver also attaches itself as an observer
	waitCh         chan int
	observerGameCh chan *game.GameMessage
	observerHandCh chan *game.HandMessage
	gameScript     *game.GameScript
	nc             *natsgo.Conn
}

func NewDriverBot(url string) (*DriverBot, error) {
	nc, err := natsgo.Connect(url)
	if err != nil {
		driverBotLogger.Error().Msg(fmt.Sprintf("Error connecting to NATS server, error: %v", err))
		return nil, err
	}

	driverUuid := uuid.New()
	driverBot := &DriverBot{
		botId:          driverUuid.String(),
		stopped:        make(chan bool),
		players:        make(map[uint64]*PlayerBot),
		playersInSeats: make(map[uint32]uint64),
		nc:             nc,
		waitCh:         make(chan int),
		observerGameCh: make(chan *game.GameMessage),
		observerHandCh: make(chan *game.HandMessage),
	}
	nc.Subscribe(GameToBotDriver, driverBot.listenForMessages)
	return driverBot, nil
}

func (b *DriverBot) Cleanup() {
	b.nc.Close()
}

func (b *DriverBot) listenForMessages(msg *natsgo.Msg) {
	// unmarshal the message
	var botMessage DriverBotMessage
	err := jsoniter.Unmarshal(msg.Data, &botMessage)
	if err != nil {
		return
	}
	if botMessage.BotId == b.botId {
		// this is our message, handle it
		switch botMessage.MessageType {
		case GameInitialized:
			b.onGameInitialized(&botMessage)
		}
	}
}

func (b *DriverBot) RunGameScript(filename string) error {
	fmt.Printf("Running game script: %s\n", filename)

	// load game script
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		// failed to load game script file
		fmt.Printf("Failed to load file: %s\n", filename)
		return err
	}

	var gameScript game.GameScript
	err = yaml.Unmarshal(data, &gameScript)
	if err != nil {
		// failed to load game script file
		fmt.Printf("Loading json failed: %s, err: %v\n", filename, err)
		return err
	}
	if gameScript.Disabled {
		return nil
	}

	b.run(&gameScript)

	return nil
}

func (b *DriverBot) run(gameScript *game.GameScript) error {
	b.gameScript = gameScript
	initializeGameMsg := &DriverBotMessage{
		BotId:       b.botId,
		MessageType: BotDriverInitializeGame,
		GameConfig:  &gameScript.GameConfig,
	}

	// initialize game by sending the message to game server
	data, _ := jsoniter.Marshal(initializeGameMsg)

	// send to game server
	e := b.nc.Publish(BotDriverToGame, data)
	if e != nil {
		return e
	}

	// wait for all the players to sit
	<-b.waitCh
	driverBotLogger.Info().Msg("All players sat in the table")

	// get table state
	b.observer.getTableState()
	<-b.waitCh
	driverBotLogger.Info().Msg(fmt.Sprintf("Table state: %v", b.observer.lastGameMessage))

	// verify table state
	e = b.verifyTableResult(b.gameScript.AssignSeat.Verify.Table.Players, "take-seat")
	if e != nil {
		return e
	}

	// play hands
	for _, hand := range b.gameScript.Hands {
		err := b.runHand(&hand)
		if err != nil {
			return err
		}
	}

	return nil
}

func (b *DriverBot) onGameInitialized(message *DriverBotMessage) error {

	// attach driverbot as one of the players/observer
	observer, e := NewPlayerBot(NatsURL, 0xFFFFFFFF)
	if e != nil {
		driverBotLogger.Error().Msg("Error occurred when creating bot player")
		return e
	}
	b.players[botPlayerID] = observer
	observer.setObserver(b.waitCh)
	b.observer = observer
	observer.joinGame(message.ClubId, message.GameId)
	//observer.initialize(message.ClubId, message.GameNum)

	// now let the players to join the game
	for _, player := range b.gameScript.Players {
		botPlayer, e := NewPlayerBot(NatsURL, player.ID)
		if e != nil {
			driverBotLogger.Error().Msg("Error occurred when creating bot player")
			return e
		}
		b.players[player.ID] = botPlayer
		// player joined the game
		e = botPlayer.joinGame(message.ClubId, message.GameId)
		if e != nil {
			driverBotLogger.Error().Msg(fmt.Sprintf("Error occurred when bot player joing game. %d:%d", message.ClubId, message.GameId))
			return e
		}
	}

	for _, playerSeat := range b.gameScript.AssignSeat.Seats {
		b.playersInSeats[playerSeat.SeatNo] = playerSeat.Player
		b.players[playerSeat.Player].sitAtTable(playerSeat.SeatNo, playerSeat.BuyIn)
	}

	allPlayersSat := false
	for !allPlayersSat {
		allPlayersSat = true
		for _, player := range b.players {
			if player.playerID == botPlayerID {
				continue
			}
			if !player.playerAtSit {
				allPlayersSat = false
				break
			}
		}
	}
	driverBotLogger.Info().Msg("All players took the seats")
	b.waitCh <- 1
	return nil
}

func (b *DriverBot) verifyTableResult(expectedPlayers []game.PlayerAtTable, where string) error {
	if expectedPlayers == nil {
		return nil
	}

	if expectedPlayers != nil {
		explectedPlayers := expectedPlayers
		// validate the player stack here to ensure sit-in command worked
		expectedPlayersInTable := len(explectedPlayers)
		actualPlayersInTable := len(b.observer.playerStateMessage.GetPlayersState())
		if expectedPlayersInTable != actualPlayersInTable {
			e := fmt.Errorf("[%s section] Expected number of players (%d) did not match the actual players (%d)",
				where, expectedPlayersInTable, actualPlayersInTable)
			//g.result.addError(e)
			return e
		}
	}
	actualPlayers := b.observer.playerStateMessage.GetPlayersState()

	// verify player in each seat and their stack
	for i, expected := range expectedPlayers {
		actual := actualPlayers[i]
		if actual.PlayerId != expected.PlayerID {
			e := fmt.Errorf("[%s section] Expected player (%v) actual player (%v)",
				where, expected, actual)
			//g.result.addError(e)
			return e
		}

		if actual.GetCurrentBalance() != expected.Stack {
			e := fmt.Errorf("[%s section] Player %d stack does not match. Expected: %f, actual: %f",
				where, actual.PlayerId, expected.Stack, actual.CurrentBalance)
			//g.result.addError(e)
			return e
		}
	}

	return nil
}

func (b *DriverBot) runHand(hand *game.Hand) error {
	b.noMoreActions = false
	b.currentHand = hand
	e := b.setupHand()
	if e != nil {
		return e
	}

	// deal hand
	b.observer.dealHand()

	// wait for the hand to be dealt
	<-b.waitCh

	// pre-flop actions
	e = b.preflopActions()
	if e != nil {
		return e
	}
	lastHandMessage := b.observer.lastHandMessage
	result := false
	if lastHandMessage.MessageType == "RESULT" {
		result = true
	}

	// flop
	e = b.flopActions()
	if e != nil {
		return e
	}
	lastHandMessage = b.observer.lastHandMessage
	result = false
	if lastHandMessage.MessageType == "RESULT" {
		result = true
	}

	// turn
	e = b.turnActions()
	if e != nil {
		return e
	}
	lastHandMessage = b.observer.lastHandMessage
	result = false
	if lastHandMessage.MessageType == "RESULT" {
		result = true
	}

	// river
	e = b.riverActions()
	if e != nil {
		return e
	}
	lastHandMessage = b.observer.lastHandMessage
	result = false
	if lastHandMessage.MessageType == "RESULT" {
		result = true
	}

	// verify results
	if result {
		lastHandMessage = b.observer.lastHandMessage
		handResult := lastHandMessage.GetHandResult()
		_ = handResult

		err := b.verifyHandResult(handResult)
		if err != nil {
			return err
		}
	}

	_ = result

	return nil
}

func (b *DriverBot) setupHand() error {
	currentHand := b.currentHand

	playerCards := make([]poker.CardsInAscii, 0)
	for _, cards := range currentHand.Setup.SeatCards {
		playerCards = append(playerCards, cards.Cards)
	}
	// arrange deck
	deck := poker.DeckFromScript(playerCards,
		currentHand.Setup.Flop,
		poker.NewCard(currentHand.Setup.Turn),
		poker.NewCard(currentHand.Setup.River))

	// setup hand
	b.observer.setupNextHand(deck.GetBytes(), currentHand.Setup.ButtonPos)

	return nil
}

func (b *DriverBot) preflopActions() error {
	e := b.performBettingRound(&b.currentHand.PreflopAction)
	return e
}

func (b *DriverBot) flopActions() error {
	e := b.performBettingRound(&b.currentHand.FlopAction)
	return e
}

func (b *DriverBot) turnActions() error {
	e := b.performBettingRound(&b.currentHand.TurnAction)
	return e
}

func (b *DriverBot) riverActions() error {
	e := b.performBettingRound(&b.currentHand.RiverAction)
	return e
}

func (b *DriverBot) performBettingRound(bettingRound *game.BettingRound) error {
	if !b.noMoreActions {
		if bettingRound.SeatActions != nil {
			bettingRound.Actions = make([]game.TestHandAction, len(bettingRound.SeatActions))
			for i, actionStr := range bettingRound.SeatActions {
				// split the string
				s := strings.Split(strings.Trim(actionStr, " "), ",")
				if len(s) == 3 {
					seatNo, _ := strconv.Atoi(strings.Trim(s[0], " "))
					action := strings.Trim(s[1], " ")
					amount, _ := strconv.ParseFloat(strings.Trim(s[2], " "), 32)
					bettingRound.Actions[i] = game.TestHandAction{
						SeatNo: uint32(seatNo),
						Action: action,
						Amount: float32(amount),
					}
				} else if len(s) == 2 {
					seatNo, _ := strconv.Atoi(strings.Trim(s[0], " "))
					action := strings.Trim(s[1], " ")
					bettingRound.Actions[i] = game.TestHandAction{
						SeatNo: uint32(seatNo),
						Action: action,
					}
				} else {
					e := fmt.Errorf("Invalid action found: %s", bettingRound.SeatActions)
					return e
				}
			}
		}

		for _, action := range bettingRound.Actions {
			// get player id in the seat
			playerID := b.playersInSeats[action.SeatNo]
			playerBot := b.players[playerID]
			actionType := game.ACTION(game.ACTION_value[action.Action])
			playerBot.act(b.currentHand.Num, actionType, action.Amount)

			<-b.waitCh
		}
	}

	lastHandMessage := b.observer.lastHandMessage
	// if last hand message was no more downs, there will be no more actions from the players
	if lastHandMessage.MessageType == game.HandNoMoreActions {
		b.noMoreActions = true
		// wait for betting round message (flop, turn, river, showdown)
		<-b.waitCh
	} else if lastHandMessage.MessageType != "RESULT" {
		// wait for betting round message (flop, turn, river, showdown)
		<-b.waitCh
	}

	// verify next action is correct
	verify := bettingRound.Verify
	err := b.verifyBettingRound(&verify)
	if err != nil {
		return err
	}
	return nil
}

func (b *DriverBot) verifyBettingRound(verify *game.VerifyBettingRound) error {
	lastHandMessage := b.observer.lastHandMessage
	if verify.State != "" {
		if verify.State == "FLOP" {
			// make sure the hand state is set correctly
			if lastHandMessage.HandStatus != game.HandStatus_FLOP {
				//h.addError(fmt.Errorf("Expected hand status as FLOP Actual: %s", game.HandStatus_name[int32(lastHandMessage.HandStatus)]))
				return fmt.Errorf("Expected hand state as FLOP")
			}

			// verify the board has the correct cards
			if verify.Board != nil {
				flopMessage := b.observer.flop
				boardCardsFromGame := poker.ByteCardsToStringArray(flopMessage.Board)
				expectedCards := verify.Board
				if !reflect.DeepEqual(boardCardsFromGame, expectedCards) {
					e := fmt.Errorf("Flopped cards did not match with expected cards. Expected: %s actual: %s",
						poker.CardsToString(expectedCards), poker.CardsToString(flopMessage.Board))
					//h.addError(e)
					return e
				}
			}
		} else if verify.State == "TURN" {
			// make sure the hand state is set correctly
			if lastHandMessage.HandStatus != game.HandStatus_TURN {
				//h.addError(fmt.Errorf("Expected hand status as TURN Actual: %s", game.HandStatus_name[int32(lastHandMessage.HandStatus)]))
				return fmt.Errorf("Expected hand state as TURN")
			}

			// verify the board has the correct cards
			if verify.Board != nil {
				turnMessage := b.observer.turn
				boardCardsFromGame := poker.ByteCardsToStringArray(turnMessage.Board)
				expectedCards := verify.Board
				if !reflect.DeepEqual(boardCardsFromGame, expectedCards) {
					e := fmt.Errorf("Flopped cards did not match with expected cards. Expected: %s actual: %s",
						poker.CardsToString(expectedCards), poker.CardsToString(turnMessage.Board))
					//h.addError(e)
					return e
				}
			}
		} else if verify.State == "RESULT" {
			if lastHandMessage.MessageType != "RESULT" {
				//h.addError(fmt.Errorf("Expected result after preflop actions. Actual message: %s", lastHandMessage.MessageType))
				return fmt.Errorf("Failed at preflop verification step")
			}
		}
	}

	if verify.Pots != nil {
		// get pot information from the observer
		gamePots := b.observer.actionChange.GetActionChange().SeatsPots
		if b.observer.noMoreActions != nil {
			gamePots = b.observer.noMoreActions.GetNoMoreActions().Pots
		}

		if len(verify.Pots) != len(gamePots) {
			e := fmt.Errorf("Pot count does not match. Expected: %d actual: %d", len(verify.Pots), len(gamePots))
			//h.gameScript.result.addError(e)
			return e
		}

		for i, expectedPot := range verify.Pots {
			actualPot := gamePots[i]
			if expectedPot.Pot != actualPot.Pot {
				e := fmt.Errorf("Pot [%d] amount does not match. Expected: %f actual: %f",
					i, expectedPot.Pot, actualPot.Pot)
				//h.gameScript.result.addError(e)
				return e
			}

			if expectedPot.SeatsInPot != nil {
				// verify the seats are in the pot
				for _, seatNo := range expectedPot.SeatsInPot {
					found := false
					for _, actualSeat := range actualPot.Seats {
						if actualSeat == seatNo {
							found = true
							break
						}
					}
					if !found {
						e := fmt.Errorf("Pot [%d] seat %d is not in the pot", i, seatNo)
						//h.gameScript.result.addError(e)
						return e
					}
				}
			}
		}
	}

	return nil
}

func (b *DriverBot) verifyHandResult(handResult *game.HandResult) error {
	passed := true
	var e error
	for i, expectedWinner := range b.currentHand.Result.Winners {
		potWinner := handResult.HandLog.PotWinners[uint32(i)]
		winners := potWinner.GetHiWinners()
		if len(winners) != 1 {
			passed = false
		}
		handWinner := winners[0]
		if handWinner.SeatNo != expectedWinner.Seat {
			e = fmt.Errorf("Winner seat no didn't match. Expected %d, actual: %d",
				expectedWinner.Seat, handWinner.SeatNo)
			passed = false
		}

		if handWinner.Amount != expectedWinner.Receive {
			e = fmt.Errorf("Winner winning didn't match. Expected %f, actual: %f",
				expectedWinner.Receive, handWinner.Amount)
			passed = false
		}
	}

	if b.currentHand.Result.ActionEndedAt != "" {
		actualActionEndedAt := game.HandStatus_name[int32(handResult.HandLog.WonAt)]
		if b.currentHand.Result.ActionEndedAt != actualActionEndedAt {
			e = fmt.Errorf("Action won at is not matching. Expected %s, actual: %s",
				b.currentHand.Result.ActionEndedAt, actualActionEndedAt)
			passed = false
		}
	}

	_ = e

	// now verify players stack
	expectedStacks := b.currentHand.Result.Stacks
	for _, expectedStack := range expectedStacks {
		for seatNo, player := range handResult.Players {
			if seatNo == expectedStack.Seat {
				if player.Balance.After != expectedStack.Stack {
					e := fmt.Errorf("Player %d seatNo: %d is not matching. Expected %f, actual: %f", player.Id, seatNo,
						expectedStack.Stack, player.Balance.After)
					_ = e
					passed = false
				}
			}
		}
	}

	if !passed {
		return fmt.Errorf("Failed when verifying the hand result")
	}
	return nil
}
