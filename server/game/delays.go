package game

import (
	"fmt"
	"io/ioutil"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

type Delays struct {
	BeforeDeal          uint32 `yaml:"beforeDeal"`
	DealSingleCard      uint32 `yaml:"dealSingleCard"`
	PlayerActed         uint32 `yaml:"playerActed"`
	GoToFlop            uint32 `yaml:"goToFlop"`
	GoToTurn            uint32 `yaml:"goToTurn"`
	GoToRiver           uint32 `yaml:"goToRiver"`
	PendingUpdatesRetry uint32 `yaml:"pendingUpdatesRetry"`
	OnMoveToNextHand    uint32 `yaml:"onMoveToNextHand"`
	MoveToNextHand      uint32 `yaml:"moveToNextHand"`
	ResultPerWinner     uint32 `yaml:"resultPerWinner"`
}

func ParseDelayConfig(delaysFile string) (Delays, error) {
	bytes, err := ioutil.ReadFile(delaysFile)
	if err != nil {
		return Delays{}, errors.Wrap(err, fmt.Sprintf("Error reading delay config file [%s]", delaysFile))
	}

	var data Delays
	err = yaml.Unmarshal(bytes, &data)
	if err != nil {
		return Delays{}, errors.Wrap(err, fmt.Sprintf("Error parsing delays YAML file [%s]", delaysFile))
	}

	return data, nil
}
