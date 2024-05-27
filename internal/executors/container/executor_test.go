package container

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pablintino/automation-executor/internal/config"
	"github.com/pablintino/automation-executor/internal/executors/common"
	"github.com/pablintino/automation-executor/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	executorTestImageBaseAlpine    string = "docker.io/library/alpine:latest"
	executorTestImageBaseNonAlpine string = "docker.io/library/debian:latest"
)

type cleanupRegistry struct {
	containers []string
	volumes    []string
	// Used to assert that resources should be already cleared by the tested code
	resourcesAlreadyDeleted bool
	t                       *testing.T
}

func newCleanupRegistry(t *testing.T, resourcesAlreadyDeleted bool) *cleanupRegistry {
	return &cleanupRegistry{
		t:                       t,
		containers:              make([]string, 0),
		volumes:                 make([]string, 0),
		resourcesAlreadyDeleted: resourcesAlreadyDeleted,
	}
}

func (r *cleanupRegistry) addContainerFromData(data map[string]interface{}) {
	r.containers = append(r.containers, data["Id"].(string))
}
func (r *cleanupRegistry) addVolume(id string) {
	r.volumes = append(r.volumes, id)
}

func (r *cleanupRegistry) cleanup() {
	for _, container := range r.containers {
		deleted, err := deleteSingleContainer(container)
		assert.NoError(r.t, err)
		if r.resourcesAlreadyDeleted {
			assert.False(r.t, deleted)
		}
	}
	for _, volume := range r.volumes {
		deleted, err := deleteSingleVolume(volume)
		assert.NoError(r.t, err)
		if r.resourcesAlreadyDeleted {
			assert.False(r.t, deleted)
		}
	}
}

type stubImageResolver struct {
	image string
}

func (s *stubImageResolver) GetSupportImage() (string, error) {
	return s.image, nil
}

func waitGetContainerData(
	t *testing.T,
	execCommand *common.ExecutorCommand,
	runningCmd common.RunningCommand,
	runId uuid.UUID,
	catchFn func(common.RunningCommand, map[string]interface{})) (map[string]interface{}, error) {
	var doneFlag atomic.Bool
	containerDataChan := make(chan map[string]interface{})
	go func() {
		var err error
		var containerData map[string]interface{}
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
				containerResourcesLabelRunId:         runId.String(),
				containerResourcesLabelCmdId:         runningCmd.Id().String(),
				containerResourcesLabelContainerType: typeValue,
			})
			assert.NoError(t, err)
			if containerData != nil {
				// The container is running, kill it
				if catchFn != nil {
					catchFn(runningCmd, containerData)
				}
				break
			}
			time.Sleep(200 * time.Microsecond)
		}
		containerDataChan <- containerData
	}()
	err := runningCmd.Wait()
	doneFlag.Store(true)
	return <-containerDataChan, err
}

func assertContainerCommandSuccess(brt *baseRunTest, data *commandData) {
	assert.NotNil(brt.t, data.containerInfo)
	assert.NotNil(brt.t, data.runningCmd)
	assert.Equal(brt.t, "running test\n", data.stdoutBuffer.String())
	assert.Equal(brt.t, "running stderr test\n", data.stderrBuffer.String())
	if data.runningCmd != nil {
		assert.Equal(brt.t, 0, data.runningCmd.StatusCode())
		assert.True(brt.t, data.runningCmd.Finished())
		assert.False(brt.t, data.runningCmd.Killed())
		assert.Nil(brt.t, data.runningCmd.Error())
	}
	// Both running pointers should be empty
	assert.Nil(brt.t, brt.executor.GetRunningCommand(data.execCommand.IsSupport))
	assert.Nil(brt.t, brt.executor.GetPreviousRunningCommand(data.execCommand.IsSupport))
	assert.NotNil(brt.t, data.containerInfo)
	if data.containerInfo == nil {
		// If no container could be fetched no way to assert against that data
		return
	}

	assertVolumeIsMounted(brt.t, data.containerInfo, brt.workspaceVolume, brt.testParams.executorOpts.WorkspaceDirectory)
	ranImage := data.containerInfo["ImageName"].(string)
	if data.execCommand.IsSupport && data.testParams.image == "" {
		assert.Equal(brt.t, brt.imageResolver.image, ranImage)
	} else {
		assert.Equal(brt.t, data.testParams.image, ranImage)
	}
}

