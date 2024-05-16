package container

import (
	"errors"
	"github.com/pablintino/automation-executor/internal/config"
	"os"
	"path"
)

const defaultImageName = "localhost/automation-executor/support-image:latest"

type imageProvider struct {
	containerExecConfig *config.ContainerExecutorConfig
	runtime             ContainerRuntime
	imageId             string
}

func NewImageProvider(containerExecConfig *config.ContainerExecutorConfig, runtime ContainerRuntime) *imageProvider {
	return &imageProvider{containerExecConfig: containerExecConfig, runtime: runtime}
}

func (b *imageProvider) Init() error {
	imageName := defaultImageName
	if b.containerExecConfig.SupportImage != "" {
		imageName = b.containerExecConfig.SupportImage
	}

	if b.containerExecConfig.BuildSupportImage {
		err := b.BuildImage(imageName)
		if err != nil {
			return err
		}
	}
	b.imageId = imageName
	return nil
}

func (b *imageProvider) BuildImage(imageName string) error {
	filePath, err := getContainerFilePath()
	if err != nil {
		return err
	}
	exists, err := b.runtime.ExistsImage(imageName)
	if err != nil || exists {
		return err
	}
	return b.runtime.Build(imageName, filePath)
}

func getContainerFilePath() (string, error) {
	binPath, err := os.Executable()
	if err != nil {
		return "", err
	}
	wdPath, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for _, location := range []string{path.Dir(binPath), wdPath} {
		containerFilePath := path.Join(location, "containerfiles", "Containerfile")
		if _, err := os.Stat(containerFilePath); !os.IsNotExist(err) {
			return containerFilePath, nil
		}
	}

	return "", errors.New("unable to find container file path")
}

func (b *imageProvider) GetSupportImage() (string, error) {
	return b.imageId, nil
}
