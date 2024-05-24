package container

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/containers/buildah/define"
	"github.com/containers/podman/v3/pkg/bindings"
	"github.com/containers/podman/v3/pkg/bindings/containers"
	"github.com/containers/podman/v3/pkg/bindings/images"
	"github.com/containers/podman/v3/pkg/bindings/volumes"
	"github.com/containers/podman/v3/pkg/domain/entities"
	"github.com/containers/podman/v3/pkg/specgen"
	"github.com/pablintino/automation-executor/internal/config"
)

type podmanRuntime struct {
	clientCtx context.Context
	config    *config.ContainerExecutorConfig
}

func newPodmanRuntime(executorConfig *config.ContainerExecutorConfig) (*podmanRuntime, error) {
	var socket string
	if executorConfig.Socket != "" {
		socket = executorConfig.Socket
	} else {
		sockDir := os.Getenv("XDG_RUNTIME_DIR")
		if sockDir == "" {
			sockDir = "/var/run"
		}
		socket = "unix:" + sockDir + "/podman/podman.sock"
	}
	if socket == "" {
		return nil, fmt.Errorf("could not find container socket")
	}

	clientCtx, err := bindings.NewConnection(context.Background(), socket)
	if err != nil {
		return nil, err
	}
	return &podmanRuntime{clientCtx: clientCtx, config: executorConfig}, nil
}

func (r *podmanRuntime) CreateVolume(name string, labels map[string]string) (string, error) {
	result, err := volumes.Create(r.clientCtx, entities.VolumeCreateOptions{
		Name:   name,
		Driver: "local",
		Label:  labels,
	}, nil)
	if err != nil {
		return "", err
	}
	return result.Name, err
}

func (r *podmanRuntime) DeleteVolume(name string, force bool) error {
	exists, err := volumes.Exists(r.clientCtx, name, nil)
	if err != nil {

	}
	if exists {
		opts := &volumes.RemoveOptions{Force: &force}
		return volumes.Remove(r.clientCtx, name, opts)
	}
	return nil
}

func (r *podmanRuntime) Build(name string, containerFile string) error {
	buildOpts := entities.BuildOptions{BuildOptions: define.BuildOptions{Output: name}}
	result, err := images.Build(r.clientCtx, []string{containerFile}, buildOpts)
	if err != nil {
		return err
	}
	// TODO
	fmt.Println(result)
	return nil
}

func (r *podmanRuntime) CreateContainer(name string, runOpts *ContainerRunOpts) (Container, error) {
	s := specgen.NewSpecGenerator(runOpts.Image, false)
	// We do not need a pseudo tty, keep it to false
	s.Terminal = false
	s.Labels = runOpts.Labels
	s.Volumes = buildPodmanVolumes(runOpts.Volumes)
	s.Env = runOpts.Environ
	s.Command = runOpts.Command
	s.Name = name
	s.Stdin = runOpts.PreserveStdin

	createResponse, err := containers.CreateWithSpec(r.clientCtx, s, nil)
	if err != nil {
		return nil, err
	}

	return &containerImp{runtime: r, runOpts: *runOpts, runtimeId: createResponse.ID}, nil
}

func (r *podmanRuntime) DestroyContainer(name string) error {
	force := true
	opts := &containers.RemoveOptions{Force: &force}
	return containers.Remove(r.clientCtx, name, opts)
}

func (r *podmanRuntime) StartAttach(ctx context.Context, id string, streams *ContainerStreams) error {
	attachErr := make(chan error)
	attachReady := make(chan bool)
	go func() {
		stream := true
		opts := &containers.AttachOptions{Stream: &stream}
		err := containers.Attach(r.clientCtx, id, streams.Input, streams.Output, streams.Error, attachReady, opts)
		attachErr <- err
	}()
	// Wait till the attachment is done
	select {
	case <-attachReady:
		if err := containers.Start(r.clientCtx, id, nil); err != nil {
			return err
		}
	case err := <-attachErr:
		return err
	}

	select {
	case err := <-attachErr:
		return err
	case <-ctx.Done():
		return fmt.Errorf("container start and attach aborted %s", id)
	}
}

