package msgcheck

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"sync"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"
	"voyager.com/botrunner/internal/game"
)

// MsgCollector collects game/hand messages from different players and compares them against the
// expected messages.
type MsgCollector struct {
	// Key: player name
	gameMsgs     map[string][]gameMsg
	handMsgs     map[string][]handMsg
	expectedMsgs *expectedMsgs

	gameMsgsLock sync.RWMutex
	handMsgsLock sync.RWMutex
}

type gameMsg struct {
	msg *game.GameMessage
	raw []byte
}

type handMsg struct {
	msg *game.HandMessage
	raw []byte
}

// For parsing the expected messages json file.
type expectedMsgsJSON struct {
	GameMsgs map[string][]string `json:"gameMsgs"`
	HandMsgs map[string][]string `json:"handMsgs"`
}

type expectedMsgs struct {
	gameMsgs map[string][]*game.GameMessage
	handMsgs map[string][]*game.HandMessage
}

// NewMsgCollector creates an instance of MsgCollector.
func NewMsgCollector(expectedMsgFile string) (*MsgCollector, error) {
	var expectedMsgs *expectedMsgs
	var err error
	if expectedMsgFile != "" {
		expectedMsgs, err = parseExpectedMsgsFile(expectedMsgFile)
		if err != nil {
			return nil, err
		}
	}
	mc := MsgCollector{
		gameMsgs:     make(map[string][]gameMsg),
		handMsgs:     make(map[string][]handMsg),
		expectedMsgs: expectedMsgs,
	}
	return &mc, nil
}

func parseExpectedMsgsFile(expectedMsgsFile string) (*expectedMsgs, error) {
	jsonFile, err := os.Open(expectedMsgsFile)
	if err != nil {
		return nil, err
	}
	defer jsonFile.Close()
	byteValue, _ := ioutil.ReadAll(jsonFile)
	var result expectedMsgsJSON
	json.Unmarshal(byteValue, &result)
	ret := expectedMsgs{
		gameMsgs: make(map[string][]*game.GameMessage),
		handMsgs: make(map[string][]*game.HandMessage),
	}
	for key, rawMsgs := range result.GameMsgs {
		ret.gameMsgs[key] = make([]*game.GameMessage, 0)
		for i, rawMsg := range rawMsgs {
			var message game.GameMessage
			err := protojson.Unmarshal([]byte(rawMsg), &message)
			if err != nil {
				return nil, errors.Wrapf(err, "Unable to parse expected game message %d for player %s", i, key)
			}
			ret.gameMsgs[key] = append(ret.gameMsgs[key], &message)
		}
	}
	for key, rawMsgs := range result.HandMsgs {
		ret.handMsgs[key] = make([]*game.HandMessage, 0)
		for i, rawMsg := range rawMsgs {
			var message game.HandMessage
			err := protojson.Unmarshal([]byte(rawMsg), &message)
			if err != nil {
				return nil, errors.Wrapf(err, "Unable to parse expected hand message %d for player %s", i, key)
			}
			ret.handMsgs[key] = append(ret.handMsgs[key], &message)
		}
	}
	return &ret, nil
}

// AddGameMsg appends the game message to the collection.
func (mc *MsgCollector) AddGameMsg(key string, msg *game.GameMessage, rawMsg []byte) {
	mc.gameMsgsLock.Lock()
	defer mc.gameMsgsLock.Unlock()

	_, exists := mc.gameMsgs[key]
	if !exists {
		mc.gameMsgs[key] = make([]gameMsg, 0)
	}
	mc.gameMsgs[key] = append(mc.gameMsgs[key], gameMsg{
		msg: msg,
		raw: rawMsg,
	})
}

