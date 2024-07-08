package container

import "github.com/pablintino/automation-executor/internal/models"

type FakeImageSecretResolver struct{}

func newFakeImageSecretResolver() *FakeImageSecretResolver {
	return &FakeImageSecretResolver{}
}

func (r *FakeImageSecretResolver) GetSecretByRegistry(registry string) (*models.RegistrySecretModel, error) {
	return nil, nil
}
