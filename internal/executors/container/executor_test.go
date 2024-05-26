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

type basicTestRunData struct {
	runSupport           bool
	runPayload           bool
	payloadImage         string
	supportImage         string
	executor             *ContainerExecutorImpl
	workspaceVolume      string
	supportContainerData map[string]interface{}
	payloadContainerData map[string]interface{}
	runId                uuid.UUID
	imageResolver        *stubImageResolver
	executorOpts         *common.ExecutorOpts
	wg                   sync.WaitGroup
}

func TestContainerCreateBasic(t *testing.T) {
	logging.Initialize(true)
	defer logging.Release()
	t.Cleanup(func() {
		cleanUpAllPodmanTestResources()
	})
	tests := []struct {
		name         string
		runSupport   bool
		runPayload   bool
		payloadImage string
		supportImage string
	}{
		{
			runPayload:   true,
			name:         "container-create-basic",
			payloadImage: "docker.io/library/debian:latest",
		},
		{
			runSupport:   true,
			name:         "container-create-support-basic",
			supportImage: "docker.io/library/debian:latest",
		},
		{
			runSupport: true,
			name:       "container-create-support-internal-image-basic",
		},
		{
			runSupport:   true,
			runPayload:   true,
			name:         "container-create-both-basic",
			supportImage: "docker.io/library/debian:latest",
			payloadImage: "docker.io/library/debian:latest",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := &config.ContainerExecutorConfig{}
			runtime, err := newPodmanRuntime(config)
			require.NoError(t, err)
			resolver := &stubImageResolver{image: "docker.io/library/alpine:latest"}
			runId := uuid.New()
			opts := &common.ExecutorOpts{WorkspaceDirectory: "/tmp/workspace"}
			executor, err := NewContainerExecutor(config, runtime, resolver, runId, opts, logging.Logger)

			assert.NoError(t, err)
			assert.False(t, executor.Recovered())
			if err != nil {
				// Do not continue the test, is not worthy
				return
			}

			testData := &basicTestRunData{
				supportImage:  test.supportImage,
				payloadImage:  test.payloadImage,
				executor:      executor,
				runId:         runId,
				imageResolver: resolver,
				executorOpts:  opts,
			}
			defer func() {
				err := executor.Destroy()
				assert.NoError(t, err)
				if testData.workspaceVolume != "" {
					volumeExists, err := checkResourceExists("volume", testData.workspaceVolume)
					assert.NoError(t, err)
					assert.False(t, volumeExists)
				}

				if test.runPayload {
					assert.NotNil(t, testData.payloadContainerData)
					if testData.payloadContainerData != nil {
						containerExists, err := checkResourceExists("container", testData.payloadContainerData["Id"].(string))
						assert.NoError(t, err)
						assert.False(t, containerExists)
					}
				}
				if test.runSupport {
					assert.NotNil(t, testData.supportContainerData)
					if testData.supportContainerData != nil {
						containerExists, err := checkResourceExists("container", testData.supportContainerData["Id"].(string))
						assert.NoError(t, err)
						assert.False(t, containerExists)
					}
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
				testData.workspaceVolume = workspaceVolumes[0]
			}
			if err != nil || testData.workspaceVolume == "" {
				// Do not continue the test, is not worthy
				return
			}
			script := "echo 'running test'; echo 'running stderr test' >&2 ;sleep 3"
			if test.runSupport {
				execCommand := &common.ExecutorCommand{
					ImageName: test.supportImage,
					IsSupport: true,
					Script:    script,
					Command:   []string{"/bin/sh"},
				}
				testData.wg.Add(1)
				go funcName(t, executor, execCommand, testData)
			}
			if test.runPayload {
				execCommand := &common.ExecutorCommand{
					ImageName: test.payloadImage,
					IsSupport: false,
					Script:    script,
					Command:   []string{"/bin/sh"},
				}
				testData.wg.Add(1)
				go funcName(t, executor, execCommand, testData)
			}

			if test.runPayload && test.runSupport {
				// Watch that pointers are properly handled
				go func() {
					for range 10 {
						runningSupport := executor.GetRunningCommand(true)
						runningPayload := executor.GetRunningCommand(false)
						if runningPayload != nil && runningSupport != nil && runningSupport != runningPayload {
							// Done, both coexisted and point to different instances
							return
						}
						time.Sleep(200 * time.Microsecond)
					}
					assert.Fail(t, "support and payload container didn't coexist")
				}()
			}
			testData.wg.Wait()

		})
	}
}

func funcName(t *testing.T, executor *ContainerExecutorImpl, execCommand *common.ExecutorCommand, testData *basicTestRunData) {
	defer testData.wg.Done()
	stdOutWriter := bytes.NewBufferString("")
	stdErrWriter := bytes.NewBufferString("")
	streams := &common.ExecutorStreams{
		OutputStream: stdOutWriter,
		ErrorStream:  stdErrWriter,
	}
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	runningCmd, err := executor.Execute(ctx, execCommand, streams)
	assert.Equal(t, runningCmd, executor.GetRunningCommand(execCommand.IsSupport))
	assert.Nil(t, executor.GetPreviousRunningCommand(execCommand.IsSupport))
	assert.NoError(t, err)
	if err != nil {
		// Do not continue the test, is not worthy, it already failed
		// so no need to wait for the execution that won't happen
		return
	}

	var waitGroup sync.WaitGroup
	var doneFlag atomic.Bool
	waitGroup.Add(1)
	var containerData map[string]interface{}
	go func() {
		for {
			if doneFlag.Load() {
				break
			}
			typeValue := containerResourcesContainerTypeValuePayload
			if execCommand.IsSupport {
				typeValue = containerResourcesContainerTypeValueSupport
			}
			containerData, err = getSinglePodmanContainerByLabels(map[string]string{
				containerResourcesLabel:              containerResourcesLabelValue,
				containerResourcesLabelRunId:         testData.runId.String(),
				containerResourcesLabelCmdId:         runningCmd.Id().String(),
				containerResourcesLabelContainerType: typeValue,
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
	assert.Equal(t, "running test\n", stdOutWriter.String())
	assert.Equal(t, "running stderr test\n", stdErrWriter.String())
	assert.Equal(t, 0, runningCmd.StatusCode())
	assert.True(t, runningCmd.Finished())
	assert.False(t, runningCmd.Killed())
	assert.Nil(t, runningCmd.Error())
	assert.NotNil(t, containerData)
	if containerData == nil {
		// If no container could be fetched no way to assert against that data
		return
	}

	assertVolumeIsMounted(t, containerData, testData.workspaceVolume, testData.executorOpts.WorkspaceDirectory)
	ranImage := containerData["ImageName"].(string)
	if execCommand.IsSupport {
		testData.supportContainerData = containerData
		if testData.supportImage == "" {
			assert.Equal(t, testData.imageResolver.image, ranImage)
		} else {
			assert.Equal(t, testData.supportImage, ranImage)
		}
	} else {
		testData.payloadContainerData = containerData
		assert.Equal(t, testData.payloadImage, ranImage)
	}
}
