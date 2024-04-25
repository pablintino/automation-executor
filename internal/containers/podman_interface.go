package containers

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path"

	"github.com/pablintino/automation-executor/internal/shells"

	"os"
	"sync/atomic"

	"github.com/containers/podman/v3/pkg/api/handlers"
	"github.com/containers/podman/v3/pkg/bindings"
	"github.com/containers/podman/v3/pkg/bindings/containers"
	"github.com/containers/podman/v3/pkg/specgen"
	docker "github.com/docker/docker/api/types"
	spec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pablintino/automation-executor/internal/config"
	"github.com/pablintino/automation-executor/internal/db"
	"github.com/pablintino/automation-executor/internal/models"
	"github.com/pablintino/automation-executor/internal/storage"
	"github.com/pablintino/automation-executor/internal/utils"
)

type PodmanContainerInterface struct {
	ctx          context.Context
	podmanConfig *config.PodmanConfig
	shellBuilder shells.ShellEnvironmentBuilder
	containerDb  db.ContainerDb
}

type ContainerImpl struct {
	runtimeId       string
	fnOnReady       func(container Container)
	fnOnBootFailure func(container Container, err error)
	startRoutine    *utils.RetryRoutine
	iface           *PodmanContainerInterface
	isReady         atomic.Bool
	dbModel         *models.ContainerModel
	containerWs     string
}

func NewPodmanContainerInterface(config *config.PodmanConfig, containerDb db.ContainerDb, shellBuilder shells.ShellEnvironmentBuilder) (*PodmanContainerInterface, error) {
	var socket string
	if config.Socket != "" {
		socket = config.Socket
	} else {
		sockDir := os.Getenv("XDG_RUNTIME_DIR")
		if sockDir == "" {
			sockDir = "/var/run"
		}
		socket = "unix:" + sockDir + "/podman/podman.sock"
	}
	if socket == "" {
		return nil, fmt.Errorf("could not find podman socket")
	}

	ctx, err := bindings.NewConnection(context.Background(), socket)
	if err != nil {
		return nil, err
	}
	return &PodmanContainerInterface{ctx: ctx, podmanConfig: config, containerDb: containerDb, shellBuilder: shellBuilder}, nil
}

func (p *PodmanContainerInterface) RunContainer(workspace storage.Workspace, runOpts *ContainerRunOpts) (Container, error) {
	s := specgen.NewSpecGenerator(runOpts.Image, false)
	s.Terminal = true
	s.Labels = runOpts.Labels
	s.Mounts = p.buildMounts(runOpts.Mounts)
	bashCmd := fmt.Sprintf("trap \"exit\" TERM; for try in {1..%d}; do sleep 1; done", 3600)
	s.Command = []string{"/bin/bash", "-c", bashCmd}

	deferred, err := p.containerDb.CreateContainerTxDeferred(models.ContainerTypePodman)
	if err != nil {
		return nil, err
	}
	defer deferred.Cancel()

	containerWs, err := workspace.CreateContainerWorkspace(deferred.ContainerId)
	if err != nil {
		return nil, err
	}
	createResponse, err := containers.CreateWithSpec(p.ctx, s, nil)
	if err != nil {
		return nil, err
	}
	containerModel, err := deferred.Save(createResponse.ID)
	if err != nil {
		return nil, err
	}

	err = containers.Start(p.ctx, createResponse.ID, nil)
	if err != nil {
		return nil, err
	}

	container := &ContainerImpl{
		runtimeId: containerModel.RuntimeId, iface: p, fnOnReady: runOpts.OnReady,
		fnOnBootFailure: runOpts.OnBootFailure, dbModel: containerModel, containerWs: containerWs,
	}
	container.startRoutine = utils.NewRetryRoutine(1000, 180, func() (bool, error) {
		containerData, err := containers.Inspect(container.iface.ctx, container.runtimeId, nil)
		if err != nil {
			return false, err
		}
		return containerData.State.Running && !containerData.State.Paused, nil
	}, container.bootCallback, nil)
	container.startRoutine.Start()
	return container, nil
}

func (p *PodmanContainerInterface) buildMounts(requestedMounts map[string]string) []spec.Mount {
	mounts := make([]spec.Mount, 0)
	for source, target := range requestedMounts {
		mounts = append(mounts, spec.Mount{Destination: target, Source: source, Type: "linux"})
	}
	return mounts
}

func (c *ContainerImpl) bootCallback(success bool, err error) {
	if !success {
		if c.fnOnBootFailure != nil {
			c.fnOnBootFailure(c, err)
		}
		return
	}
	c.isReady.Store(true)
	if c.fnOnReady != nil {
		c.fnOnReady(c)
	}
}

func (c *ContainerImpl) RunCommand(env map[string]string, command ...string) (int, error) {
	if !c.isReady.Load() {
		return 1, errors.New("container not ready")
	}
	deferrable, err := c.iface.containerDb.CreateContainerExecTxDeferred(c.dbModel.Id)
	if err != nil {
		return 1, err
	}
	defer deferrable.Cancel()

	logPath := path.Join("/var/log/execs", deferrable.ContainerExecId.String())
	cmdOut, envOut := c.prepareCommandEnvironment(logPath, env, command...)
	execCreateConfig := handlers.ExecCreateConfig{
		ExecConfig: docker.ExecConfig{
			Env:          envOut,
			Tty:          false,
			Cmd:          cmdOut,
			AttachStdout: false,
			AttachStderr: false,
		},
	}

	execSessionId, err := containers.ExecCreate(c.iface.ctx, c.runtimeId, &execCreateConfig)
	if err != nil {
		return 1, err
	}

	if _, err = deferrable.Save(execSessionId); err != nil {
		return 1, err
	}
	return 0, nil
}

func (c *ContainerImpl) prepareCommandEnvironment(logPath string, env map[string]string, command ...string) ([]string, []string) {
	cmdOut, shellEnvs := c.iface.shellBuilder.BuildShellRun(logPath, command...)

	envVarsDict := make(map[string]string)
	if env != nil {
		maps.Copy(envVarsDict, env)
	}
	maps.Copy(envVarsDict, shellEnvs)
	envOut := make([]string, len(envVarsDict))
	for name, value := range envVarsDict {
		envOut = append(envOut, fmt.Sprintf("%s=%s", name, value))
	}
	return cmdOut, envOut
}

func (c *ContainerImpl) RuntimeId() string {
	return c.runtimeId
}
