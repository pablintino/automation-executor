package container

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/pablintino/automation-executor/internal/config"
	"github.com/pablintino/automation-executor/internal/executors/common"
	"github.com/pablintino/automation-executor/internal/utils"
	"github.com/pablintino/automation-executor/logging"
)

const (
	containerResourcesLabel                          = "app"
	containerResourcesLabelValue                     = "automation-excutor"
	containerResourcesLabelRunId                     = "automation-executor-run-id"
	containerResourcesLabelContainerType             = "automation-executor-container-type"
	containerResourcesLabelContainerTypeValuePayload = "payload"
	containerResourcesLabelContainerTypeValueSupport = "support"
	volumeNamePrefix                                 = "automation-executor-volume"
	containerPayloadNamePrefix                       = "automation-executor-payload"
)

type containerRunningCommand interface {
	common.RunningCommand
	Container() Container
}

type cmdSyncStore struct {
	currentCmd  containerRunningCommand
	previousCmd containerRunningCommand
	mtx         sync.Mutex
}

func (c *cmdSyncStore) clear(cmd containerRunningCommand) {
	if cmd == nil {
		c.currentCmd = nil
		c.previousCmd = nil
	} else if cmd == c.currentCmd {
		c.currentCmd = nil
	} else if cmd == c.previousCmd {
		c.previousCmd = nil
	}
}

func (c *cmdSyncStore) push(cmd containerRunningCommand) {
	if c.currentCmd != nil {
		c.previousCmd = c.currentCmd
	}
	c.currentCmd = cmd
}

type ContainerExecutorImpl struct {
	config        *config.ContainerExecutorConfig
	runId         uuid.UUID
	payloadCmd    cmdSyncStore
	supportCmd    cmdSyncStore
	runtime       ContainerRuntime
	imageResolver supportImageResolver
	volumeName    string
	opts          *common.ExecutorOpts
	recovered     bool
}

func NewContainerExecutor(config *config.ContainerExecutorConfig, runtime ContainerRuntime, imageResolver supportImageResolver, runId uuid.UUID, opts *common.ExecutorOpts) (*ContainerExecutorImpl, error) {
	if opts == nil {
		return nil, fmt.Errorf("options cannot be nil")
	}
	if opts.WorkspaceDirectory == "" {
		return nil, fmt.Errorf("workspace directory cannot be empty")
	}
	instance := &ContainerExecutorImpl{config: config, runId: runId, runtime: runtime, imageResolver: imageResolver, opts: opts}
	return instance, instance.init()
}

func (e *ContainerExecutorImpl) Recovered() bool {
	return e.recovered
}

func (e *ContainerExecutorImpl) Prepare() error {
	err := e.prepareSharedVolume()
	if err != nil {
		return err
	}
	return nil
}

func (e *ContainerExecutorImpl) Destroy() error {
	var err error
	if errC := e.destroyContainers(); errC != nil {
		logging.Logger.Errorw("failed to destroy executor containers", "err", errC)
		err = errors.Join(err, errC)
	}
	if errV := e.destroySharedVolume(); errV != nil {
		logging.Logger.Errorw("failed to destroy executor shared volume", "err", err)
		err = errors.Join(err, errV)
	}
	return err
}

func (e *ContainerExecutorImpl) prepareSharedVolume() error {
	if e.volumeName != "" {
		// Already created
		return nil
	}
	volumeName := volumeNamePrefix + "-" + strings.ToLower(utils.RandomString(10))
	logging.Logger.Infow("creating executor volume", "volumeName", volumeName, "runId", e.runId.String())
	name, err := e.runtime.CreateVolume(volumeName, e.buildResourceLabels(nil))
	if err != nil {
		return err
	}
	e.volumeName = name
	return nil
}

func (e *ContainerExecutorImpl) destroySharedVolume() error {
	if e.volumeName != "" {
		if err := e.runtime.DeleteVolume(e.volumeName, false); err != nil {
			return err
		}
		e.volumeName = ""
	}
	return nil
}

func (e *ContainerExecutorImpl) destroyContainers() error {
	payloadDestroyErr := e.clearCmdStore(&e.payloadCmd)
	if payloadDestroyErr != nil {
		logging.Logger.Errorw("failed to destroy executor payload container", "err", payloadDestroyErr)
	}
	supportDestroyErr := e.clearCmdStore(&e.supportCmd)
	if supportDestroyErr != nil {
		logging.Logger.Errorw("failed to destroy executor support container", "err", supportDestroyErr)
	}
	return errors.Join(payloadDestroyErr, supportDestroyErr)
}

