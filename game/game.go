package game

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/rs/zerolog/log"
)

/**
NOTE: Seat numbers are indexed from 1-9 like the real poker table.
**/

var gameLogger = log.With().Str("logger_name", "game::game").Logger()

var players = map[uint32]string{
	1000: "rob",
	1001: "steve",
	1002: "arun",
	1003: "bob",
	1004: "jacob",
}

// this should be (club num + game num + hand num)
var uniqueHandId = 1

//var runningGames = map[uint64]*GameState{}
//var runningHands = map[uint64]*HandState{}

var runningGames = map[uint64][]byte{}
var runningHands = map[uint64][]byte{}

type Game struct {
	state *GameState
}

func NewGame() (*Game, uint64) {
	playersState := make(map[uint32]*PlayerState)

	playersState[1000] = &PlayerState{BuyIn: 100, CurrentBalance: 100, Status: PlayerState_PLAYING}
	playersState[1001] = &PlayerState{BuyIn: 200, CurrentBalance: 200, Status: PlayerState_PLAYING}
	playersState[1002] = &PlayerState{BuyIn: 200, CurrentBalance: 200, Status: PlayerState_PLAYING}
	playersState[1003] = &PlayerState{BuyIn: 100, CurrentBalance: 100, Status: PlayerState_PLAYING}
	playersState[1004] = &PlayerState{BuyIn: 150, CurrentBalance: 150, Status: PlayerState_PLAYING}

	runningGamesLen := uint64(len(runningGames))
	gameState := GameState{
		GameNum:               runningGamesLen + 1,
		PlayersInSeats:        []uint32{1000, 0, 1001, 0, 1002, 1003, 0, 1004, 0},
		PlayersState:          playersState,
		UtgStraddleAllowed:    false,
		ButtonStraddleAllowed: false,
		Status:                GameState_RUNNING,
		GameType:              GameState_HOLDEM,
		HandNum:               0,
		ButtonPos:             5,
		SmallBlind:            1.0,
		BigBlind:              2.0,
		MaxSeats:              9,
	}

	game := &Game{
		state: &gameState,
	}
	game.save()
	return game, gameState.GameNum
}

func LoadGame(gameNum uint64) (Game, error) {
	if _, ok := runningGames[gameNum]; !ok {
		gameLogger.Error().Msg(fmt.Sprintf("Game %d is not found", gameNum))

		// we need to try to load from redis cache here
		return Game{}, fmt.Errorf(fmt.Sprintf("Game %d is not found", gameNum))
	}
	stateInBytes, _ := runningGames[gameNum]

	gameState := &GameState{}
	err := proto.Unmarshal(stateInBytes, gameState)
	if err != nil {
		panic("Error occured when unmarshalling game state")
	}
	return Game{
		state: gameState,
	}, nil
}

func (g *Game) save() error {
	var err error
	runningGames[g.state.GetGameNum()], err = proto.Marshal(g.state)
	if err != nil {
		return err
	}

	return nil
}

func (g *Game) DealNextHand() (HandState, uint64) {
	g.state.HandNum++

	// TODO: we need to add club number to the unique id
	handID := uint64(uint64(g.state.GameNum<<32) | uint64(g.state.HandNum))
	handState := HandState{
		UniqueHandId: handID,
		GameNum:      g.state.GetGameNum(),
		HandNum:      g.state.GetHandNum(),
	}

	handState.initialize(g)
	// store hand state in memory
	handStateBytes, err := proto.Marshal(&handState)
	if err != nil {
		panic("Handstate couldn't be marshalled")
	}
	// TODO: Store in redis
	runningHands[handID] = handStateBytes
	g.state.ButtonPos = handState.GetButtonPos()

	// save the game
	g.save()

	_ = handState
	return handState, handID
}
