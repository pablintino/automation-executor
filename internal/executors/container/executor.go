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
	containerResourcesLabel      = "app"
	containerResourcesLabelValue = "automation-excutor"
	containerResourcesRunIdLabel = "automation-executor-run-id"
	volumeNamePrefix             = "automation-executor-volume"
	containerPayloadNamePrefix   = "automation-executor-payload"
)

type containerRunningCommand interface {
	common.RunningCommand
	Container() Container
}

type cmdSyncStore struct {
	cmd containerRunningCommand
	mtx sync.Mutex
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
}

func NewContainerExecutor(config *config.ContainerExecutorConfig, runtime ContainerRuntime, imageResolver supportImageResolver, runId uuid.UUID, opts *common.ExecutorOpts) (*ContainerExecutorImpl, error) {
	if opts == nil {
		return nil, fmt.Errorf("options cannot be nil")
	}
	if opts.WorkspaceDirectory == "" {
		return nil, fmt.Errorf("workspace directory cannot be empty")
	}
	return &ContainerExecutorImpl{config: config, runId: runId, runtime: runtime, imageResolver: imageResolver, opts: opts}, nil
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
	if err = e.destroySharedVolume(); err != nil {
		logging.Logger.Errorw("failed to destroy executor shared volume", "err", err)
	}
	if errC := e.destroyContainers(); errC != nil {
		logging.Logger.Errorw("failed to destroy executor containers", "err", errC)
		err = errors.Join(err, errC)
	}
	return err
}

func (e *ContainerExecutorImpl) prepareSharedVolume() error {
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
		return e.runtime.DeleteVolume(e.volumeName)
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

func (e *ContainerExecutorImpl) Execute(ctx context.Context, command *common.ExecutorCommand, streams *common.ExecutorStreams) (common.RunningCommand, error) {
	var store *cmdSyncStore
	if command.IsSupport {
		store = &e.supportCmd
	} else {
		store = &e.payloadCmd
	}
	store.mtx.Lock()
	defer store.mtx.Unlock()

	if command.IsSupport && store.cmd != nil {
		return nil, fmt.Errorf("cannot execute a new support command as another one is still running")
	}
	if store.cmd != nil {
		return nil, fmt.Errorf("cannot execute new command as another one is still running")
	}

	cmdUuid := uuid.New()
	container, err := e.requestContainer(cmdUuid, command)
	if err != nil {
		return nil, err
	}

	cmd := newContainerAttachedCommand(cmdUuid, command, container, e)
	cmd.start(ctx, streams)
	store.cmd = cmd
	return store.cmd, nil
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
		Command:     command.Command,
		Image:       image,
		Labels:      e.buildResourceLabels(nil),
		Volumes:     e.buildMounts(),
		Mounts:      e.config.ExtraMounts,
		AttachStdin: requiresInputStream,
	}
	name := containerPayloadNamePrefix + "-" + strings.ReplaceAll(cmdUuid.String(), "-", "")[:10]
	container, err := e.runtime.CreateContainer(name, runOpts)
	return container, err
}

func (e *ContainerExecutorImpl) clearCmdStore(store *cmdSyncStore) error {
	store.mtx.Lock()
	defer store.mtx.Unlock()
	if store.cmd == nil {
		return nil
	}
	if err := e.destroyStoreResources(store); err != nil {
		return err
	}
	// Clear the pointer to signal the store cmd is gone
	store.cmd = nil
	return nil
}

func (e *ContainerExecutorImpl) clearCmdStoreFromCmd(cmd *containerAttachedCommand) error {
	var store *cmdSyncStore
	if cmd.command.IsSupport {
		store = &e.supportCmd
	} else {
		store = &e.payloadCmd
	}
	store.mtx.Lock()
	defer store.mtx.Unlock()
	if store.cmd != cmd {
		runningId := ""
		if store.cmd != nil {
			runningId = store.cmd.Id().String()
		}
		return fmt.Errorf("cannot clear cmd store from a different running command. Running: %s, requested: %s ", runningId, cmd.Id())
	}
	if err := e.destroyStoreResources(store); err != nil {
		return err
	}
	// Clear the pointer to signal the store cmd is gone
	store.cmd = nil
	return nil
}

func (e *ContainerExecutorImpl) destroyStoreResources(store *cmdSyncStore) error {
	containerId := store.cmd.Container().Id()
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
	targetLabels[containerResourcesRunIdLabel] = e.runId.String()
	targetLabels[containerResourcesLabel] = containerResourcesLabelValue
	return targetLabels
}

func (e *ContainerExecutorImpl) buildMounts() map[string]string {
	targetMounts := make(map[string]string)
	for key, value := range e.config.ExtraMounts {
		targetMounts[key] = value
	}
	targetMounts[e.volumeName] = e.opts.WorkspaceDirectory
	return targetMounts
}

type containerAttachedCommand struct {
	id        uuid.UUID
	container Container
	command   *common.ExecutorCommand
	waiter    chan error
	exectutor *ContainerExecutorImpl
	state     struct {
		code     int
		finished bool
		killed   bool
		err      error
		mtx      sync.RWMutex
	}
}

func newContainerAttachedCommand(cmdUuid uuid.UUID, command *common.ExecutorCommand, container Container, exectutor *ContainerExecutorImpl) *containerAttachedCommand {
	return &containerAttachedCommand{
		container: container,
		id:        cmdUuid,
		command:   command,
		exectutor: exectutor,
		waiter:    make(chan error),
	}
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
	return <-c.waiter
}

func (c *containerAttachedCommand) Destroy() error {
	return c.exectutor.clearCmdStoreFromCmd(c)
}

func (c *containerAttachedCommand) Container() Container {
	return c.container
}

func (c *containerAttachedCommand) start(ctx context.Context, streams *common.ExecutorStreams) {
	var reader io.Reader
	if c.command.Script != "" {
		reader = strings.NewReader(c.command.Script)
	}
	runStreams := &ContainerStreams{
		Output: streams.OutputStream,
		Error:  streams.ErrorStream,
		Input:  reader,
	}
	go func() {
		if err := c.container.StartAttach(ctx, runStreams); err != nil {
			c.state.mtx.Lock()
			c.setStateError(err)
			c.state.mtx.Unlock()
		} else {
			c.checkSetExitState()
		}
		c.exectutor.clearCmdStoreFromCmd(c)
		c.waiter <- c.state.err
	}()
}

func (c *containerAttachedCommand) checkSetExitState() bool {
	c.state.mtx.Lock()
	defer c.state.mtx.Unlock()
	if c.state.finished {
		return true
	}
	exists, err := c.exectutor.runtime.ExistsContainer(c.container.Id())
	if err != nil {
		// In case we are not able to get if it exists
		// consider it as failed for simplicity
		c.setStateError(err)
		return true
	}

	// If the container doesn't exit it's usually because
	// it has been destroyed underneath
	if !exists {
		c.state.code = 1
		c.state.killed = true
		c.state.finished = true
		return c.state.finished
	}

	state, err := c.exectutor.runtime.GetState(c.container.Id())
	if err != nil {
		// In case we are not able to get the state
		// consider it as failed for simplicity
		c.setStateError(err)
		return true
	}
	currentState := strings.ToLower(state.Status)
	if currentState == containerStateStopped || currentState == containerStateExited {
		c.state.code = int(state.ExitCode)
		c.state.finished = true
	}
	return c.state.finished
}

func (c *containerAttachedCommand) setStateError(err error) {
	// Assume the called has properly called the mutex
	c.state.finished = true
	c.state.code = 1
	c.state.err = err
}
