package game

import (
	"fmt"

	"github.com/golang/protobuf/proto"
)

type MemoryHandStateTracker struct {
	activeHands map[string][]byte
}

func NewMemoryHandStateTracker() (*MemoryHandStateTracker, error) {
	return &MemoryHandStateTracker{
		activeHands: make(map[string][]byte),
	}, nil
}

func (m *MemoryHandStateTracker) Load(gameCode string) (*HandState, error) {
	return m.load(gameCode)
}

func (m *MemoryHandStateTracker) load(key string) (*HandState, error) {
	if handStateBytes, ok := m.activeHands[key]; ok {
		handState := HandState{}
		err := proto.Unmarshal(handStateBytes, &handState)
		if err != nil {
			return nil, err
		}
		return &handState, nil
	}
	return nil, fmt.Errorf("Hand state for Key: %s is not found", key)
}

func (m *MemoryHandStateTracker) Save(gameCode string, state *HandState) error {
	return m.save(gameCode, state)
}

func (m *MemoryHandStateTracker) save(key string, state *HandState) error {
	stateInBytes, err := proto.Marshal(state)
	if err != nil {
		return err
	}
	m.activeHands[key] = stateInBytes
	return nil
}

func (m *MemoryHandStateTracker) Remove(gameCode string) error {
	return m.remove(gameCode)
}

func (m *MemoryHandStateTracker) remove(key string) error {
	if _, ok := m.activeHands[key]; ok {
		delete(m.activeHands, key)
	}

	return nil
}
