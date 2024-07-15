package test

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	pathsRelativeScripts   = "scripts"
	pathsRelativeScriptsDb = "db"
)

type TestPaths interface {
	RepoRoot() string
	Scripts() string
	DbScripts() string
}

type TestPathsImpl struct {
	repoRoot string
}

func (p *TestPathsImpl) RepoRoot() string {
	return p.repoRoot
}

func (p *TestPathsImpl) Scripts() string {
	return filepath.Join(p.repoRoot, pathsRelativeScripts)
}

func (p *TestPathsImpl) DbScripts() string {
	return filepath.Join(p.Scripts(), pathsRelativeScriptsDb)
}

func NewTestPaths(t *testing.T) *TestPathsImpl {
	root, err := GetRepoRoot()
	if err != nil {
		t.Fatal(err)
		return nil
	}
	return &TestPathsImpl{repoRoot: root}
}

func GetRepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	_, filename, _, _ := runtime.Caller(0)
	cmd.Dir = filepath.Dir(filename)
	pathBytes, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(pathBytes)), nil
}