func assertSuccessOnContainersFinished(brt *baseRunTest) error {
	if brt.testParams.runSupport {
		assert.NotNil(brt.t, brt.supportCommandData)
		if brt.supportCommandData != nil {
			assertContainerCommandSuccess(brt, brt.supportCommandData)
		}
	}
	if brt.testParams.runPayload {
		assert.NotNil(brt.t, brt.payloadCommandData)
		if brt.payloadCommandData != nil {
			assertContainerCommandSuccess(brt, brt.payloadCommandData)
		}
	}
	return nil
}

func TestCommonRunBasedTests(t *testing.T) {
	logging.Initialize(true)
	defer logging.Release()
	t.Cleanup(func() {
		cleanUpAllPodmanTestResources()
	})

	tests := []struct {
		name string
		data *executorTestBaseParams
	}{
		{
			name: "container-create-basic",
			data: &executorTestBaseParams{
				runPayload: true,
				runSupport: false,
				payloadCmdParams: executorCommandParams{
					image: executorTestImageBaseNonAlpine,
				},
				executorOpts: &common.ExecutorOpts{WorkspaceDirectory: "/tmp/workspace"},
				onDestroyStart: func(brt *baseRunTest) error {
					assert.NotNil(brt.t, brt.executor)
					if brt.executor != nil {
						assert.NoError(brt.t, brt.executor.Destroy())
					}
					return nil
				},
				onContainersFinished: assertSuccessOnContainersFinished,
			},
		},
		{
			name: "container-create-support-basic",
			data: &executorTestBaseParams{
				runPayload: false,
				runSupport: true,
				supportCmdParams: executorCommandParams{
					image: executorTestImageBaseNonAlpine,
				},
				executorOpts: &common.ExecutorOpts{WorkspaceDirectory: "/tmp/workspace"},
				onDestroyStart: func(brt *baseRunTest) error {
					assert.NotNil(brt.t, brt.executor)
					if brt.executor != nil {
						assert.NoError(brt.t, brt.executor.Destroy())
					}
					return nil
				},
				onContainersFinished: assertSuccessOnContainersFinished,
			},
		},
		{
			name: "container-create-both-basic",
			data: &executorTestBaseParams{
				runPayload: true,
				runSupport: true,
				payloadCmdParams: executorCommandParams{
					image: executorTestImageBaseNonAlpine,
				},
				supportCmdParams: executorCommandParams{
					image: executorTestImageBaseNonAlpine,
				},
				executorOpts: &common.ExecutorOpts{WorkspaceDirectory: "/tmp/workspace"},
				onDestroyStart: func(brt *baseRunTest) error {
					assert.NotNil(brt.t, brt.executor)
					if brt.executor != nil {
						assert.NoError(brt.t, brt.executor.Destroy())
					}
					return nil
				},
				onContainersFinished: assertSuccessOnContainersFinished,
			},
		},
		{
			name: "container-create-support-internal-image-basic",
			data: &executorTestBaseParams{
				runPayload:       false,
				runSupport:       true,
				supportCmdParams: executorCommandParams{},
				executorOpts:     &common.ExecutorOpts{WorkspaceDirectory: "/tmp/workspace"},
				onDestroyStart: func(brt *baseRunTest) error {
					assert.NotNil(brt.t, brt.executor)
					if brt.executor != nil {
						assert.NoError(brt.t, brt.executor.Destroy())
					}
					return nil
				},
				onContainersFinished: assertSuccessOnContainersFinished,
			},
		},
		{
			name: "container-killing-basic",
			data: &executorTestBaseParams{
				runPayload: true,
				runSupport: false,
				payloadCmdParams: executorCommandParams{
					image: executorTestImageBaseNonAlpine,
				},
				executorOpts: &common.ExecutorOpts{WorkspaceDirectory: "/tmp/workspace"},
				onDestroyStart: func(brt *baseRunTest) error {
					assert.NotNil(brt.t, brt.executor)
					if brt.executor != nil {
						assert.NoError(brt.t, brt.executor.Destroy())
					}
					return nil
				},
				onContainersRunning: func(brt *baseRunTest) error {
					assert.NotNil(brt.t, brt.payloadCommandData)
					if brt.payloadCommandData != nil {
						assert.NoError(brt.t, brt.payloadCommandData.runningCmd.Kill())
					}
					return nil
				},
				onContainersFinished: func(brt *baseRunTest) error {
					assert.NotNil(brt.t, brt.payloadCommandData.stdoutBuffer)
					assert.NotNil(brt.t, brt.payloadCommandData.stderrBuffer)
					assert.Equal(t, "running test\n", brt.payloadCommandData.stdoutBuffer.String())
					assert.Equal(t, "running stderr test\n", brt.payloadCommandData.stderrBuffer.String())
					assert.Equal(t, 1, brt.payloadCommandData.runningCmd.StatusCode())
					assert.True(t, brt.payloadCommandData.runningCmd.Finished())
					assert.True(t, brt.payloadCommandData.runningCmd.Killed())
					assert.Nil(t, brt.payloadCommandData.runningCmd.Error())
					assert.NotNil(t, brt.payloadCommandData.containerInfo)

					// Both running pointers should be empty
					assert.Nil(t, brt.executor.GetRunningCommand(brt.payloadCommandData.execCommand.IsSupport))
					assert.Nil(t, brt.executor.GetPreviousRunningCommand(brt.payloadCommandData.execCommand.IsSupport))
					return nil
				},
			},
		},
		{
			name: "container-recover-basic",
			data: &executorTestBaseParams{
				runPayload: true,
				runSupport: true,
				payloadCmdParams: executorCommandParams{
					image: executorTestImageBaseNonAlpine,
				},
				executorOpts: &common.ExecutorOpts{WorkspaceDirectory: "/tmp/workspace"},
				onDestroyStart: func(brt *baseRunTest) error {
					assert.NotNil(brt.t, brt.executor)
					if brt.executor != nil {
						assert.NoError(brt.t, brt.executor.Destroy())
					}
					return nil
				},
				onContainersRunning: func(brt *baseRunTest) error {
					loadedExecutor, err := NewContainerExecutor(
						brt.executorConfig, brt.runtime, brt.imageResolver,
						brt.runId, brt.testParams.executorOpts, logging.Logger,
					)
					assert.NoError(t, err)
					assert.True(t, loadedExecutor.Recovered())
					assert.Nil(t, loadedExecutor.GetPreviousRunningCommand(true))
					assert.Nil(t, loadedExecutor.GetPreviousRunningCommand(false))
					loadedSupportCmd := loadedExecutor.GetRunningCommand(true)
					loadedPayloadCmd := loadedExecutor.GetRunningCommand(false)
					assert.NotNil(t, loadedSupportCmd)
					assert.NotNil(t, loadedPayloadCmd)
					assert.NotNil(t, brt.supportCommandData)
					assert.NotNil(t, brt.payloadCommandData)
					if loadedPayloadCmd != nil && brt.payloadCommandData != nil {
						assert.Equal(t, brt.payloadCommandData.runningCmd.Id(), loadedPayloadCmd.Id())
					}
					if loadedSupportCmd != nil && brt.supportCommandData != nil {
						assert.Equal(t, brt.supportCommandData.runningCmd.Id(), loadedSupportCmd.Id())
					}
					return err
				},
			},
		},
	}
	for _, testData := range tests {
		t.Run(testData.name, func(t *testing.T) {
			newBaseRunTest(t, testData.data).run()
		})
	}
}