// AddHandMsg appends the hand message to the collection.
func (mc *MsgCollector) AddHandMsg(key string, msg *game.HandMessage, rawMsg []byte) {
	mc.handMsgsLock.Lock()
	defer mc.handMsgsLock.Unlock()

	_, exists := mc.handMsgs[key]
	if !exists {
		mc.handMsgs[key] = make([]handMsg, 0)
	}
	mc.handMsgs[key] = append(mc.handMsgs[key], handMsg{
		msg: msg,
		raw: rawMsg,
	})
}

// Verify compares collected messages against the expected messages and returns an error if there is a mismatch.
func (mc *MsgCollector) Verify() error {
	for playerName, msgs := range mc.gameMsgs {
		fmt.Printf("Verifying game messages for player [%s].\n", playerName)
		expectedMsgs := mc.expectedMsgs.gameMsgs[playerName]
		numMsgs := len(msgs)
		if numMsgs != len(expectedMsgs) {
			return fmt.Errorf("Number of game messages [%d] does not match the expected [%d]", len(msgs), len(expectedMsgs))
		}
		for i := 0; i < numMsgs; i++ {
			var expectedMsg *game.GameMessage = expectedMsgs[i]
			var actualMsg *game.GameMessage = msgs[i].msg
			// TODO: Compare other fields as well based on the message type.
			if actualMsg.GetMessageType() != expectedMsg.GetMessageType() {
				return fmt.Errorf("Message type mismatch for player [%s] game message %d. Expected: [%s] Actual: [%s]", playerName, i, expectedMsg.GetMessageType(), actualMsg.GetMessageType())
			}
		}
	}
	for playerName, msgs := range mc.handMsgs {
		fmt.Printf("Verifying hand messages for player [%s].\n", playerName)
		expectedMsgs := mc.expectedMsgs.handMsgs[playerName]
		numMsgs := len(msgs)
		if numMsgs != len(expectedMsgs) {
			return fmt.Errorf("Number of hand messages [%d] does not match the expected [%d]", len(msgs), len(expectedMsgs))
		}
		for i := 0; i < numMsgs; i++ {
			var expectedMsg *game.HandMessage = expectedMsgs[i]
			var actualMsg *game.HandMessage = msgs[i].msg
			numMsgItems := len(expectedMsg.GetMessages())
			for j := 0; j < numMsgItems; j++ {
				expectedMsgItem := expectedMsg.GetMessages()[j]
				actualMsgItem := actualMsg.GetMessages()[j]
				// TODO: Compare other fields as well based on the message type.
				if actualMsgItem.MessageType != expectedMsgItem.MessageType {
					return fmt.Errorf("Message type mismatch for player [%s] hand message %d item %d. Expected: [%s] Actual: [%s]", playerName, i, j, expectedMsgItem.MessageType, actualMsgItem.MessageType)
				}
			}
		}
	}
	return nil
}

// ToPrettyJSONString converts all collected game/hand messages from all players into
// a pretty json string for printing out to a file, etc.
func (mc *MsgCollector) ToPrettyJSONString() (string, error) {
	mc.gameMsgsLock.RLock()
	mc.handMsgsLock.RLock()
	defer mc.gameMsgsLock.RUnlock()
	defer mc.handMsgsLock.RUnlock()

	// Use only the raw message strings.
	gameMsgs := make(map[string][]string)
	for key := range mc.gameMsgs {
		gameMsgs[key] = make([]string, 0)
		for _, msgs := range mc.gameMsgs[key] {
			gameMsgs[key] = append(gameMsgs[key], string(msgs.raw))
		}
	}
	handMsgs := make(map[string][]string)
	for key := range mc.handMsgs {
		handMsgs[key] = make([]string, 0)
		for _, msgs := range mc.handMsgs[key] {
			handMsgs[key] = append(handMsgs[key], string(msgs.raw))
		}
	}
	d := expectedMsgsJSON{
		GameMsgs: gameMsgs,
		HandMsgs: handMsgs,
	}
	jsonData, err := json.MarshalIndent(d, "", "    ")
	if err != nil {
		return "", errors.Wrap(err, "Error while serializing collected msgs into json")
	}
	return string(jsonData), nil
}
