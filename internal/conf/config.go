package conf

import (
	"gopkg.in/yaml.v2"
	"os"
	"path/filepath"
)

type Config struct {
	Domain        string `yaml:"domain"`
	AuthServerURL string `yaml:"auth_server_url"`
}

// ParseConfig parses YAML configuration data and returns a Config struct.
// This function is designed for unit testing and doesn't perform any I/O.
func ParseConfig(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func LoadConfig() (*Config, error) {
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
		data, err = os.ReadFile(filepath.Clean(path))
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, err
	}

	return ParseConfig(data)
}