type executorCommandParams struct {
	scriptTime     int
	execCtxTimeout int
	image          string
}

type executorTestBaseParams struct {
	runPayload                bool
	runSupport                bool
	skipOutStreams            bool
	executorConfig            *config.ContainerExecutorConfig
	executorOpts              *common.ExecutorOpts
	payloadCmdParams          executorCommandParams
	supportCmdParams          executorCommandParams
	onInstantiatePre          func(*baseRunTest) error
	onDestroyStart            func(*baseRunTest) error
	onDestroyEnd              func(*baseRunTest) error
	onPrepareStart            func(*baseRunTest) error
	onPrepareEnd              func(*baseRunTest) error
	onSupportContainerRunning func(*baseRunTest) error
	onPayloadContainerRunning func(*baseRunTest) error
	onContainersRunning       func(*baseRunTest) error
	onContainersFinished      func(*baseRunTest) error
}

type commandData struct {
	stdoutBuffer  *bytes.Buffer
	stderrBuffer  *bytes.Buffer
	runningCmd    common.RunningCommand
	execCommand   *common.ExecutorCommand
	containerInfo map[string]interface{}
	testParams    *executorCommandParams
}

type baseRunTest struct {
	cleanupRegistry           *cleanupRegistry
	testParams                *executorTestBaseParams
	executor                  *ContainerExecutorImpl
	executorConfig            *config.ContainerExecutorConfig
	imageResolver             *stubImageResolver
	runId                     uuid.UUID
	runtime                   *podmanRuntime
	t                         *testing.T
	workspaceVolume           string
	supportCommandData        *commandData
	payloadCommandData        *commandData
	payloadWaitGroup          sync.WaitGroup
	containersRunningSignaled atomic.Bool
}

