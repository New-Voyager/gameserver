package bot

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"voyager.com/server/game"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
)

var botPlayerLogger = log.With().Str("logger_name", "bot::player").Logger()

type PlayerBot struct {
	botID                  string
	stopped                chan bool
	playerID               uint32
	clubID                 uint32
	gameID                 uint64
	player2GameSubject     string
	game2PlayerSubject     string
	game2AllPlayersSubject string
	player2HandSubject     string
	hand2PlayerSubject     string
	hand2PlayerAllSubject  string
	seatNo                 uint32
	playerAtSit            bool
	// players cards
	cards []uint32
	// current hand message
	currentHand *game.HandMessage

	// preserve different stages of the messages
	flop          *game.Flop
	turn          *game.Turn
	river         *game.River
	showdown      *game.Showdown
	actionChange  *game.HandMessage
	noMoreActions *game.HandMessage

	observer           bool
	lastGameMessage    *game.GameMessage
	lastHandMessage    *game.HandMessage
	playerStateMessage *game.GameTableStateMessage

	waitObserverCh     chan int
	nc                 *natsgo.Conn
	game2PlayerSub     *natsgo.Subscription
	hand2PlayerSub     *natsgo.Subscription
	game2AllPlayersSub *natsgo.Subscription
	hand2AllSub        *natsgo.Subscription
}

func NewPlayerBot(natsUrl string, playerID uint32) (*PlayerBot, error) {
	nc, err := natsgo.Connect(natsUrl)
	if err != nil {
		driverBotLogger.Error().Msg(fmt.Sprintf("Error connecting to NATS server, error: %v", err))
		return nil, err
	}

	botUuid := uuid.New()
	playerBot := &PlayerBot{
		botID:    botUuid.String(),
		stopped:  make(chan bool),
		playerID: playerID,
		nc:       nc,
	}

	return playerBot, nil
}

func (p *PlayerBot) setObserver(waitObserverCh chan int) {
	p.observer = true
	p.waitObserverCh = waitObserverCh
}

func (p *PlayerBot) game2Player(msg *natsgo.Msg) {
	botPlayerLogger.Info().Msg(fmt.Sprintf("Message from %s", string(msg.Data)))

	var message game.GameMessage
	err := protojson.Unmarshal(msg.Data, &message)
	if err == nil {
		p.lastGameMessage = &message

		switch message.MessageType {
		case game.PlayerSat:
			playerSatMsg := message.GetPlayerSat()
			if playerSatMsg.PlayerId == p.playerID {
				p.playerAtSit = true
			}
		case game.GameTableState:
			if message.PlayerId != 0 {
				if message.PlayerId == botPlayerID {
					p.playerStateMessage = message.GetTableState()
					p.waitObserverCh <- 1
				}
			}
		}
	}
}

func (p *PlayerBot) hand2Player(msg *natsgo.Msg) {
	botPlayerLogger.Info().Msg(fmt.Sprintf("Message from %s", string(msg.Data)))
	var message game.HandMessage
	err := protojson.Unmarshal(msg.Data, &message)
	if err == nil {
		// handle hand messages to the player
		switch message.MessageType {
		case game.HandNextAction:
			p.actionChange = &message
		case game.HandNoMoreActions:
			p.noMoreActions = &message
		case game.HandNewHand:
			p.currentHand = &message
			p.flop = nil
			p.turn = nil
			p.river = nil
			p.cards = nil
		case game.HandDeal:
			if message.PlayerId == p.playerID {
				p.cards = message.GetDealCards().Cards
			}
		case game.HandFlop:
			p.flop = message.GetFlop()
		case game.HandTurn:
			p.turn = message.GetTurn()
		case game.HandRiver:
			p.river = message.GetRiver()
		case game.HandShowDown:
			p.showdown = message.GetShowdown()
		}
		p.lastHandMessage = &message

		if p.observer {
			p.waitObserverCh <- 1
		}
	}
}

