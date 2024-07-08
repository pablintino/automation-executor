package container

import (
	"context"
	"github.com/pablintino/automation-executor/internal/models"
	"io"

	"github.com/pablintino/automation-executor/internal/utils"
)

const (
	containerStateExited     = "exited"
	containerStateStopped    = "stopped"
	containerStateCreated    = "created"
	containerStateConfigured = "configured"
	containerStateRunning    = "running"

	ErrContainerRunAborted = utils.ConstError("container run aborted")
)

type ContainerRunOpts struct {
	Labels        map[string]string
	Image         string
	Command       []string
	Volumes       map[string]string
	Mounts        map[string]string
	Environ       map[string]string
	PreserveStdin bool
}

type containerImp struct {
	runtime   ContainerRuntime
	runOpts   ContainerRunOpts
	runtimeId string
}

type ContainerStreams struct {
	Output io.Writer
	Error  io.Writer
	Input  io.Reader
}

type ContainerState struct {
	Status   string `json:"Status"`
	Running  bool   `json:"Running"`
	Paused   bool   `json:"Paused"`
	ExitCode int32  `json:"ExitCode"`
}

type Container interface {
	GetRunOpts() ContainerRunOpts
	Id() string
	StartAttach(ctx context.Context, streams *ContainerStreams) error
	Attach(ctx context.Context, streams *ContainerStreams) error
}

type ImageSecretResolver interface {
	GetSecretByRegistry(registry string) (*models.RegistrySecretModel, error)
}

type ContainerRuntime interface {
	CreateVolume(name string, labels map[string]string) (string, error)
	DeleteVolume(name string, force bool) error
	Build(name string, containerFile string) error
	ExistsImage(name string) (bool, error)
	PullImage(name string) error
	ExistsContainer(name string) (bool, error)
	GetVolumesByLabels(labels map[string]string) ([]string, error)
	GetContainersByLabels(labels map[string]string) ([]Container, error)
	CreateContainer(name string, runOpts *ContainerRunOpts) (Container, error)
	DestroyContainer(name string) error
	StartAttach(ctx context.Context, id string, streams *ContainerStreams) error
	Attach(ctx context.Context, id string, streams *ContainerStreams) error
	GetState(id string) (*ContainerState, error)
}

func (c *containerImp) StartAttach(ctx context.Context, streams *ContainerStreams) error {
	return c.runtime.StartAttach(ctx, c.runtimeId, streams)
}

func (c *containerImp) Attach(ctx context.Context, streams *ContainerStreams) error {
	return c.runtime.Attach(ctx, c.runtimeId, streams)
}

func (c *containerImp) GetRunOpts() ContainerRunOpts {
	return c.runOpts
}

func (c *containerImp) Id() string {
	return c.runtimeId
}
