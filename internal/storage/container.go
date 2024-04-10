package storage

import (
	"github.com/pablintino/automation-executor/internal/config"
	"github.com/spf13/afero"
)

type Container struct {
	fs               afero.Fs
	artifactsFs      afero.Fs
	ArtifactsScanner ArtifactsScanner
}

func NewContainer(artifactsConfig *config.ArtifactsConfig) *Container {
	fs := afero.NewOsFs()
	artfactsFs := afero.NewBasePathFs(fs, artifactsConfig.StoragePath)
	return &Container{
		fs:               fs,
		artifactsFs:      artfactsFs,
		ArtifactsScanner: NewArtifactsScanner(artifactsConfig),
	}
}
