package secrets

import (
	"fmt"
	"github.com/pablintino/automation-executor/internal/db"
	"github.com/pablintino/automation-executor/internal/models"
	"go.uber.org/zap"
)

type SecretsService struct {
	secretsDb db.SecretsDb
	logger    *zap.SugaredLogger
}

func NewSecretsService(secretsDb db.SecretsDb, logger *zap.SugaredLogger) SecretsService {
	return SecretsService{
		secretsDb: secretsDb,
		logger:    logger,
	}
}

func (s *SecretsService) AddSecretCredentialsTuple(name string, user string, credential string) (*models.SecretModel, error) {
	return s.createSecret(
		&models.SecretModel{
			Name:   name,
			Key:    user,
			Value:  credential,
			TypeId: models.SecretTypeIdCredentialsTupleId,
		},
	)
}

func (s *SecretsService) AddSecretToken(name string, token string) (*models.SecretModel, error) {
	return s.createSecret(
		&models.SecretModel{
			Name:   name,
			Key:    token,
			TypeId: models.SecretTypeIdKeyTokenId,
		},
	)
}
func (s *SecretsService) GetSecretByRegistry(registry string) (*models.RegistrySecretModel, error) {
	model, err := s.secretsDb.GetRegistrySecretByRegistry(registry)
	if err != nil {
		return nil, err
	}
	return model, nil
}

func (s *SecretsService) GetSecretByName(name string) (*models.SecretModel, error) {
	model, err := s.secretsDb.GetSecretByName(name)
	if err != nil {
		return nil, err
	}
	return model, nil
}

func (s *SecretsService) createSecret(model *models.SecretModel) (*models.SecretModel, error) {
	exists, err := s.secretsDb.Exists(model.Name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("secret %s already exists", model.Name)
	}
	return s.secretsDb.Save(model)
}
