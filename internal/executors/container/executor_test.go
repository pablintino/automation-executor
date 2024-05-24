package container

import (
	"bytes"
	"context"
	"github.com/google/uuid"
	"github.com/pablintino/automation-executor/internal/config"
	"github.com/pablintino/automation-executor/internal/executors/common"
	"github.com/pablintino/automation-executor/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type stubImageResolver struct {
	image string
}

func (s *stubImageResolver) GetSupportImage() (string, error) {
	return s.image, nil
}

func TestContainerCreateBasic(t *testing.T) {
	logging.Initialize(true)
	defer logging.Release()
	t.Cleanup(func() {
		cleanUpAllPodmanTestResources()
	})

	t.Run("container-create-basic", func(t *testing.T) {
		config := &config.ContainerExecutorConfig{}
		runtime, err := newPodmanRuntime(config)
		require.NoError(t, err)
		resolver := &stubImageResolver{image: "docker.io/library/alpine"}
		runId := uuid.New()
		opts := &common.ExecutorOpts{WorkspaceDirectory: "/tmp/workspace"}
		executor, err := NewContainerExecutor(config, runtime, resolver, runId, opts, logging.Logger)
		assert.NoError(t, err)
		assert.False(t, executor.Recovered())
		if err != nil {
			// Do not continue the test, is not worthy
			return
		}

		var containerData map[string]interface{}
		var workspaceVolume string
		defer func() {
			err := executor.Destroy()
			assert.NoError(t, err)
			if workspaceVolume != "" {
				volumeExists, err := checkResourceExists("volume", workspaceVolume)
				assert.NoError(t, err)
				assert.False(t, volumeExists)
			}
			assert.NotNil(t, containerData)
			if containerData != nil {
				containerExists, err := checkResourceExists("container", containerData["Id"].(string))
				assert.NoError(t, err)
				assert.False(t, containerExists)
			}
		}()
		err = executor.Prepare()
		assert.NoError(t, err)
		if err != nil {
			// If prepare already failed there is no point in continuing
			return
		}

		workspaceVolumes, err := getPodmanVolumesByLabels(map[string]string{
			containerResourcesLabel:      containerResourcesLabelValue,
			containerResourcesLabelRunId: runId.String(),
		})
		assert.NoError(t, err)
		assert.NotNil(t, workspaceVolumes)
		assert.Len(t, workspaceVolumes, 1)
		if workspaceVolumes != nil && len(workspaceVolumes) == 1 {
			workspaceVolume = workspaceVolumes[0]
		}
		if err != nil || workspaceVolume == "" {
			// Do not continue the test, is not worthy
			return
		}

		script := "echo 'running test'; echo 'running stderr test' >&2 ;sleep 3"
		execCommand := &common.ExecutorCommand{
			ImageName: "docker.io/library/debian:latest",
			IsSupport: false,
			Script:    script,
			Command:   []string{"/bin/bash"},
		}

		stdOutWriter := bytes.NewBufferString("")
		stdErrWriter := bytes.NewBufferString("")
		streams := &common.ExecutorStreams{
			OutputStream: stdOutWriter,
			ErrorStream:  stdErrWriter,
		}
		ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
		runningCmd, err := executor.Execute(ctx, execCommand, streams)
		assert.Equal(t, runningCmd, executor.GetRunningCommand(false))
		assert.Nil(t, executor.GetPreviousRunningCommand(false))
		assert.NoError(t, err)
		if err != nil {
			// Do not continue the test, is not worthy, it already failed
			// so no need to wait for the execution that won't happen
			return
		}

		var waitGroup sync.WaitGroup
		var doneFlag atomic.Bool
		waitGroup.Add(1)
		go func() {
			for {
				if doneFlag.Load() {
					break
				}
				containerData, err = getSinglePodmanContainerByLabels(map[string]string{
					containerResourcesLabel:              containerResourcesLabelValue,
					containerResourcesLabelRunId:         runId.String(),
					containerResourcesLabelCmdId:         runningCmd.Id().String(),
					containerResourcesLabelContainerType: containerResourcesLabelContainerTypeValuePayload,
				})
				assert.NoError(t, err)
				if containerData != nil {
					break
				}
				time.Sleep(200 * time.Microsecond)
			}
			waitGroup.Done()
		}()
		err = runningCmd.Wait()
		doneFlag.Store(true)
		waitGroup.Wait()
		assert.NoError(t, err)
		require.NotNil(t, containerData)
		assertVolumeIsMounted(t, containerData, workspaceVolume, opts.WorkspaceDirectory)

		assert.Equal(t, "running test\n", stdOutWriter.String())
		assert.Equal(t, "running stderr test\n", stdErrWriter.String())
		assert.Equal(t, 0, runningCmd.StatusCode())
		assert.True(t, runningCmd.Finished())
		assert.False(t, runningCmd.Killed())
		assert.Nil(t, runningCmd.Error())
	})
}
