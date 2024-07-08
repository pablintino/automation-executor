package executors

import (
	"errors"
	"go.uber.org/zap"

	"github.com/google/uuid"
	"github.com/pablintino/automation-executor/internal/config"
	"github.com/pablintino/automation-executor/internal/executors/common"
	"github.com/pablintino/automation-executor/internal/executors/container"
)

type ExecutorFactoryImpl struct {
	executorConfig  *config.ExecutorConfig
	containerConfig *config.ContainerExecutorConfig
	factory         common.ExecutorFactory
}

func NewExecutorFactory(
	executorConfig *config.ExecutorConfig,
	containerConfig *config.ContainerExecutorConfig,
	imageSecretResolver container.ImageSecretResolver,
	logger *zap.SugaredLogger) (*ExecutorFactoryImpl, error) {
	instance := &ExecutorFactoryImpl{executorConfig: executorConfig, containerConfig: containerConfig}
	if executorConfig.Type == config.ExecutorConfigTypeValueContainer {
		factory, err := container.NewContainerExecutorFactory(containerConfig, imageSecretResolver, logger)
		if err != nil {
			return nil, err
		}
		instance.factory = factory
	} else {
		return nil, errors.New("unsupported executor type " + executorConfig.Type)
	}
	return instance, nil
}

func (f *ExecutorFactoryImpl) GetExecutor(runId uuid.UUID, opts *common.ExecutorOpts) (common.Executor, error) {
	return f.factory.GetExecutor(runId, opts)
}
