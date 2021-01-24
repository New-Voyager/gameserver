package game

import (
	"fmt"

	"github.com/golang/protobuf/proto"
)

type MemoryGameStateTracker struct {
	activeGames map[string][]byte
	clubGames   map[uint32]uint64
}

type MemoryHandStateTracker struct {
	activeHands map[string][]byte
}

func NewMemoryGameStateTracker() *MemoryGameStateTracker {

	return &MemoryGameStateTracker{
		activeGames: make(map[string][]byte),
		clubGames:   make(map[uint32]uint64),
	}
}

func NewMemoryHandStateTracker() *MemoryHandStateTracker {
	return &MemoryHandStateTracker{
		activeHands: make(map[string][]byte),
	}
}

func (m *MemoryGameStateTracker) Load(clubID uint32, gameID uint64) (*GameState, error) {
	key := fmt.Sprintf("%d", gameID)
	if gameStateBytes, ok := m.activeGames[key]; ok {
		gameState := &GameState{}
		err := proto.Unmarshal(gameStateBytes, gameState)
		if err != nil {
			return nil, err
		}
		return gameState, nil
	}
	return nil, fmt.Errorf("Club: %d, Game: %d is not found", clubID, gameID)
}

func (m *MemoryGameStateTracker) Save(clubID uint32, gameID uint64, state *GameState) error {
	key := fmt.Sprintf("%d", gameID)
	stateInBytes, err := proto.Marshal(state)
	if err != nil {
		return err
	}
	m.activeGames[key] = stateInBytes
	return nil
}

func (m *MemoryGameStateTracker) Remove(clubID uint32, gameID uint64) error {
	key := fmt.Sprintf("%d", gameID)
	if _, ok := m.activeGames[key]; ok {
		delete(m.activeGames, key)
	}

	return nil
}

func (m *MemoryGameStateTracker) NextGameId(clubID uint32) (uint64, error) {
	if _, ok := m.clubGames[clubID]; !ok {
		m.clubGames[clubID] = 0
	}
	m.clubGames[clubID] = m.clubGames[clubID] + 1
	return m.clubGames[clubID], nil
}

func (m *MemoryHandStateTracker) Load(clubID uint32, gameID uint64, handID uint32) (*HandState, error) {
	key := fmt.Sprintf("%d:%d", gameID, handID)
	if handStateBytes, ok := m.activeHands[key]; ok {
		handState := HandState{}
		err := proto.Unmarshal(handStateBytes, &handState)
		if err != nil {
			return nil, err
		}
		return &handState, nil
	}
	return nil, fmt.Errorf("Club: %d, Game: %d, Hand: %d is not found", clubID, gameID, handID)
}

func (m *MemoryHandStateTracker) Save(clubID uint32, gameID uint64, handID uint32, state *HandState) error {
	key := fmt.Sprintf("%d:%d", gameID, handID)
	stateInBytes, err := proto.Marshal(state)
	if err != nil {
		return err
	}
	m.activeHands[key] = stateInBytes
	return nil
}

func (m *MemoryHandStateTracker) Remove(clubID uint32, gameID uint64, handID uint32) error {
	key := fmt.Sprintf("%d:%d", gameID, handID)
	if _, ok := m.activeHands[key]; ok {
		delete(m.activeHands, key)
	}

	return nil
}
