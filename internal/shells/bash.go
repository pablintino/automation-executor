package shells

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"path/filepath"

	"github.com/pablintino/automation-executor/internal/config"
)

type bashShellEnvironmentBuilder struct {
	shellConfig *config.ShellConfig
}

func NewBashShellEnvironmentBuilder(shellConfig *config.ShellConfig) *bashShellEnvironmentBuilder {
	return &bashShellEnvironmentBuilder{shellConfig: shellConfig}
}

func (b *bashShellEnvironmentBuilder) BuildShellRun(logPath string, commands ...string) ([]string, map[string]string) {
	var buf bytes.Buffer
	addBashHeader(&buf, b.shellConfig.Tracing, logPath)
	for _, cmd := range commands {
		buf.WriteString(cmd + "\n")
	}
	env := map[string]string{
		"EXECUTOR_SCRIPT": base64.StdEncoding.EncodeToString(buf.Bytes()),
		"SHELL":           "/bin/bash",
	}
	return []string{
		"/bin/bash",
		"-eco",
		"pipefail",
		"\"echo EXECUTOR_SCRIPT | base64 -d | /bin/bash -e\"",
	}, env

}

func (b *bashShellEnvironmentBuilder) BuildShellRunScript(logPath string, scriptReader io.Reader) ([]string, map[string]string, error) {
	pr, pw := io.Pipe()
	encoder := base64.NewEncoder(base64.StdEncoding, pw)
	gz := gzip.NewWriter(encoder)
	go func() {
		_, err := io.Copy(gz, scriptReader)
		gz.Close()
		encoder.Close()
		if err != nil {
			pw.CloseWithError(err)
		} else {
			pw.Close()
		}
	}()

	out, err := io.ReadAll(pr)
	if err != nil {
		return nil, nil, err
	}

	var buf bytes.Buffer
	addBashHeader(&buf, false, logPath)
	buf.WriteString(
		fmt.Sprintf(
			"base64 -d <<< \"$EXECUTOR_SCRIPT\" | gunzip | /bin/bash %s\n",
			buildBashOpts(b.shellConfig.Tracing),
		),
	)

	env := map[string]string{
		"EXECUTOR_SCRIPT":         string(out),
		"EXECUTOR_SUPPORT_SCRIPT": base64.StdEncoding.EncodeToString(buf.Bytes()),
		"SHELL":                   "/bin/bash",
	}
	return []string{
		"/bin/bash",
		"-eco",
		"pipefail",
		"\"echo $EXECUTOR_SUPPORT_SCRIPT | base64 -d | /bin/bash -e\"",
	}, env, nil
}

func addBashHeader(buf *bytes.Buffer, traceEnabled bool, logPath string) {
	buf.WriteString("#!/bin/bash\n")
	buf.WriteString(fmt.Sprintf("set %s\n", buildBashOpts(traceEnabled)))
	if logPath != "" {
		buf.WriteString(fmt.Sprintf("mkdir -p %s\n", filepath.Dir(logPath)))
		buf.WriteString(fmt.Sprintf("exec > >(tee %s) 2>&1\n", logPath))
	}
}

func buildBashOpts(traceEnabled bool) string {
	opts := "-eu"
	if traceEnabled {
		opts += "x"
	}
	opts += "o pipefail"
	return opts
}
