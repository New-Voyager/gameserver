package game

import (
	"fmt"

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

type Game struct {
	clubID           uint32
	gameNum          uint32
	gameStatePersist PersistGameState
	handStatePersist PersistHandState
	state            *GameState
}

func NewGame(clubID uint32, gameStatePersist PersistGameState, handStatePersist PersistHandState) (*Game, error) {
	playersState := make(map[uint32]*PlayerState)

	playersState[1000] = &PlayerState{BuyIn: 100, CurrentBalance: 100, Status: PlayerState_PLAYING}
	playersState[1001] = &PlayerState{BuyIn: 200, CurrentBalance: 200, Status: PlayerState_PLAYING}
	playersState[1002] = &PlayerState{BuyIn: 200, CurrentBalance: 200, Status: PlayerState_PLAYING}
	playersState[1003] = &PlayerState{BuyIn: 100, CurrentBalance: 100, Status: PlayerState_PLAYING}
	playersState[1004] = &PlayerState{BuyIn: 150, CurrentBalance: 150, Status: PlayerState_PLAYING}

	gameNum := gameStatePersist.NextGameId(clubID)

	gameState := GameState{
		ClubId:                clubID,
		GameNum:               gameNum,
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
		clubID:           clubID,
		gameNum:          gameNum,
		gameStatePersist: gameStatePersist,
		handStatePersist: handStatePersist,
		state:            &gameState,
	}

	gameStatePersist.Save(clubID, gameNum, &gameState)
	return game, nil
}

func LoadGame(clubID uint32, gameNum uint32, gameStatePersist PersistGameState, handStatePersist PersistHandState) (*Game, error) {
	gameState, err := gameStatePersist.Load(clubID, gameNum)
	if err != nil {
		gameLogger.Error().Msg(fmt.Sprintf("Game %d is not found", gameNum))
		// we need to try to load from redis cache here
		return nil, fmt.Errorf(fmt.Sprintf("Game %d is not found", gameNum))
	}
	return &Game{
		clubID:           clubID,
		gameNum:          gameNum,
		state:            gameState,
		gameStatePersist: gameStatePersist,
		handStatePersist: handStatePersist,
	}, nil
}

func (g *Game) DealNextHand() (*HandState, uint64) {
	g.state.HandNum++

	// TODO: we need to add club number to the unique id
	handID := uint64(uint64(g.state.GameNum<<32) | uint64(g.state.HandNum))
	handState := HandState{
		ClubId:  g.state.ClubId,
		GameNum: g.state.GetGameNum(),
		HandNum: g.state.GetHandNum(),
	}

	handState.initialize(g)

	// TODO: Store in redis
	// save the hand state
	g.handStatePersist.Save(g.clubID, g.gameNum, handState.HandNum, &handState)

	g.state.ButtonPos = handState.GetButtonPos()
	// save the game
	g.gameStatePersist.Save(g.clubID, g.gameNum, g.state)

	_ = handState
	return &handState, handID
}

func (g *Game) LoadHand(handNum uint32) (*HandState, error) {
	handState, err := LoadHandState(g.handStatePersist, g.clubID, g.gameNum, handNum)
	return handState, err
}
