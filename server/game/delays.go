package game

import (
	"fmt"
	"io/ioutil"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

type Delays struct {
	PendingUpdatesRetry uint32 `yaml:"pendingUpdatesRetry"`
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
