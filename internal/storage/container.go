package storage

import (
	"github.com/pablintino/automation-executor/internal/config"
)

type Container struct {
	StorageManager   Manager
	ArtifactsScanner ArtifactsScanner
}

func NewContainer(artifactsConfig *config.StorageConfig) (*Container, error) {
	manager, err := NewStorageManager(artifactsConfig)
	if err != nil {
		return nil, err
	}
	return &Container{
		StorageManager:   manager,
		ArtifactsScanner: NewArtifactsScanner(artifactsConfig),
	}, nil
}
