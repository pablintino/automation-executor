package environment

import (
	"io"
)

type EnvironmentCreationOpts struct {
	Image string
}

type Environment interface {
	Bootstrap() error
	RunCommand(command ...string) error
	FetchFromFS(writer io.Writer, paths ...string) error
	CopyToFS(reader io.Reader, path string) error
	Destroy() error
}

type EnvironmentService interface {
	CreateContainerEnv(opts *EnvironmentCreationOpts) (Environment, error)
}

type EnvironmentServiceImpl struct {
}
