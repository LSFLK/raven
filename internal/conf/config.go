package conf

import (
	"gopkg.in/yaml.v2"
	"os"
	"path/filepath"
	"raven/internal/blobstorage"
)

type Config struct {
	Domain        string             `yaml:"domain"`
	AuthServerURL string             `yaml:"auth_server_url"`
	BlobStorage   blobstorage.Config `yaml:"blob_storage"`
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
		data, err = os.ReadFile(filepath.Clean(path))
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
