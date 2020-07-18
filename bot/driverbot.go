package bot

import (
	"fmt"
	"io/ioutil"

	"voyager.com/server/game"
	"voyager.com/server/poker"

	jsoniter "github.com/json-iterator/go"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
)

var driverBotLogger = log.With().Str("logger_name", "server::driverbot").Logger()

const NatsURL = "nats://localhost:4222"
const BotDriverToGame = "driverbot.2game"
const GameToBotDriver = "game.2driverpot"
const botPlayerID = 0xFFFFFFFF

// bot driver messages to game
const (
	BotDriverInitializeGame = "B2GInitializeGame"
	BotDriverStartGame      = "B2GStartGame"
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
	GameNum     uint32           `json:"game-num"`
}

type DriverBot struct {
	botId          string
	stopped        chan bool
	players        map[uint32]*PlayerBot
	currentHand    *game.Hand
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
		players:        make(map[uint32]*PlayerBot),
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
	observer.joinGame(message.ClubId, message.GameNum)
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
		e = botPlayer.joinGame(message.ClubId, message.GameNum)
		if e != nil {
			driverBotLogger.Error().Msg(fmt.Sprintf("Error occurred when bot player joing game. %d:%d", message.ClubId, message.GameNum))
			return e
		}
	}

	for _, playerSeat := range b.gameScript.AssignSeat.Seats {
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
	b.currentHand = hand
	e := b.setupHand()
	if e != nil {
		return e
	}

	// deal hand
	b.observer.dealHand()

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
