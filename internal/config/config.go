package config

import (
	"errors"

	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

const (
	ExecutorConfigTypeValueContainer         = "container"
	ContainerExecutorConfigFlavorValuePodman = "podman"
)

type DatabaseConfig struct {
	DataSource string `koanf:"datasource"`
	Driver     string `koanf:"driver"`
}

type ShellConfig struct {
	Tracing      bool   `koanf:"enable-tracing"`
	ShellTimeout uint64 `koanf:"timeout"`
}

type ContainerExecutorConfig struct {
	Socket            string            `koanf:"socket"`
	SupportImage      string            `koanf:"support-image"`
	Flavor            string            `koanf:"flavor"`
	BuildSupportImage bool              `koanf:"build-support-image"`
	ExtraMounts       map[string]string `koanf:"extra-mounts"`
}

type ExecutorConfig struct {
	Type string `koanf:"type"`
}

type StorageConfig struct {
	ArtifactsPath  string `koanf:"artifacts-location"`
	WorkspacesPath string `koanf:"workspaces-location"`
	LoadSize       uint32 `koanf:"scanner-load-size"`
}

type Config struct {
	DatabaseConfig          DatabaseConfig          `koanf:"database"`
	StorageConfig           StorageConfig           `koanf:"storage"`
	ContainerExecutorConfig ContainerExecutorConfig `koanf:"executor-container"`
	ExecutorConfig          ExecutorConfig          `koanf:"executor"`
	ShellConfig             ShellConfig             `koanf:"shell"`
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
		"storage.scanner-load-size": uint32(4096),
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
