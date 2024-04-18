package main

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"github.com/containers/podman/v3/libpod/define"
	"github.com/containers/podman/v3/pkg/api/handlers"
	"github.com/containers/podman/v3/pkg/bindings"
	"github.com/containers/podman/v3/pkg/bindings/containers"
	"github.com/containers/podman/v3/pkg/specgen"
	docker "github.com/docker/docker/api/types"
	"github.com/pablintino/automation-executor/internal/config"
	"github.com/pablintino/automation-executor/internal/db"
	"github.com/pablintino/automation-executor/internal/storage"
	"github.com/pablintino/automation-executor/logging"
	"gocloud.dev/blob/fileblob"
)

type MyWriteCloser struct {
	*bufio.Writer
}

func (mwc *MyWriteCloser) Close() error {
	mwc.Flush()
	return nil
}

func main123() {
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

	ssiD := "6a843e76877fefb71e4dc4c0b2ab5addbb7a8696eafaa7364e503d64a5419ed5"
	sessionInspect, err := containers.ExecInspect(conn, ssiD, nil)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println(sessionInspect)
	mwc := &MyWriteCloser{bufio.NewWriter(os.Stdout)}
	defer mwc.Close()
	startAndAttachOptions := new(containers.ExecStartAndAttachOptions)
	startAndAttachOptions.WithOutputStream(mwc).WithAttachOutput(true)
	startAndAttachOptions.WithErrorStream(mwc).WithAttachError(true)
	err = containers.ExecStartAndAttach(conn, ssiD, startAndAttachOptions)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	sessionInspect, err = containers.ExecInspect(conn, ssiD, nil)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if sessionInspect.ExitCode != 0 {
		fmt.Println("Program execution failed")
	}
}

func main33() {
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
	s := specgen.NewSpecGenerator("debian:bookworm-slim", false)
	s.Terminal = true
	s.Labels = map[string]string{"test-label": "label-value"}
	bashCmd := fmt.Sprintf("trap \"exit\" TERM; for try in {1..%d}; do sleep 1; done", 3600)
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
	fmt.Println(execSessionId)
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

func main3() {
	options := &fileblob.Options{NoTempDir: true}
	bucket, err := fileblob.OpenBucket("/home/pablintino/Sources/automation-executor/test", options)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer bucket.Close()

	// We now have a *blob.Bucket! We can write our application using the
	// *blob.Bucket type, and have the freedom to change the initialization code
	// above to choose a different service-specific driver later.

	// In this example, we'll write a blob and then read it.
	ctx := context.Background()
	if err := bucket.WriteAll(ctx, "foo.txt", []byte("Go Cloud Development Kit"), nil); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	b, err := bucket.ReadAll(ctx, "foo.txt")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println(string(b))
}

func main() {
	config, err := config.Configure()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	db, err := db.NewSQLDatabase(&config.DatabaseConfig)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	testValues, err := db.Tasks().GetAllAnsible()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println(testValues)
}

func main354() {
	logging.Initialize(true)
	config, err := config.Configure()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	container := storage.NewContainer(&config.ArtifactsConfig)
	file, err := os.Open("/home/pablintino/Desktop/test.tar.gz")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer func() {
		file.Close()
	}()
	scanConfig, err := storage.NewScanConfig([]string{"*.json"})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	result, err := container.ArtifactsScanner.ScanArchive(file, scanConfig)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println(result)
}
