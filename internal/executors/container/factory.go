package container

import (
	"errors"

	"github.com/google/uuid"
	"github.com/pablintino/automation-executor/internal/config"
	"github.com/pablintino/automation-executor/internal/executors/common"
)

type ContainerExecutorFactory struct {
	runtime         ContainerRuntime
	containerConfig *config.ContainerExecutorConfig
	imageBuilder    *imageProvider
}

type supportImageResolver interface {
	GetSupportImage() (string, error)
}

func NewContainerExecutorFactory(containerConfig *config.ContainerExecutorConfig) (*ContainerExecutorFactory, error) {
	runtime, err := getContainerRuntime(containerConfig)
	if err != nil {
		return nil, err
	}
	builder := NewImageProvider(containerConfig, runtime)
	if err := builder.Init(); err != nil {
		return nil, err
	}

	return &ContainerExecutorFactory{
		containerConfig: containerConfig,
		runtime:         runtime,
		imageBuilder:    builder,
	}, nil

}

func (f *ContainerExecutorFactory) GetExecutor(runId uuid.UUID, opts *common.ExecutorOpts) (common.Executor, error) {
	return NewContainerExecutor(f.containerConfig, f.runtime, f.imageBuilder, runId, opts)
}

func getContainerRuntime(containerConfig *config.ContainerExecutorConfig) (ContainerRuntime, error) {
	if containerConfig.Flavor == config.ContainerExecutorConfigFlavorValuePodman {
		containerRuntime, err := newPodmanRuntime(containerConfig)
		if err != nil {
			return nil, err
		}
		return containerRuntime, nil
	}
	return nil, errors.New("unsupported container runtime " + containerConfig.Flavor)
}
