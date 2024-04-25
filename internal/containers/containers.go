package containers

import "github.com/pablintino/automation-executor/internal/storage"

type OnReadyFn func(container Container)
type OnBootFailureFn func(container Container, err error)

type ContainerRunOpts struct {
	Labels        map[string]string
	Image         string
	Mounts        map[string]string
	OnReady       OnReadyFn
	OnBootFailure OnBootFailureFn
}

type ContainerRuntimeInterface interface {
	RunContainer(workspace storage.Workspace, runOpts *ContainerRunOpts)
	DestroyContainer(id string) error
}

type Container interface {
	RuntimeId() string
	RunCommand(env map[string]string, command ...string) (int, error)
}
