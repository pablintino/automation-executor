package common

import (
	"context"
	"io"

	"github.com/google/uuid"
)

type ExecutorCommand struct {
	Environ map[string]string
	Script  string
	Command []string
	// Optional if the executor is not based on containers (not docker non k8s)
	ImageName string
	IsSupport bool
}

type ExecutorStreams struct {
	OutputStream io.Writer
	ErrorStream  io.Writer
}

type ExecutorOpts struct {
	WorkspaceDirectory string
}

type RunningCommand interface {
	Id() uuid.UUID
	Finished() bool
	Killed() bool
	StatusCode() int
	Error() error
	Wait() error
	Destroy() error
}

type Executor interface {
	Prepare() error
	Destroy() error
	Execute(ctx context.Context, command *ExecutorCommand, streams *ExecutorStreams) (RunningCommand, error)
}

type ExecutorFactory interface {
	GetExecutor(runId uuid.UUID, opts *ExecutorOpts) (Executor, error)
}
