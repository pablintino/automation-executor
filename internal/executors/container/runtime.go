package container

import (
	"context"
	"io"
)

const (
	containerStateExited  = "exited"
	containerStateStopped = "stopped"
)

type ContainerRunOpts struct {
	Labels      map[string]string
	Image       string
	Command     []string
	Volumes     map[string]string
	Mounts      map[string]string
	AttachStdin bool
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
}

type ContainerRuntime interface {
	CreateVolume(name string, labels map[string]string) (string, error)
	DeleteVolume(name string) error
	Build(name string, containerFile string) error
	ExistsImage(name string) (bool, error)
	ExistsContainer(name string) (bool, error)
	CreateContainer(name string, runOpts *ContainerRunOpts) (Container, error)
	DestroyContainer(name string) error
	StartAttach(ctx context.Context, id string, streams *ContainerStreams) error
	GetState(id string) (*ContainerState, error)
}

func (c *containerImp) StartAttach(ctx context.Context, streams *ContainerStreams) error {
	return c.runtime.StartAttach(ctx, c.runtimeId, streams)
}

func (c *containerImp) GetRunOpts() ContainerRunOpts {
	return c.runOpts
}

func (c *containerImp) Id() string {
	return c.runtimeId
}
