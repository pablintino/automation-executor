package main

import (
	"bufio"
	"context"
	"fmt"
	"github.com/containers/podman/v3/libpod/define"
	"github.com/containers/podman/v3/pkg/api/handlers"
	"github.com/containers/podman/v3/pkg/bindings"
	"github.com/containers/podman/v3/pkg/bindings/containers"
	"github.com/containers/podman/v3/pkg/specgen"
	docker "github.com/docker/docker/api/types"
	"os"
)

type MyWriteCloser struct {
	*bufio.Writer
}

func (mwc *MyWriteCloser) Close() error {
	mwc.Flush()
	return nil
}

func main() {
	// Get Podman socket location
	sock_dir := os.Getenv("XDG_RUNTIME_DIR")
	if sock_dir == "" {
		sock_dir = "/var/run"
	}
	socket := "unix:" + sock_dir + "/podman/podman.sock"

	conn, err := bindings.NewConnection(context.Background(), socket)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	s := specgen.NewSpecGenerator("localhost/tmp-ansible-container", false)
	s.Terminal = true
	s.Labels = map[string]string{"test-label": "label-value"}
	bashCmd := fmt.Sprintf("trap \"exit\" TERM; for try in {1..%d}; do sleep 1; done", 20)
	s.Command = []string{"/bin/bash", "-c", bashCmd}

	r, err := containers.CreateWithSpec(conn, s, nil)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer func() {
		removeOpts := &containers.RemoveOptions{}
		removeOpts.WithForce(true)
		removeOpts.WithVolumes(true)
		err := containers.Remove(conn, r.ID, removeOpts)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}()

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
			Cmd:          []string{"ls", "-ltr", "/etc"},
			AttachStdout: true,
			AttachStderr: true,
		},
	}
	execSessionId, err := containers.ExecCreate(conn, r.ID, &execCreateConfig)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	mwc := &MyWriteCloser{bufio.NewWriter(os.Stdout)}
	defer mwc.Close()
	startAndAttachOptions := new(containers.ExecStartAndAttachOptions)
	startAndAttachOptions.WithOutputStream(mwc).WithAttachOutput(true)
	startAndAttachOptions.WithErrorStream(mwc).WithAttachError(true)
	err = containers.ExecStartAndAttach(conn, execSessionId, startAndAttachOptions)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	sessionInspect, err := containers.ExecInspect(conn, execSessionId, nil)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if sessionInspect.ExitCode != 0 {
		fmt.Println("Program execution failed")
	}
}