func (e *ContainerExecutorImpl) GetRunningCommand(support bool) common.RunningCommand {
	store := e.getStore(support)
	store.mtx.Lock()
	defer store.mtx.Unlock()
	return store.currentCmd
}

func (e *ContainerExecutorImpl) GetPreviousRunningCommand(support bool) common.RunningCommand {
	store := e.getStore(support)
	store.mtx.Lock()
	defer store.mtx.Unlock()
	return store.previousCmd
}

func (e *ContainerExecutorImpl) Execute(ctx context.Context, command *common.ExecutorCommand, streams *common.ExecutorStreams) (common.RunningCommand, error) {
	store := e.getStore(command.IsSupport)
	store.mtx.Lock()
	defer store.mtx.Unlock()

	if command.IsSupport && store.currentCmd != nil {
		return nil, fmt.Errorf("cannot execute a new support command as another one is still running")
	}
	if store.currentCmd != nil {
		return nil, fmt.Errorf("cannot execute new command as another one is still running")
	}

	cmdUuid := uuid.New()
	container, err := e.requestContainer(cmdUuid, command)
	if err != nil {
		return nil, err
	}

	cmd := newContainerAttachedCommand(cmdUuid, command, container, e)
	store.push(cmd)
	go cmd.attachRoutine(ctx, streams, true)
	return cmd, nil
}

func (e *ContainerExecutorImpl) requestContainer(cmdUuid uuid.UUID, command *common.ExecutorCommand) (Container, error) {
	var image string
	if command.IsSupport && command.ImageName == "" {
		supportImage, err := e.imageResolver.GetSupportImage()
		if err != nil {
			return nil, err
		}
		image = supportImage
	} else {
		if command.ImageName == "" {
			return nil, fmt.Errorf("image name is required")
		}
		image = command.ImageName
	}

	requiresInputStream := false
	if command.Script != "" {
		requiresInputStream = true
	}

	runOpts := &ContainerRunOpts{
		Command:       command.Command,
		Image:         image,
		Labels:        e.buildContainerLabels(nil, command.IsSupport),
		Volumes:       e.buildMounts(),
		Mounts:        e.config.ExtraMounts,
		PreserveStdin: requiresInputStream,
	}
	name := containerPayloadNamePrefix + "-" + strings.ReplaceAll(cmdUuid.String(), "-", "")[:10]
	container, err := e.runtime.CreateContainer(name, runOpts)
	return container, err
}

func (e *ContainerExecutorImpl) clearCmdStore(store *cmdSyncStore) error {
	store.mtx.Lock()
	defer store.mtx.Unlock()

	var err error
	if store.currentCmd != nil {
		err = e.destroyRunningCmdResources(store.currentCmd)
	}
	if store.previousCmd != nil {
		err = errors.Join(e.destroyRunningCmdResources(store.previousCmd), err)
	}
	store.clear(nil)
	return err
}

func (e *ContainerExecutorImpl) clearCmdStoreFromCmd(cmd *containerAttachedCommand, ignoreAlreadyCleared bool) error {
	store := e.getStore(cmd.cmd.IsSupport)
	store.mtx.Lock()
	defer store.mtx.Unlock()
	if !ignoreAlreadyCleared && store.currentCmd == nil {
		return errors.New("invalid attempt to clear the internal cmd store twice")
	} else if store.currentCmd == nil {
		// Do not attempt to clear it if already cleared and ignoreAlreadyCleared is true
		return nil
	}
	if store.currentCmd != cmd {
		runningId := ""
		if store.currentCmd != nil {
			runningId = store.currentCmd.Id().String()
		}
		return fmt.Errorf("cannot clear cmd store from a different running command. Running: %s, requested: %s ", runningId, cmd.Id())
	}
	if err := e.destroyRunningCmdResources(cmd); err != nil {
		return err
	}
	store.clear(cmd)
	return nil
}

func (e *ContainerExecutorImpl) destroyRunningCmdResources(cmd containerRunningCommand) error {
	containerId := cmd.Container().Id()
	containerExists, err := e.runtime.ExistsContainer(containerId)
	if err != nil {
		return err
	}
	if containerExists {
		return e.runtime.DestroyContainer(containerId)
	}
	return nil
}