func (p *PlayerBot) initialize(clubID uint32, gameID uint64) {
	// game subjects
	p.player2GameSubject = fmt.Sprintf("game.%d.player", gameID)
	p.game2AllPlayersSubject = fmt.Sprintf("game.%d.allplayers", gameID)
	p.game2PlayerSubject = fmt.Sprintf("game.%d.player.%d", gameID, p.playerID)

	// hand subjects
	p.player2HandSubject = fmt.Sprintf("game.%d.hand.player", gameID)
	p.hand2PlayerSubject = fmt.Sprintf("game.%d.hand.player.%d", gameID, p.playerID)
	p.hand2PlayerAllSubject = fmt.Sprintf("game.%d.hand.all", gameID)

}

func (p *PlayerBot) joinGame(clubID uint32, gameID uint64) error {
	p.initialize(clubID, gameID)

	var e error
	p.game2PlayerSub, e = p.nc.Subscribe(p.game2PlayerSubject, p.game2Player)
	if e != nil {
		botPlayerLogger.Error().
			Msg(fmt.Sprintf("Subscription to %s failed. Error: %v", p.game2PlayerSubject, e))
		return e
	}
	p.hand2PlayerSub, e = p.nc.Subscribe(p.hand2PlayerSubject, p.hand2Player)
	if e != nil {
		p.game2PlayerSub.Unsubscribe()
		botPlayerLogger.Error().
			Msg(fmt.Sprintf("Subscription to %s failed. Error: %v", p.hand2PlayerSubject, e))
		return e
	}
	p.hand2AllSub, e = p.nc.Subscribe(p.hand2PlayerAllSubject, p.hand2Player)
	if e != nil {
		p.game2PlayerSub.Unsubscribe()
		botPlayerLogger.Error().
			Msg(fmt.Sprintf("Subscription to %s failed. Error: %v", p.hand2PlayerAllSubject, e))
		return e
	}
	p.game2AllPlayersSub, e = p.nc.Subscribe(p.game2AllPlayersSubject, p.game2Player)
	if e != nil {
		botPlayerLogger.Error().
			Msg(fmt.Sprintf("Subscription to %s failed. Error: %v", p.game2AllPlayersSubject, e))
		return e
	}

	// send a message to the game that this player is joining the game
	var gameMessage game.GameMessage
	gameMessage.ClubId = clubID
	gameMessage.GameId = gameID
	gameMessage.PlayerId = p.playerID
	gameMessage.MessageType = game.GameJoin
	name := fmt.Sprintf("bot-%d", p.playerID)
	joinGame := &game.GameJoinMessage{Name: name}
	gameMessage.GameMessage = &game.GameMessage_JoinGame{JoinGame: joinGame}
	protoData, err := protojson.Marshal(&gameMessage)
	fmt.Printf("proto: %s\n", string(protoData))

	if err != nil {
		p.hand2PlayerSub.Unsubscribe()
		p.game2PlayerSub.Unsubscribe()
		botPlayerLogger.Error().
			Msg(fmt.Sprintf("Could not join game. Error: %v", e))
		return e
	}

	// send to game channel/subject
	p.nc.Publish(p.player2GameSubject, protoData)

	p.clubID = clubID
	p.gameID = gameID
	botPlayerLogger.Info().Msg(fmt.Sprintf("Player %d joined %d: %d", p.playerID, clubID, gameID))
	return nil
}

func (p *PlayerBot) sitAtTable(seatNo uint32, buyIn float32) error {
	var message game.GameMessage
	message.ClubId = p.clubID
	message.GameId = p.gameID
	message.MessageType = game.PlayerTakeSeat
	message.PlayerId = p.playerID
	p.seatNo = seatNo
	sitMessage := &game.GameSitMessage{PlayerId: p.playerID, SeatNo: seatNo, BuyIn: buyIn}
	// only club owner/manager can start a game
	message.GameMessage = &game.GameMessage_TakeSeat{TakeSeat: sitMessage}
	protoData, err := protojson.Marshal(&message)
	fmt.Printf("proto: %s\n", string(protoData))
	if err != nil {
		botPlayerLogger.Error().
			Msg(fmt.Sprintf("Could not sit at table. Error: %v", err))
		return err
	}

	// send to game channel/subject
	p.nc.Publish(p.player2GameSubject, protoData)

	return err
}

