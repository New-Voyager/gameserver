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
	gameNum                uint32
	player2GameSubject     string
	game2PlayerSubject     string
	game2AllPlayersSubject string
	player2HandSubject     string
	hand2PlayerSubject     string
	playerAtSit            bool

	observer           bool
	lastGameMessage    *game.GameMessage
	lastHandMessage    *game.HandMessage
	playerStateMessage *game.GameTableStateMessage

	waitObserverCh     chan int
	nc                 *natsgo.Conn
	game2PlayerSub     *natsgo.Subscription
	hand2PlayerSub     *natsgo.Subscription
	game2AllPlayersSub *natsgo.Subscription
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
	}
}

func (p *PlayerBot) initialize(clubID uint32, gameNum uint32) {
	// game subjects
	p.player2GameSubject = fmt.Sprintf("game.%d%d.player", clubID, gameNum)
	p.game2AllPlayersSubject = fmt.Sprintf("game.%d%d.allplayers", clubID, gameNum)
	p.game2PlayerSubject = fmt.Sprintf("game.%d%d.player.%d", clubID, gameNum, p.playerID)

	// hand subjects
	p.player2HandSubject = fmt.Sprintf("game.%d%d.hand.player", clubID, gameNum)
	p.hand2PlayerSubject = fmt.Sprintf("game.%d%d.hand.player.%d", clubID, gameNum, p.playerID)
}

func (p *PlayerBot) joinGame(clubID uint32, gameNum uint32) error {
	p.initialize(clubID, gameNum)

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
	p.game2AllPlayersSub, e = p.nc.Subscribe(p.game2AllPlayersSubject, p.game2Player)
	if e != nil {
		botPlayerLogger.Error().
			Msg(fmt.Sprintf("Subscription to %s failed. Error: %v", p.game2AllPlayersSubject, e))
		return e
	}

	// send a message to the game that this player is joining the game
	var gameMessage game.GameMessage
	gameMessage.ClubId = clubID
	gameMessage.GameNum = gameNum
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
	p.gameNum = gameNum
	botPlayerLogger.Info().Msg(fmt.Sprintf("Player %d joined %d: %d", p.playerID, clubID, gameNum))
	return nil
}

func (p *PlayerBot) sitAtTable(seatNo uint32, buyIn float32) error {
	var message game.GameMessage
	message.ClubId = p.clubID
	message.GameNum = p.gameNum
	message.MessageType = game.PlayerTakeSeat
	message.PlayerId = p.playerID

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
	gameMessage.GameNum = p.gameNum
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
	gameMessage.GameNum = p.gameNum
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
	gameMessage.GameNum = p.gameNum
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
