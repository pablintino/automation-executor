package containers

import (
	"context"
	"fmt"
	"github.com/containers/podman/v3/libpod/define"
	"github.com/containers/podman/v3/pkg/api/handlers"
	"github.com/containers/podman/v3/pkg/bindings"
	"github.com/containers/podman/v3/pkg/bindings/containers"
	"github.com/containers/podman/v3/pkg/specgen"
	docker "github.com/docker/docker/api/types"
	"github.com/pablintino/automation-executor/internal/config"
	"os"
)

type ContainerRunOpts struct {
	Id      string
	Name    string
	Labels  map[string]string
	Image   string
	Volumes map[string]string
}

type ContainerImpl struct {
}

type Container interface {
	RunCommand(command ...string) (int, error)
}

type ContainerRuntimeInterface interface {
	RunContainer(runOpts *ContainerRunOpts) (Container, error)
	DestroyContainer(id string) error
}

type PodmanContainerInterface struct {
	ctx context.Context
}

func NewPodmanContainerInterface(config *config.PodmanConfig) (*PodmanContainerInterface, error) {
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
	return &PodmanContainerInterface{ctx: ctx}, nil
}

func (p *PodmanContainerInterface) RunContainer(runOpts *ContainerRunOpts) (Container, error) {
	s := specgen.NewSpecGenerator(runOpts.Image, false)
	s.Terminal = true
	s.Labels = map[string]string{"test-label": "label-value"}
	bashCmd := fmt.Sprintf("trap \"exit\" TERM; for try in {1..%d}; do sleep 1; done", 3600)
	s.Command = []string{"/bin/bash", "-c", bashCmd}

	r, err := containers.CreateWithSpec(conn, s, nil)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Container start
	err = containers.Start(conn, r.ID, nil)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	condition := containers.WaitOptions{Condition: []define.ContainerStatus{define.ContainerStateRunning}}
	_, err = containers.Wait(conn, r.ID, &condition)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	execCreateConfig := handlers.ExecCreateConfig{
		ExecConfig: docker.ExecConfig{
			Tty:          false,
			Cmd:          []string{"sleep", "3500"},
			AttachStdout: true,
			AttachStderr: true,
		},
	}
	execSessionId, err := containers.ExecCreate(conn, r.ID, &execCreateConfig)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
