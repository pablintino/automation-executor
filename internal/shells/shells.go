package shells

import (
	"io"
)

type ShellEnvironmentBuilder interface {
	BuildShellRun(logPath string, commands ...string) ([]string, map[string]string)
	BuildShellRunScript(logPath string, scriptReader io.Reader) ([]string, map[string]string, error)
}
