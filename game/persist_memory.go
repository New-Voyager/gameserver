package game

import (
	"fmt"
	"sync"

	"github.com/golang/protobuf/proto"
)

type MemoryGameStateTracker struct {
	activeGames map[string][]byte
	clubGames   map[uint32]uint64

	activeGamesLock sync.RWMutex
	clubGamesLock   sync.RWMutex
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

func (m *MemoryGameStateTracker) Load(gameCode string) (*GameState, error) {
	key := gameCode
	m.activeGamesLock.RLock()
	defer m.activeGamesLock.RUnlock()
	if gameStateBytes, ok := m.activeGames[key]; ok {
		gameState := &GameState{}
		err := proto.Unmarshal(gameStateBytes, gameState)
		if err != nil {
			return nil, err
		}
		return gameState, nil
	}
	return nil, fmt.Errorf("Game: %s is not found", gameCode)
}

func (m *MemoryGameStateTracker) Save(gameCode string, state *GameState) error {
	key := fmt.Sprintf("%s", gameCode)
	stateInBytes, err := proto.Marshal(state)
	if err != nil {
		return err
	}
	m.activeGamesLock.Lock()
	defer m.activeGamesLock.Unlock()
	m.activeGames[key] = stateInBytes
	return nil
}

func (m *MemoryGameStateTracker) Remove(gameCode string) error {
	key := fmt.Sprintf("%s", gameCode)
	m.activeGamesLock.Lock()
	defer m.activeGamesLock.Unlock()
	if _, ok := m.activeGames[key]; ok {
		delete(m.activeGames, key)
	}

	return nil
}

func (m *MemoryHandStateTracker) Load(gameCode string, handID uint32) (*HandState, error) {
	key := fmt.Sprintf("%s:%d", gameCode, handID)
	return m.load(key, gameCode, handID)
}

func (m *MemoryHandStateTracker) LoadClone(gameCode string, handID uint32) (*HandState, error) {
	key := fmt.Sprintf("%s:%d:clone", gameCode, handID)
	return m.load(key, gameCode, handID)
}

func (m *MemoryHandStateTracker) load(key string, gameCode string, handID uint32) (*HandState, error) {
	if handStateBytes, ok := m.activeHands[key]; ok {
		handState := HandState{}
		err := proto.Unmarshal(handStateBytes, &handState)
		if err != nil {
			return nil, err
		}
		return &handState, nil
	}
	return nil, fmt.Errorf("Game: %s, Hand: %d is not found", gameCode, handID)
}

func (m *MemoryHandStateTracker) Save(gameCode string, handID uint32, state *HandState) error {
	key := fmt.Sprintf("%s:%d", gameCode, handID)
	return m.save(key, state)
}

func (m *MemoryHandStateTracker) SaveClone(gameCode string, handID uint32, state *HandState) error {
	key := fmt.Sprintf("%s:%d:clone", gameCode, handID)
	return m.save(key, state)
}

func (m *MemoryHandStateTracker) save(key string, state *HandState) error {
	stateInBytes, err := proto.Marshal(state)
	if err != nil {
		return err
	}
	m.activeHands[key] = stateInBytes
	return nil
}

func (m *MemoryHandStateTracker) Remove(gameCode string, handID uint32) error {
	key := fmt.Sprintf("%s:%d", gameCode, handID)
	return m.remove(key)
}

func (m *MemoryHandStateTracker) RemoveClone(gameCode string, handID uint32) error {
	key := fmt.Sprintf("%s:clone", gameCode, handID)
	return m.remove(key)
}

func (m *MemoryHandStateTracker) remove(key string) error {
	if _, ok := m.activeHands[key]; ok {
		delete(m.activeHands, key)
	}

	return nil
}
