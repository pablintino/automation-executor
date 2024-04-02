package containers

import "github.com/pablintino/automation-executor/internal/git"

type ContainerEnv struct {
	id           string
	Image        string
	Repositories *git.GitRepos
	OutputPaths  []string
}