func (e *ContainerExecutorImpl) buildResourceLabels(labels map[string]string) map[string]string {
	targetLabels := make(map[string]string)
	for key, value := range labels {
		targetLabels[key] = value
	}
	targetLabels[containerResourcesLabelRunId] = e.runId.String()
	targetLabels[containerResourcesLabel] = containerResourcesLabelValue
	return targetLabels
}

func (e *ContainerExecutorImpl) buildContainerLabels(labels map[string]string, isSupport bool) map[string]string {
	result := e.buildResourceLabels(labels)
	labelTypeValue := containerResourcesLabelContainerTypeValuePayload
	if isSupport {
		labelTypeValue = containerResourcesLabelContainerTypeValueSupport
	}
	result[containerResourcesLabelContainerType] = labelTypeValue
	return result
}

func (e *ContainerExecutorImpl) buildMounts() map[string]string {
	targetMounts := make(map[string]string)
	for key, value := range e.config.ExtraMounts {
		targetMounts[key] = value
	}
	targetMounts[e.volumeName] = e.opts.WorkspaceDirectory
	return targetMounts
}

func (e *ContainerExecutorImpl) init() error {
	// Check if there are resources already created
	volumeName, initVolErr := e.initVolume()
	if initVolErr != nil {
		logging.Logger.Errorw("failure recovering executor volume",
			"err", initVolErr, "runId", e.runId.String())
	}
	initSupportErr := e.initStore(true, volumeName)
	if initSupportErr != nil {
		logging.Logger.Errorw("failure recovering support container",
			"err", initSupportErr, "runId", e.runId.String())
	}
	initPayloadErr := e.initStore(false, volumeName)
	if initPayloadErr != nil {
		logging.Logger.Errorw("failure recovering payload container",
			"err", initPayloadErr, "runId", e.runId.String())
	}
	resultErr := errors.Join(initVolErr, initSupportErr, initPayloadErr)
	if resultErr != nil {
		if destroyCtrsErr := e.destroyContainers(); destroyCtrsErr != nil {
			logging.Logger.Errorw("failure destroying unrecoverable containers",
				"err", initPayloadErr, "runId", e.runId.String())
		}
		if volumeName != "" {
			if destroyVolErr := e.runtime.DeleteVolume(volumeName, true); destroyVolErr != nil {
				logging.Logger.Errorw("failure destroying unrecoverable containers",
					"err", initPayloadErr, "runId", e.runId.String())
			}
		}
		return resultErr
	}

	if volumeName != "" {
		// If the volume exists the executor was created by another call before
		e.recovered = true
	}
	return nil
}

func (e *ContainerExecutorImpl) initVolume() (string, error) {
	volumes, err := e.runtime.GetVolumesByLabels(e.buildResourceLabels(nil))
	if err != nil {
		return "", nil
	}
	targetVol := ""
	var resultError error
	for _, volName := range volumes {
		if targetVol == "" {
			targetVol = volName
		} else {
			resultError = fmt.Errorf("multiple volumes exists for the same runId %s volume: %s", e.runId.String(), targetVol)
		}
	}
	if resultError == nil {
		return targetVol, nil
	}

	for _, volName := range volumes {
		if err := e.runtime.DeleteVolume(volName, true); err != nil {
			resultError = errors.Join(resultError, err)
		}
	}

	return "", resultError
}

func (e *ContainerExecutorImpl) initStore(support bool, volume string) error {
	containerList, err := e.runtime.GetContainersByLabels(e.buildContainerLabels(nil, support))
	if err != nil {
		return err
	}
	var resultError error
	var containerCommand *containerAttachedCommand
	for _, item := range containerList {
		if containerCommand == nil {
			_, volumeMounted := item.GetRunOpts().Volumes[volume]
			var buildErr error
			if volumeMounted {
				containerCommand, buildErr = newContainerAttachedCommandFromContainer(item, e)
			} else {
				buildErr = fmt.Errorf("command container mathing cmdId has non consisted volumes %s", item.Id())
			}

			if buildErr != nil {
				resultError = errors.Join(resultError, buildErr)
			} else if containerCommand != nil {
				continue
			}
		}
		if err := e.runtime.DestroyContainer(item.Id()); err != nil {
			resultError = errors.Join(resultError, err)
		}
	}
	if resultError != nil && containerCommand != nil {
		if err := e.runtime.DestroyContainer(containerCommand.Container().Id()); err != nil {
			resultError = errors.Join(resultError, err)
		}
	}
	if resultError != nil || containerCommand == nil {
		return resultError
	}

	store := e.getStore(support)
	store.mtx.Lock()
	defer store.mtx.Unlock()
	if containerCommand.state.finished {
		store.previousCmd = containerCommand
	} else {
		store.currentCmd = containerCommand
	}
	return nil
}

