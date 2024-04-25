package shells

import (
	"fmt"
	"github.com/pablintino/automation-executor/internal/config"
	"github.com/pablintino/automation-executor/logging"
	"testing"
)

func TestBashShellEnvironmentBuilderRegularCommand(t *testing.T) {
	builder := NewBashShellEnvironmentBuilder(&config.ShellConfig{Tracing: true})
	cmd, env := builder.BuildShellRun("/tmp/test-dir/var/log-out.log", "date", "uptime", "free -m")
	fmt.Println(cmd)
	fmt.Println(env)

	logging.Initialize(false)
	defer logging.Release()

	data := []struct {
		name     string
		logPath  string
		commands []string
		config   *config.ShellConfig
	}{
		{
			name:    "simple-run",
			logPath: "/tmp/test-dir/var/log-out.log",
		},
	}

	for _, tt := range data {
		t.Run(tt.name, func(t *testing.T) {
			cmd, env := builder.BuildShellRun(tt.logPath, tt.commands...)
		})
	}
}