func (p *PlayerBot) getTableState() error {
	queryTableState := &game.GameQueryTableStateMessage{PlayerId: p.playerID}
	var gameMessage game.GameMessage
	gameMessage.ClubId = p.clubID
	gameMessage.GameId = p.gameID
	gameMessage.PlayerId = p.playerID
	gameMessage.MessageType = game.GameQueryTableState
	gameMessage.GameMessage = &game.GameMessage_QueryTableState{QueryTableState: queryTableState}

	protoData, err := protojson.Marshal(&gameMessage)
	fmt.Printf("proto: %s\n", string(protoData))
	if err != nil {
		botPlayerLogger.Error().
			Msg(fmt.Sprintf("Could not sit at table. Error: %v", err))
		return err
	}

	// send to game channel/subject
	p.nc.Publish(p.player2GameSubject, protoData)
	return nil
}

func (p *PlayerBot) setupNextHand(deck []byte, buttonPos uint32) error {
	var gameMessage game.GameMessage

	nextHand := &game.GameSetupNextHandMessage{
		Deck:      deck,
		ButtonPos: buttonPos,
	}

	gameMessage.ClubId = p.clubID
	gameMessage.GameId = p.gameID
	gameMessage.PlayerId = p.playerID
	gameMessage.MessageType = game.GameSetupNextHand
	gameMessage.GameMessage = &game.GameMessage_NextHand{NextHand: nextHand}
	protoData, err := protojson.Marshal(&gameMessage)
	fmt.Printf("proto: %s\n", string(protoData))
	if err != nil {
		botPlayerLogger.Error().
			Msg(fmt.Sprintf("Could not setup hand. Error: %v", err))
		return err
	}

	// send to game channel/subject
	e := p.nc.Publish(p.player2GameSubject, protoData)
	return e
}

func (p *PlayerBot) dealHand() error {
	var gameMessage game.GameMessage

	dealHandMessage := &game.GameDealHandMessage{}

	gameMessage.ClubId = p.clubID
	gameMessage.GameId = p.gameID
	gameMessage.MessageType = game.GameDealHand
	gameMessage.GameMessage = &game.GameMessage_DealHand{DealHand: dealHandMessage}
	protoData, err := protojson.Marshal(&gameMessage)
	fmt.Printf("proto: %s\n", string(protoData))
	if err != nil {
		botPlayerLogger.Error().
			Msg(fmt.Sprintf("Could not setup deal hand. Error: %v", err))
		return err
	}

	// send to game channel/subject
	e := p.nc.Publish(p.player2GameSubject, protoData)
	return e
}

func (p *PlayerBot) act(handNum uint32, action game.ACTION, amount float32) error {
	// send handmessage
	message := game.HandMessage{
		ClubId:      p.clubID,
		GameId:      p.gameID,
		HandNum:     handNum,
		PlayerId:    p.playerID,
		MessageType: game.HandPlayerActed,
	}
	handAction := game.HandAction{SeatNo: p.seatNo, Action: action, Amount: amount}
	message.HandMessage = &game.HandMessage_PlayerActed{PlayerActed: &handAction}

	protoData, err := protojson.Marshal(&message)
	fmt.Printf("proto: %s\n", string(protoData))
	if err != nil {
		botPlayerLogger.Error().
			Msg(fmt.Sprintf("Could not setup deal hand. Error: %v", err))
		return err
	}

	// send to game channel/subject
	e := p.nc.Publish(p.player2HandSubject, protoData)

	return e
}
