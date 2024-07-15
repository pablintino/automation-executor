package db

import (
	"github.com/google/uuid"
	"github.com/pablintino/automation-executor/internal/models"
	"github.com/pablintino/automation-executor/internal/test"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestITSecretsDbRegistrySecretBasicCrudOk(t *testing.T) {
	tb, err := test.NewTBDefault(t)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	dbConfig := tb.DB().GetDBConfig()
	db, err := NewSQLDatabase(&dbConfig)
	assert.NoError(t, err)
	if db == nil {
		t.Fatal("db is nil")
	}

	registrySecret := models.RegistrySecretModel{
		Registry: "registry.domain.local",
		SecretModel: models.SecretModel{
			Name:   "test-name",
			Key:    "test-key-value",
			Value:  "test-value-value",
			TypeId: models.SecretTypeIdRegistryTupleId,
		},
	}
	result, err := db.Secrets().SaveRegistrySecret(&registrySecret)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	if result != nil {
		assert.NotEqual(t, uuid.UUID{}, result.RegistrySecretId)
	}

	getRegistrySecretModel, err := db.Secrets().GetRegistrySecretByRegistry(registrySecret.Registry)
	assert.NoError(t, err)
	assert.NotNil(t, getRegistrySecretModel)
	if getRegistrySecretModel != nil {
		assert.Equal(t, registrySecret, *getRegistrySecretModel)
	}

	getSecretModel, err := db.Secrets().GetSecretByName(registrySecret.Name)
	assert.NotNil(t, getSecretModel)
	assert.NoError(t, err)
	if getSecretModel != nil {
		// Registry ID is not the same one as the backing ID
		assert.NotEqual(t, registrySecret.RegistrySecretId, getSecretModel.SecretId)
		assert.Equal(t, registrySecret.SecretModel, *getSecretModel)
	}

	existsResult, err := db.Secrets().Exists(registrySecret.Name)
	assert.NoError(t, err)
	assert.True(t, existsResult)

	deleteErr := db.Secrets().Delete(registrySecret.Name)
	assert.NoError(t, deleteErr)

	// The secret_registry row should be deleted on cascade, no explicit action needed
	afterDeleteExistsResult, err := db.Secrets().Exists(registrySecret.Name)
	assert.NoError(t, err)
	assert.False(t, afterDeleteExistsResult)
}

func TestITSecretsDbRegistryGenericSecretsCrudOk(t *testing.T) {
	tb, err := test.NewTBDefault(t)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	dbConfig := tb.DB().GetDBConfig()
	db, err := NewSQLDatabase(&dbConfig)
	assert.NoError(t, err)
	if db == nil {
		t.Fatal("db is nil")
	}

	tests := []struct {
		name   string
		secret models.SecretModel
	}{
		{
			name: "tuple-secret-basic",
			secret: models.SecretModel{
				Name:   "test-name-tuple",
				Key:    "test-key-value-tuple",
				TypeId: models.SecretTypeIdCredentialsTupleId,
			},
		},
		{
			name: "token-secret-basic",
			secret: models.SecretModel{
				Name:   "test-name-token",
				Key:    "test-key-value-token",
				TypeId: models.SecretTypeIdKeyTokenId,
			},
		},
	}
	for _, testData := range tests {
		t.Run(testData.name, func(t *testing.T) {

			result, err := db.Secrets().Save(&testData.secret)
			assert.NoError(t, err)
			assert.NotNil(t, result)
			if result != nil {
				assert.NotEqual(t, uuid.UUID{}, result.SecretId)
			}

			retrievedSecret, err := db.Secrets().GetSecretByName(testData.secret.Name)
			assert.NotNil(t, retrievedSecret)
			assert.NoError(t, err)
			if retrievedSecret != nil {
				assert.Equal(t, testData.secret, *retrievedSecret)
			}

			existsResult, err := db.Secrets().Exists(testData.secret.Name)
			assert.NoError(t, err)
			assert.True(t, existsResult)

			deleteErr := db.Secrets().Delete(testData.secret.Name)
			assert.NoError(t, deleteErr)

			afterDeleteExistsResult, err := db.Secrets().Exists(testData.secret.Name)
			assert.NoError(t, err)
			assert.False(t, afterDeleteExistsResult)
		})
	}
}
