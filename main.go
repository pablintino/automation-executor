package main

import (
	"bufio"
	"context"
	"fmt"
	"github.com/google/uuid"
	containers2 "github.com/pablintino/automation-executor/internal/containers"
	"github.com/pablintino/automation-executor/internal/shells"
	"os"
	"time"

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

func main2() {
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
	logging.Initialize(true)
	config, err := config.Configure()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	database, err := db.NewSQLDatabase(&config.DatabaseConfig)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	podmanIface, err := containers2.NewPodmanContainerInterface(&config.PodmanConfig, database.Containers(), shells.NewBashShellEnvironmentBuilder(&config.ShellConfig))
	storageManager, err := storage.NewStorageManager(&config.StorageConfig)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	envUuid, err := uuid.NewUUID()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	ws, err := storageManager.CreateWorkspace(envUuid)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	chann := make(chan interface{})
	out, err := podmanIface.RunContainer(ws, &containers2.ContainerRunOpts{
		Image: "docker.io/library/ubuntu:latest",
		OnReady: func(container containers2.Container) {
			chann <- true
		},
	})
	fmt.Println(out)
	fmt.Println(err)
	select {
	case msg := <-chann:
		fmt.Println(msg)
	}
	time.Sleep(time.Duration(60) * time.Second)
}

func main55() {
	logging.Initialize(true)
	config, err := config.Configure()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	container, err := storage.NewContainer(&config.StorageConfig)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
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
