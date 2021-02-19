package scriptreader

import (
	"fmt"
	"io/ioutil"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// Clubs contains clubs YAML content.
type Clubs struct {
	Clubs []Club `yaml:"clubs"`
}

// Club contains a club entry in the clubs array.
type Club struct {
	Name        string
	Description string
	Owner       string
	Members     []string
}

// ReadClubsConfig reads clubs configuration yaml file.
func ReadClubsConfig(fileName string) (*Clubs, error) {
	bytes, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, errors.Wrapf(err, "Error reading clubs configuration file [%s]", fileName)
	}

	var data Clubs
	err = yaml.Unmarshal(bytes, &data)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("Error parsing YAML file [%s]", fileName))
	}

	return &data, nil
}