func newBaseRunTest(t *testing.T, testParams *executorTestBaseParams) *baseRunTest {
	return &baseRunTest{
		cleanupRegistry: newCleanupRegistry(t, true),
		testParams:      testParams,
		imageResolver:   &stubImageResolver{image: executorTestImageBaseAlpine},
		runId:           uuid.New(),
		t:               t,
	}
}

func (b *baseRunTest) runInstantiate() error {
	b.executorConfig = b.testParams.executorConfig
	if b.executorConfig == nil {
		b.executorConfig = &config.ContainerExecutorConfig{}
	}
	runtime, err := newPodmanRuntime(b.executorConfig)
	require.NoError(b.t, err)
	b.runtime = runtime

	executorOpts := b.testParams.executorOpts
	if executorOpts == nil {
		executorOpts = &common.ExecutorOpts{}
	}

	if b.testParams.onInstantiatePre != nil {
		assert.NoError(b.t, b.testParams.onInstantiatePre(b))
	}

	executor, err := NewContainerExecutor(b.executorConfig, runtime, b.imageResolver, b.runId, executorOpts, logging.Logger)
	assert.NoError(b.t, err)
	if err != nil {
		return err
	}
	b.executor = executor
	return nil
}

func (b *baseRunTest) runPrepare() error {
	if b.testParams.onPrepareStart != nil {
		assert.NoError(b.t, b.testParams.onPrepareStart(b))
	}
	err := b.executor.Prepare()
	assert.NoError(b.t, err)
	if err != nil {
		// If prepare already failed there is no point in continuing
		return err
	}

	if b.testParams.onPrepareEnd != nil {
		assert.NoError(b.t, b.testParams.onPrepareEnd(b))
	}

	workspaceVolumes, err := getPodmanVolumesByLabels(map[string]string{
		containerResourcesLabel:      containerResourcesLabelValue,
		containerResourcesLabelRunId: b.runId.String(),
	})
	assert.NoError(b.t, err)
	assert.NotNil(b.t, workspaceVolumes)
	assert.Len(b.t, workspaceVolumes, 1)
	if err == nil && workspaceVolumes != nil && len(workspaceVolumes) == 1 {
		b.workspaceVolume = workspaceVolumes[0]
	}
	assert.NoError(b.t, err)
	return err
}

