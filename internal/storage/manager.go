package storage

import (
	"os"
	"path"

	"github.com/google/uuid"
	"github.com/pablintino/automation-executor/internal/config"
)

const containersWorkspaceDir = "containers"

type storageManagerImp struct {
	storageConfig  *config.StorageConfig
	workspacesPath string
}

type WorkspaceImpl struct {
	location      string
	environmentId uuid.UUID
}

type Workspace interface {
	Location() string
	CreateContainerWorkspace(containerId uuid.UUID) (string, error)
}

type Manager interface {
	CreateWorkspace(environmentId uuid.UUID) (Workspace, error)
}

func NewStorageManager(storageConfig *config.StorageConfig) (*storageManagerImp, error) {
	if err := os.MkdirAll(storageConfig.WorkspacesPath, 0755); err != nil {
		return nil, err
	}
	return &storageManagerImp{storageConfig: storageConfig, workspacesPath: storageConfig.WorkspacesPath}, nil

}

func (m *storageManagerImp) CreateWorkspace(environmentId uuid.UUID) (Workspace, error) {
	workspacePath := path.Join(m.workspacesPath, environmentId.String())
	if err := os.MkdirAll(workspacePath, 0755); err != nil {
		return nil, err
	}
	return &WorkspaceImpl{environmentId: environmentId, location: workspacePath}, nil
}

func (m *WorkspaceImpl) Location() string {
	return m.location
}

func (m *WorkspaceImpl) CreateContainerWorkspace(containerId uuid.UUID) (string, error) {
	path := path.Join(m.location, containersWorkspaceDir, containerId.String())
	if err := os.MkdirAll(path, 0755); err != nil {
		return "", nil
	}
	return path, nil
}