func (e *ContainerExecutorImpl) getStore(isSupport bool) *cmdSyncStore {
	if isSupport {
		return &e.supportCmd
	} else {
		return &e.payloadCmd
	}
}

type containerAttachedCommand struct {
	id        uuid.UUID
	container Container
	cmd       *common.ExecutorCommand
	cmdDone   chan error
	cmdOnce   sync.Once
	executor  *ContainerExecutorImpl
	state     struct {
		code     int
		finished bool
		killed   bool
		err      error
		mtx      sync.RWMutex
	}
}

func newContainerAttachedCommand(cmdUuid uuid.UUID, cmd *common.ExecutorCommand, container Container, executor *ContainerExecutorImpl) *containerAttachedCommand {
	instance := &containerAttachedCommand{
		container: container,
		id:        cmdUuid,
		cmd:       cmd,
		executor:  executor,
		cmdDone:   make(chan error),
	}
	return instance
}

func newContainerAttachedCommandFromContainer(container Container, executor *ContainerExecutorImpl) (*containerAttachedCommand, error) {
	opts := container.GetRunOpts()
	cmdUuid := extractContainerRunId(opts.Labels)
	if cmdUuid == uuid.Nil {
		return nil, fmt.Errorf("cannot parse cmd ID from %s container labels", container.Id())

	}
	isSupport, err := extractContainerTypeSupportFromLabels(opts.Labels)
	if err != nil {
		return nil, fmt.Errorf("cannot parse container type from %s container labels", container.Id())
	}
	cmd := &common.ExecutorCommand{
		Command:   opts.Command,
		Script:    "",
		ImageName: opts.Image,
		Environ:   opts.Environ,
		IsSupport: isSupport,
	}
	instance := newContainerAttachedCommand(cmdUuid, cmd, container, executor)
	if err := instance.checkSetStateFromContainerState(); err != nil {
		return nil, err
	}
	if instance.state.finished {
		close(instance.cmdDone)
		instance.cmdOnce.Do(func() {})
	}
	return instance, nil
}

func (c *containerAttachedCommand) Id() uuid.UUID {
	return c.id
}
func (c *containerAttachedCommand) Finished() bool {
	c.state.mtx.Lock()
	defer c.state.mtx.Unlock()
	return c.state.finished
}

func (c *containerAttachedCommand) Killed() bool {
	c.state.mtx.Lock()
	defer c.state.mtx.Unlock()
	return c.state.killed
}

func (c *containerAttachedCommand) StatusCode() int {
	c.state.mtx.Lock()
	defer c.state.mtx.Unlock()
	return c.state.code
}

func (c *containerAttachedCommand) Error() error {
	c.state.mtx.Lock()
	defer c.state.mtx.Unlock()
	return c.state.err
}

func (c *containerAttachedCommand) Wait() error {
	finished, err := c.waitFinishedBarrier()
	if err != nil || finished {
		return err
	}
	//Wait till done (closed)
	<-c.cmdDone
	return c.state.err
}

func (c *containerAttachedCommand) AttachWait(ctx context.Context, streams *common.ExecutorStreams) error {
	finished, err := c.waitFinishedBarrier()
	if err != nil || finished {
		return err
	}

	c.cmdOnce.Do(func() {
		go c.attachRoutine(ctx, streams, false)
	})

	//Wait till done (closed)
	<-c.cmdDone
	return c.state.err
}

func (c *containerAttachedCommand) Kill() error {
	if !c.Finished() {
		// Delete the resources (container). If we are attached
		// the attach cmd will return, and it will process the
		// state fetch/set and release the waiting consumers.
		return c.executor.destroyRunningCmdResources(c)
	}
	return nil
}

func (c *containerAttachedCommand) Container() Container {
	return c.container
}

func (c *containerAttachedCommand) waitFinishedBarrier() (bool, error) {
	c.state.mtx.Lock()
	defer c.state.mtx.Unlock()
	return c.state.finished, c.state.err
}

