package internal

import (
	"fmt"
	"io/ioutil"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

// TestCase contains one of the test cases in the yaml config.
type TestCase struct {
	Name    string        `yaml:"name"`
	Script  string        `yaml:"script"`
	Timeout time.Duration `yaml:"timeout"`
}

// TestConfig contains the config yaml content.
type TestConfig struct {
	Tests []TestCase `yaml:"tests"`
}

// ReadConfig reads the yaml config file and returns the parsed content.
func ReadConfig(configFile string) (*TestConfig, error) {
	bytes, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("Error reading config file [%s]", configFile))
	}

	var data TestConfig
	err = yaml.Unmarshal(bytes, &data)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("Error parsing YAML file [%s]", configFile))
	}

	return &data, nil
}
