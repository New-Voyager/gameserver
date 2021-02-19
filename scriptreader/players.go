package scriptreader

import (
	"fmt"
	"io/ioutil"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// Players contains players YAML content.
type Players struct {
	Players []Player `yaml:"players"`
}

// Player contains a player entry in the players array.
type Player struct {
	Name     string
	DeviceID string `yaml:"deviceId"`
	Email    string
	Password string
}

// ReadPlayersConfig reads players configuration yaml file.
func ReadPlayersConfig(fileName string) (*Players, error) {
	bytes, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, errors.Wrapf(err, "Error reading players configuration file [%s]", fileName)
	}

	var data Players
	err = yaml.Unmarshal(bytes, &data)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("Error parsing YAML file [%s]", fileName))
	}

	return &data, nil
}
