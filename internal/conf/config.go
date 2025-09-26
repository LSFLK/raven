package conf

import (
	"io/ioutil"
	"gopkg.in/yaml.v2"
)

type Config struct {
	Domain string `yaml:"domain"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
