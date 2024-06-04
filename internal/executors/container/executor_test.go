package container

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
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
	defaultCmdStdoutMessagePrefix  string = "running test"
	defaultCmdStderrMessagePrefix  string = "running stderr test"
	defaultCmdTemplate             string = "for i in $(seq $((%d*2))); do " +
		"echo \"" + defaultCmdStdoutMessagePrefix + " $i\";" +
		"echo \"" + defaultCmdStderrMessagePrefix + " $i\" >&2;" +
		"sleep 0.5; done"
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
	r.addContainer(data["Id"].(string))
}

func (r *cleanupRegistry) addContainer(id string) {
	r.containers = append(r.containers, id)
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

func assertContainerCommandSuccessStream(brt *baseRunTest, bufferContent string, prefix string, runTime int, strict bool) {
	assert.NotEmpty(brt.t, bufferContent)
	dataLines := strings.Split(strings.Trim(bufferContent, "\n"), "\n")
	if strict {
		// The reference command prints each 0.5 second
		// We cannot warranty data length in runs that
		// started from a recover or that were killed
		// Strict serves that purpose, allow to not
		// check the length and if the output should
		// start from zero
		assert.Len(brt.t, dataLines, runTime*2)
	} else {
		// For not strict checks we check that
		// at least one line exists
		assert.Greater(brt.t, len(dataLines), 0)
	}
	// Not efficient but we expect test commands to be quick and short
	// Note: Maybe the check is too strict, for now it works, but we
	// can assume that a line at the end may be missing as sleep is not
	// that precise
	currentValue := 0
	for index, lineData := range dataLines {
		prefixIdx := strings.Index(lineData, prefix)
		if prefixIdx != 0 {
			assert.Equal(brt.t, 0, prefixIdx)
			return
		}
		split := strings.Split(lineData, " ")
		lineIndex := split[len(split)-1]
		val, err := strconv.Atoi(lineIndex)
		assert.NoError(brt.t, err)
		if err != nil {
			return
		}
		if index == 0 && strict {
			currentValue = 1
		} else if index == 0 {
			currentValue = val
		} else {
			currentValue++
		}
		if val != currentValue {
			assert.Equal(brt.t, currentValue, val)
			return
		}
	}
}

func assertContainerCommandSuccessStreams(brt *baseRunTest, cmdData *commandData, strict bool) {
	if brt.testParams.skipOutStreams {
		return
	}
	assert.NotNil(brt.t, cmdData.stdoutBuffer)
	assert.NotNil(brt.t, cmdData.stderrBuffer)

	if cmdData.stdoutBuffer != nil {
		assertContainerCommandSuccessStream(
			brt, cmdData.stdoutBuffer.String(),
			defaultCmdStdoutMessagePrefix,
			cmdData.testParams.getRuntime(),
			strict)
	}
	if cmdData.stderrBuffer != nil {
		assertContainerCommandSuccessStream(
			brt, cmdData.stderrBuffer.String(),
			defaultCmdStderrMessagePrefix,
			cmdData.testParams.getRuntime(),
			strict)
	}
}

func assertContainerCommandSuccess(brt *baseRunTest, data *commandData) {
	assert.NotNil(brt.t, data.runningCmd)
	assertContainerCommandSuccessStreams(brt, data, !brt.testParams.preCreateRunningResources)
	if data.runningCmd != nil {
		assert.Equal(brt.t, 0, data.runningCmd.StatusCode())
		assert.True(brt.t, data.runningCmd.Finished())
		assert.False(brt.t, data.runningCmd.Killed())
		assert.Nil(brt.t, data.runningCmd.Error())
	}
	// Both running pointers should be empty
	isSupport := data == brt.supportCommandData
	assert.Nil(brt.t, brt.executor.GetRunningCommand(isSupport))
	assert.Nil(brt.t, brt.executor.GetPreviousRunningCommand(isSupport))
	assert.NotNil(brt.t, data.containerInfo)
	if data.containerInfo == nil {
		// If no container could be fetched no way to assert against that data
		return
	}

	assertVolumeIsMounted(brt.t, data.containerInfo, brt.workspaceVolume, brt.testParams.executorOpts.WorkspaceDirectory)
	ranImage := data.containerInfo["ImageName"].(string)
	if isSupport && data.testParams.image == "" {
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

func assertKilledContainerOnContainersFinished(brt *baseRunTest, cmdData *commandData) {
	assertContainerCommandSuccessStreams(brt, cmdData, false)
	assert.NotNil(brt.t, cmdData.runningCmd)
	if cmdData.runningCmd != nil {
		assert.Equal(brt.t, 1, cmdData.runningCmd.StatusCode())
		assert.True(brt.t, cmdData.runningCmd.Finished())
		assert.True(brt.t, cmdData.runningCmd.Killed())
		assert.Nil(brt.t, cmdData.runningCmd.Error())
		assert.NotNil(brt.t, cmdData.containerInfo)
	}

	assert.NotNil(brt.t, cmdData.runningCmd)
	if cmdData.runningCmd != nil {
		// Both running pointers should be empty
		assert.Nil(brt.t, brt.executor.GetRunningCommand(cmdData.execCommand.IsSupport))
		assert.Nil(brt.t, brt.executor.GetPreviousRunningCommand(cmdData.execCommand.IsSupport))
	}
}

func assertKilledOnContainersFinished(brt *baseRunTest) error {
	if brt.testParams.runSupport {
		assert.NotNil(brt.t, brt.supportCommandData)
		if brt.supportCommandData != nil {
			assertKilledContainerOnContainersFinished(brt, brt.supportCommandData)
		}
	}
	if brt.testParams.runPayload {
		assert.NotNil(brt.t, brt.payloadCommandData)
		if brt.payloadCommandData != nil {
			assertKilledContainerOnContainersFinished(brt, brt.payloadCommandData)
		}
	}
	return nil
}

func assertContainerRecovered(brt *baseRunTest, cmdData *commandData) {
	isSupport := cmdData == brt.supportCommandData
	runningCmd := brt.executor.GetRunningCommand(isSupport)
	assert.NotNil(brt.t, runningCmd)
	if runningCmd != nil {
		assert.Nil(brt.t, brt.executor.GetPreviousRunningCommand(isSupport))
		assert.False(brt.t, runningCmd.Finished())
		assert.False(brt.t, runningCmd.Killed())
		assert.NoError(brt.t, runningCmd.Error())
		assert.Equal(brt.t, 0, runningCmd.StatusCode())
	}

	assert.NotNil(brt.t, cmdData.containerInfo)
	if cmdData.containerInfo != nil {
		assertContainerLabelExists(brt.t, cmdData.containerInfo,
			containerResourcesLabelCmdId, runningCmd.Id().String())
		assertContainerLabelExists(brt.t, cmdData.containerInfo,
			containerResourcesLabelRunId, brt.runId.String())
	}
}

func assertSuccessOnInstantiatePostFromScratchRecover(brt *baseRunTest) error {
	assert.True(brt.t, brt.executor.Recovered())
	assert.Equal(brt.t, brt.executor.runId, brt.executor.Id())

	if brt.testParams.runPayload {
		assert.NotNil(brt.t, brt.payloadCommandData)
		if brt.payloadCommandData != nil {
			assertContainerRecovered(brt, brt.payloadCommandData)
		}
	}
	if brt.testParams.runSupport {
		assert.NotNil(brt.t, brt.supportCommandData)
		if brt.supportCommandData != nil {
			assertContainerRecovered(brt, brt.supportCommandData)
		}
	}
	return nil
}

func commonExecutorOnDestroyStart(brt *baseRunTest) error {
	assert.NotNil(brt.t, brt.executor)
	if brt.executor != nil {
		assert.NoError(brt.t, brt.executor.Destroy())
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
				executorOpts:         &common.ExecutorOpts{WorkspaceDirectory: "/tmp/workspace"},
				onDestroyStart:       commonExecutorOnDestroyStart,
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
				executorOpts:         &common.ExecutorOpts{WorkspaceDirectory: "/tmp/workspace"},
				onDestroyStart:       commonExecutorOnDestroyStart,
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
				executorOpts:         &common.ExecutorOpts{WorkspaceDirectory: "/tmp/workspace"},
				onDestroyStart:       commonExecutorOnDestroyStart,
				onContainersFinished: assertSuccessOnContainersFinished,
			},
		},
		{
			name: "container-create-support-internal-image-basic",
			data: &executorTestBaseParams{
				runPayload:           false,
				runSupport:           true,
				supportCmdParams:     executorCommandParams{},
				executorOpts:         &common.ExecutorOpts{WorkspaceDirectory: "/tmp/workspace"},
				onDestroyStart:       commonExecutorOnDestroyStart,
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
				executorOpts:   &common.ExecutorOpts{WorkspaceDirectory: "/tmp/workspace"},
				onDestroyStart: commonExecutorOnDestroyStart,
				onContainersRunning: func(brt *baseRunTest) error {
					assert.NotNil(brt.t, brt.payloadCommandData)
					if brt.payloadCommandData != nil {
						assert.NoError(brt.t, brt.payloadCommandData.runningCmd.Kill())
					}
					return nil
				},
				onContainersFinished: assertKilledOnContainersFinished,
			},
		},
		{
			name: "container-timeout-basic",
			data: &executorTestBaseParams{
				runPayload: true,
				runSupport: false,
				payloadCmdParams: executorCommandParams{
					image:          executorTestImageBaseNonAlpine,
					execCtxTimeout: 2,
					scriptTime:     5,
				},
				executorOpts:         &common.ExecutorOpts{WorkspaceDirectory: "/tmp/workspace"},
				onDestroyStart:       commonExecutorOnDestroyStart,
				onContainersFinished: assertKilledOnContainersFinished,
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
				executorOpts:   &common.ExecutorOpts{WorkspaceDirectory: "/tmp/workspace"},
				onDestroyStart: commonExecutorOnDestroyStart,
				onContainersRunning: func(brt *baseRunTest) error {
					loadedExecutor, err := NewContainerExecutor(
						brt.executorConfig, brt.runtime, brt.imageResolver,
						brt.runId, brt.testParams.executorOpts, logging.Logger,
					)
					assert.NoError(brt.t, err)
					assert.True(brt.t, loadedExecutor.Recovered())
					assert.Equal(brt.t, brt.executor.runId, loadedExecutor.Id())
					assert.Nil(brt.t, loadedExecutor.GetPreviousRunningCommand(true))
					assert.Nil(brt.t, loadedExecutor.GetPreviousRunningCommand(false))
					loadedSupportCmd := loadedExecutor.GetRunningCommand(true)
					loadedPayloadCmd := loadedExecutor.GetRunningCommand(false)
					assert.NotNil(brt.t, loadedSupportCmd)
					assert.NotNil(brt.t, loadedPayloadCmd)
					assert.NotNil(brt.t, brt.supportCommandData)
					assert.NotNil(brt.t, brt.payloadCommandData)
					if loadedPayloadCmd != nil && brt.payloadCommandData != nil {
						assert.Equal(brt.t, brt.payloadCommandData.runningCmd.Id(), loadedPayloadCmd.Id())
					}
					if loadedSupportCmd != nil && brt.supportCommandData != nil {
						assert.Equal(brt.t, brt.supportCommandData.runningCmd.Id(), loadedSupportCmd.Id())
					}
					return err
				},
			},
		},
		{
			name: "container-recover-from-scratch-basic",
			data: &executorTestBaseParams{
				preCreateRunningResources: true,
				runPayload:                true,
				runSupport:                true,
				payloadCmdParams: executorCommandParams{
					image: executorTestImageBaseNonAlpine,
				},
				supportCmdParams: executorCommandParams{
					image: executorTestImageBaseNonAlpine,
				},
				executorOpts:         &common.ExecutorOpts{WorkspaceDirectory: "/tmp/workspace"},
				onDestroyStart:       commonExecutorOnDestroyStart,
				onInstantiatePost:    assertSuccessOnInstantiatePostFromScratchRecover,
				onContainersFinished: assertSuccessOnContainersFinished,
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

func (c *executorCommandParams) getRuntime() int {
	runTime := c.scriptTime
	if runTime == 0 {
		runTime = 5
	}
	return runTime
}

type executorTestBaseParams struct {
	runPayload                bool
	runSupport                bool
	preCreateRunningResources bool
	skipOutStreams            bool
	executorConfig            *config.ContainerExecutorConfig
	executorOpts              *common.ExecutorOpts
	payloadCmdParams          executorCommandParams
	supportCmdParams          executorCommandParams
	onInstantiatePre          func(*baseRunTest) error
	onInstantiatePost         func(*baseRunTest) error
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

	if b.testParams.onInstantiatePost != nil {
		assert.NoError(b.t, b.testParams.onInstantiatePost(b))
	}

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

	isSupport := commandParams == &b.testParams.supportCmdParams
	cmd := &common.ExecutorCommand{
		ImageName: commandParams.image,
		IsSupport: isSupport,
		Script:    fmt.Sprintf(defaultCmdTemplate, commandParams.getRuntime()),
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
	// Do not assert the returned err, as we may want to check it later
	assert.NotNil(b.t, containerData)
}

func (b *baseRunTest) attachToRunningCmd(commandParams *executorCommandParams) {
	defer b.payloadWaitGroup.Done()

	isSupport := commandParams == &b.testParams.supportCmdParams

	var cmdData *commandData
	if isSupport {
		cmdData = b.supportCommandData
	} else {
		cmdData = b.payloadCommandData
	}

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

	runningCmd := b.executor.GetRunningCommand(isSupport)
	assert.NotNil(b.t, runningCmd)
	if runningCmd == nil {
		// Do not continue the test, is not worthy, it already failed
		// so no need to wait for the execution that won't happen
		return
	}
	ctx, _ := context.WithTimeout(context.Background(), time.Duration(ctxTimeout)*time.Second)
	cmdData.runningCmd = runningCmd
	_ = runningCmd.AttachWait(ctx, outStreams)
	// Do not assert the returned err, as we may want to check it later
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

func (b *baseRunTest) attachPayloads() {
	if b.testParams.runSupport {
		b.payloadWaitGroup.Add(1)
		go b.attachToRunningCmd(&b.testParams.supportCmdParams)
	}
	if b.testParams.runPayload {
		b.payloadWaitGroup.Add(1)
		go b.attachToRunningCmd(&b.testParams.payloadCmdParams)
	}
	b.payloadWaitGroup.Wait()

	if b.testParams.onContainersFinished != nil {
		b.testParams.onContainersFinished(b)
	}
}

func (b *baseRunTest) preCreateRunningResources() error {
	volName, err := createVolume(
		fmt.Sprintf("automation-executor-testing-vol-%s", strings.Trim(uuid.New().String()[:10], "-")),
		map[string]string{
			containerResourcesLabelRunId: b.runId.String(),
			containerResourcesLabel:      containerResourcesLabelValue,
		})
	if err != nil {
		return err
	}
	b.workspaceVolume = volName
	b.cleanupRegistry.addVolume(volName)
	if b.testParams.runSupport {
		supportImage := b.testParams.supportCmdParams.image
		if supportImage == "" {
			supportImage = b.imageResolver.image
		}
		b.supportCommandData = &commandData{
			testParams: &b.testParams.supportCmdParams,
		}
		err = b.preCreateContainer(supportImage, b.supportCommandData)
		if err != nil {
			return err
		}
	}
	if b.testParams.runPayload {
		b.payloadCommandData = &commandData{
			testParams: &b.testParams.payloadCmdParams,
		}
		err = b.preCreateContainer(b.testParams.payloadCmdParams.image, b.payloadCommandData)
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *baseRunTest) preCreateContainer(containerImage string, cmdData *commandData) error {
	assert.NotEmpty(b.t, containerImage)
	isSupport := cmdData == b.supportCommandData
	containerType := containerResourcesContainerTypeValueSupport
	if !isSupport {
		containerType = containerResourcesContainerTypeValuePayload
	}

	containerId, err := createContainer(
		fmt.Sprintf(
			"automation-executor-%s-container-%s",
			containerType,
			strings.Trim(uuid.New().String()[:10], "-"),
		),
		containerImage,
		[]string{"/bin/sh", "-c", fmt.Sprintf(defaultCmdTemplate, cmdData.testParams.getRuntime())},
		map[string]string{
			containerResourcesLabelRunId:         b.runId.String(),
			containerResourcesLabel:              containerResourcesLabelValue,
			containerResourcesLabelContainerType: containerType,
			containerResourcesLabelCmdId:         uuid.New().String(),
		}, map[string]string{
			b.workspaceVolume: b.testParams.executorOpts.WorkspaceDirectory,
		},
	)
	if err != nil {
		return err
	}
	b.cleanupRegistry.addContainer(containerId)
	inspectData, err := inspectContainer(containerId)
	assert.NoError(b.t, err)
	assert.NotNil(b.t, inspectData)
	if err != nil {
		return err
	}
	cmdData.containerInfo = inspectData
	return nil
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

	if b.testParams.preCreateRunningResources {
		if err := b.preCreateRunningResources(); err != nil {
			// If already failed there is no point in continuing
			return
		}
	}

	if err := b.runInstantiate(); err != nil {
		// If already failed there is no point in continuing
		return
	}

	if err := b.runPrepare(); err != nil {
		// If already failed there is no point in continuing
		return
	}

	if !b.testParams.preCreateRunningResources {
		// Run is for crating a running env from scratch
		// Recovered envs have already running containers
		b.runPayloads()
	} else {
		b.attachPayloads()
	}
}