func (r *podmanRuntime) Attach(ctx context.Context, id string, streams *ContainerStreams) error {
	attachErr := make(chan error)
	go func() {
		stream := true
		opts := &containers.AttachOptions{Stream: &stream}
		err := containers.Attach(r.clientCtx, id, streams.Input, streams.Output, streams.Error, nil, opts)
		attachErr <- err
	}()
	select {
	case err := <-attachErr:
		return err
	case <-ctx.Done():
		return fmt.Errorf("container attach aborted %s", id)
	}
}

func (r *podmanRuntime) ExistsContainer(name string) (bool, error) {
	exists, err := containers.Exists(r.clientCtx, name, nil)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (r *podmanRuntime) GetState(id string) (*ContainerState, error) {
	inspectResult, err := containers.Inspect(r.clientCtx, id, nil)
	if err != nil {
		return nil, err
	}
	return &ContainerState{
		Status:   inspectResult.State.Status,
		Running:  inspectResult.State.Running,
		Paused:   inspectResult.State.Paused,
		ExitCode: inspectResult.State.ExitCode,
	}, nil
}

func (r *podmanRuntime) ExistsImage(name string) (bool, error) {
	exists, err := images.Exists(r.clientCtx, name, nil)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (r *podmanRuntime) PullImage(name string) error {
	_, err := images.Pull(r.clientCtx, name, nil)
	return err
}

func (r *podmanRuntime) GetVolumesByLabels(labels map[string]string) ([]string, error) {
	opts := &volumes.ListOptions{Filters: map[string][]string{"label": buildLabelFilters(labels)}}
	volumes, err := volumes.List(r.clientCtx, opts)
	if err != nil {
		return nil, err
	}
	result := make([]string, len(volumes))
	for idx, item := range volumes {
		result[idx] = item.Name
	}
	return result, err
}

func (r *podmanRuntime) GetContainersByLabels(labels map[string]string) ([]Container, error) {
	finished := true
	opts := &containers.ListOptions{All: &finished, Filters: map[string][]string{"label": buildLabelFilters(labels)}}
	listResult, err := containers.List(r.clientCtx, opts)
	if err != nil {
		return nil, err
	}
	result := make([]Container, len(listResult))
	for idx, item := range listResult {
		instance, err := r.buildContainerFromId(item.ID)
		if err != nil {
			return nil, err
		}
		result[idx] = instance
	}
	return result, err
}

func (r *podmanRuntime) buildContainerFromId(id string) (Container, error) {
	inspect, err := containers.Inspect(r.clientCtx, id, nil)
	if err != nil {
		return nil, err
	}
	instance := &containerImp{runtime: r, runtimeId: id}
	instance.runOpts.Command = inspect.Config.Cmd
	instance.runOpts.Labels = inspect.Config.Labels
	instance.runOpts.Image = inspect.Config.Image
	instance.runOpts.PreserveStdin = inspect.Config.OpenStdin
	instance.runOpts.Volumes = make(map[string]string)
	instance.runOpts.Mounts = make(map[string]string)
	instance.runOpts.Environ = make(map[string]string)
	for _, env := range inspect.Config.Env {
		envSplit := strings.Split(env, "=")
		value := ""
		if len(envSplit) == 2 {
			value = envSplit[1]
		}
		instance.runOpts.Environ[envSplit[0]] = value
	}
	for _, mountData := range inspect.Mounts {
		if mountData.Type == "volume" {
			instance.runOpts.Volumes[mountData.Name] = mountData.Destination
		} else if mountData.Type == "bind" {
			instance.runOpts.Mounts[mountData.Source] = mountData.Destination
		}
	}
	return instance, err
}

func buildPodmanVolumes(requestedMounts map[string]string) []*specgen.NamedVolume {
	mounts := make([]*specgen.NamedVolume, 0)
	for source, target := range requestedMounts {
		mounts = append(mounts, &specgen.NamedVolume{Dest: target, Name: source})
	}
	return mounts
}

func buildLabelFilters(labels map[string]string) []string {
	labelFilters := make([]string, 0)
	for label, value := range labels {
		if value == "" {
			labelFilters = append(labelFilters, label)
		} else {
			labelFilters = append(labelFilters, fmt.Sprintf("%s=%s", label, value))
		}
	}
	return labelFilters
}