func (b *baseRunTest) runCommand(commandParams *executorCommandParams) {
	defer b.payloadWaitGroup.Done()

	runTime := commandParams.scriptTime
	if runTime == 0 {
		runTime = 5
	}

	isSupport := commandParams == &b.testParams.supportCmdParams
	scriptBase := "echo 'running test'; echo 'running stderr test' >&2 ;sleep %d"
	cmd := &common.ExecutorCommand{
		ImageName: commandParams.image,
		IsSupport: isSupport,
		Script:    fmt.Sprintf(scriptBase, runTime),
		Command:   []string{"/bin/sh"},
	}

	var cmdData *commandData
	if isSupport {
		cmdData = b.supportCommandData
	} else {
		cmdData = b.payloadCommandData
	}
	cmdData.execCommand = cmd

	var outStreams *common.ExecutorStreams
	if !b.testParams.skipOutStreams {
		stdOutWriter := bytes.NewBufferString("")
		stdErrWriter := bytes.NewBufferString("")
		outStreams = &common.ExecutorStreams{
			OutputStream: stdOutWriter,
			ErrorStream:  stdErrWriter,
		}
		cmdData.stdoutBuffer = stdOutWriter
		cmdData.stderrBuffer = stdErrWriter
	}
	ctxTimeout := 10
	if commandParams.execCtxTimeout > 0 {
		ctxTimeout = commandParams.execCtxTimeout
	}
	ctx, _ := context.WithTimeout(context.Background(), time.Duration(ctxTimeout)*time.Second)
	runningCmd, err := b.executor.Execute(ctx, cmd, outStreams)
	assert.NoError(b.t, err)
	if err != nil {
		// Do not continue the test, is not worthy, it already failed
		// so no need to wait for the execution that won't happen
		return
	}
	cmdData.runningCmd = runningCmd
	containerData, err := waitGetContainerData(b.t, cmd, runningCmd, b.runId,
		func(rc common.RunningCommand, containerInfo map[string]interface{}) {
			cmdData.containerInfo = containerInfo
			if containerInfo != nil {
				b.cleanupRegistry.addContainerFromData(containerInfo)
			}
			if cmd.IsSupport && b.testParams.onSupportContainerRunning != nil {
				assert.NoError(b.t, b.testParams.onPayloadContainerRunning(b))
			} else if !cmd.IsSupport && b.testParams.onPayloadContainerRunning != nil {
				assert.NoError(b.t, b.testParams.onPayloadContainerRunning(b))
			}
			if b.testParams.onContainersRunning != nil && !b.containersRunningSignaled.Load() {
				bothRan := b.payloadCommandData != nil && b.payloadCommandData.containerInfo != nil &&
					b.supportCommandData != nil && b.supportCommandData.containerInfo != nil
				singleRun := (b.payloadCommandData != nil && b.payloadCommandData.containerInfo != nil) ||
					(b.supportCommandData != nil && b.supportCommandData.containerInfo != nil)
				if bothRan || singleRun {
					b.testParams.onContainersRunning(b)
					b.containersRunningSignaled.Store(true)
				}
			}
		})
	assert.NoError(b.t, err)
	assert.NotNil(b.t, containerData)
}

func (b *baseRunTest) runPayloads() {
	if b.testParams.runSupport {
		b.supportCommandData = &commandData{
			testParams: &b.testParams.supportCmdParams,
		}
		b.payloadWaitGroup.Add(1)
		go b.runCommand(&b.testParams.supportCmdParams)
	}
	if b.testParams.runPayload {
		b.payloadCommandData = &commandData{
			testParams: &b.testParams.payloadCmdParams,
		}
		b.payloadWaitGroup.Add(1)
		go b.runCommand(&b.testParams.payloadCmdParams)
	}
	b.payloadWaitGroup.Wait()

	if b.testParams.onContainersFinished != nil {
		b.testParams.onContainersFinished(b)
	}
}

func (b *baseRunTest) destroy() {
	if b.testParams.onDestroyStart != nil {
		assert.NoError(b.t, b.testParams.onDestroyStart(b))
	}
	b.cleanupRegistry.cleanup()
	if b.testParams.onDestroyEnd != nil {
		assert.NoError(b.t, b.testParams.onDestroyEnd(b))
	}
}

func (b *baseRunTest) run() {
	defer b.destroy()

	if err := b.runInstantiate(); err != nil {
		// If already failed there is no point in continuing
		return
	}

	if err := b.runPrepare(); err != nil {
		// If already failed there is no point in continuing
		return
	}

	b.runPayloads()
}
