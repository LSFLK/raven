package conf

import (
	"os"
	"gopkg.in/yaml.v2"
)

type Config struct {
	Domain        string `yaml:"domain"`
	AuthServerURL string `yaml:"auth_server_url"`
}

func LoadConfig() (*Config, error) {
	var cfg Config

	// Try multiple possible paths
	configPaths := []string{
		"/etc/raven/raven.yaml",
		"./config/raven.yaml",
		"./raven.yaml",
		"config/raven.yaml",
	}

	var data []byte
	var err error
	for _, path := range configPaths {
		data, err = os.ReadFile(path)
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}