func (c *containerAttachedCommand) attachRoutine(ctx context.Context, streams *common.ExecutorStreams, isNew bool) {

	var reader io.Reader
	// Only attach in the first run, when the start is performed
	if c.cmd.Script != "" {
		reader = strings.NewReader(c.cmd.Script)
	}
	runStreams := &ContainerStreams{
		Output: streams.OutputStream,
		Error:  streams.ErrorStream,
		Input:  reader,
	}
	requiresStart := isNew
	canAttach := isNew
	// Avoid running the preflight if the container was created in the same run
	if !isNew {
		requiresStart, canAttach = c.preFlightCheck()
	}

	logging.Logger.Debugw("container about to be started/attached",
		"cmdId", c.Id(),
		"runId", c.executor.runId,
		"containerId", c.container.Id(),
		"startRequired", requiresStart,
		"scriptLength", len(c.cmd.Script),
	)

	var err error
	if requiresStart {
		err = c.container.StartAttach(ctx, runStreams)
	} else if canAttach {
		err = c.container.Attach(ctx, runStreams)
	}
	logging.Logger.Debugw("container attach finished",
		"cmdId", c.Id(),
		"runId", c.executor.runId,
		"containerId", c.container.Id(),
	)
	if err := c.postRunSetState(err); err != nil {
		logging.Logger.Debugw("post run error",
			"error", err,
			"cmdId", c.Id(),
			"runId", c.executor.runId,
			"containerId", c.container.Id(),
		)
	}

	// Signal all the waiting routing we are done
	close(c.cmdDone)
}

func (c *containerAttachedCommand) postRunSetState(runErr error) error {
	c.state.mtx.Lock()
	defer c.state.mtx.Unlock()

	if runErr == nil {
		exists, err := c.executor.runtime.ExistsContainer(c.container.Id())
		if err != nil {
			// In case we are not able to get if it exists
			// consider it as failed for simplicity
			c.setStateError(err)
		}

		if exists {
			c.checkSetStateFromContainerState()
		} else {
			// If the container doesn't exit it's usually because
			// it has been destroyed underneath
			c.state.code = 1
			c.state.killed = true
			c.state.finished = true
		}

	} else {
		c.setStateError(runErr)
	}

	logging.Logger.Debugw("container post run reached",
		"cmdId", c.Id(),
		"runId", c.executor.runId,
		"containerId", c.container.Id(),
		"exitCode", c.state.code,
		"error", c.state.err,
		"killed", c.state.killed,
	)

	clearErr := c.executor.clearCmdStoreFromCmd(c, false)
	return errors.Join(c.state.err, clearErr)
}

func (c *containerAttachedCommand) checkSetStateFromContainerState() error {
	state, err := c.executor.runtime.GetState(c.container.Id())
	if err != nil {
		// In case we are not able to get the state
		// consider it as failed for simplicity
		c.setStateError(err)
		return err
	}
	currentState := strings.ToLower(state.Status)
	if currentState == containerStateStopped || currentState == containerStateExited {
		c.state.code = int(state.ExitCode)
		c.state.finished = true
	}
	return err
}

func (c *containerAttachedCommand) preFlightCheck() (bool, bool) {
	// Lock not required as it's a read only operation
	// from the same routine that sets the state
	if c.state.finished {
		return false, false
	}
	exists, err := c.executor.runtime.ExistsContainer(c.container.Id())
	if err != nil || !exists {
		return false, false
	}

	state, err := c.executor.runtime.GetState(c.container.Id())
	if err != nil {
		return false, false
	}
	currentState := strings.ToLower(state.Status)
	if currentState == containerStateExited {
		return false, false
	}
	return currentState == containerStateCreated || currentState == containerStateConfigured,
		currentState == containerStateRunning
}

func (c *containerAttachedCommand) setStateError(err error) {
	// Assume the called has properly called the mutex
	c.state.finished = true
	c.state.code = 1
	c.state.err = err
}

func extractContainerRunId(labels map[string]string) uuid.UUID {
	strRunId, ok := labels[containerResourcesLabelRunId]
	if !ok {
		return uuid.Nil
	}
	id, err := uuid.Parse(strRunId)
	if err != nil {
		return uuid.Nil
	}
	return id
}
func extractContainerTypeSupportFromLabels(labels map[string]string) (bool, error) {
	containerType, ok := labels[containerResourcesLabelContainerType]
	if !ok {
		return false, errors.New("container type label not present")
	}
	return containerType == containerResourcesLabelContainerTypeValueSupport, nil
}
