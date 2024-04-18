package config

import (
	"errors"

	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

type DatabaseConfig struct {
	DataSource string `koanf:"datasource"`
	Driver     string `koanf:"driver"`
}

type PodmanConfig struct {
	Socket string `koanf:"socket"`
}

type ArtifactsConfig struct {
	StoragePath string `koanf:"location"`
	LoadSize    uint32 `koanf:"load-size"`
}

type Config struct {
	DatabaseConfig  DatabaseConfig  `koanf:"database"`
	ArtifactsConfig ArtifactsConfig `koanf:"artifacts"`
	PodmanConfig    PodmanConfig    `koanf:"podman"`
}

func Configure() (*Config, error) {
	config := &Config{}
	err := loadConfig(config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func loadConfig(config *Config) error {
	koanfInstance := koanf.New(".")
	err := koanfInstance.Load(confmap.Provider(map[string]interface{}{
		"artifacts.load-size": uint32(4096),
	}, "."), nil)
	if err != nil {
		return err
	}
	err = koanfInstance.Load(file.Provider("/etc/automation-executor/config.toml"), toml.Parser())
	if errRel := koanfInstance.Load(file.Provider("config.toml"), toml.Parser()); errRel != nil && err != nil {
		return errors.New("unable to load service configuration from known locations")
	}
	return koanfInstance.Unmarshal("", config)
}
