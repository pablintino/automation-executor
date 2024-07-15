package db

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/pablintino/automation-executor/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestSecretsDbSaveRegistrySecretEnsureEncryptionOk(t *testing.T) {
	mock, db, secretsDb := buildMockedSecretsDb(t,
		NewSqlMockFifoQueryMatcher(
			func(query string) error {
				if !strings.Contains(query, "pgp_sym_encrypt($3, 'test-key')") || !strings.Contains(query, "pgp_sym_encrypt($4, 'test-key')") {
					return fmt.Errorf("expected to find pgp_sym_encrypt for key and value cols")
				}
				return nil
			},
			func(query string) error {
				if strings.Index(query, "insert into secret_registry") != 0 {
					return fmt.Errorf("expected insert into secret_registry not found")
				}
				return nil
			},
		),
		"test-key")
	defer db.Close()
	expectedModel := models.RegistrySecretModel{
		Registry: "registry.domain.local",
		SecretModel: models.SecretModel{
			SecretId: uuid.New(),
			Key:      "unencrypted-key",
			Value:    "unencrypted-value",
			Name:     "test-secret-name",
			TypeId:   models.SecretTypeIdRegistryTupleId,
		},
	}
	secretId := uuid.New()
	mock.ExpectBegin()
	mock.ExpectQuery("<unused>").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(secretId)).
		WithArgs(expectedModel.Name, expectedModel.TypeId, expectedModel.Key, expectedModel.Value)
	mock.ExpectQuery("<unused>").
		WithArgs(models.SecretTypeIdRegistryTupleId, secretId, expectedModel.Registry).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(expectedModel.RegistrySecretId))
	mock.ExpectCommit()
	model, err := secretsDb.SaveRegistrySecret(&expectedModel)
	assert.NoError(t, err)
	assert.NotNil(t, model)
	if model != nil {
		assert.Equal(t, expectedModel, *model)
	}
}

func buildMockedSecretsDb(t *testing.T, queryMatcher sqlmock.QueryMatcher, encryptionKey string) (sqlmock.Sqlmock, *sql.DB, *sqlSecretsDb) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(queryMatcher))
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	secretsDb := NewSqlSecretsDb(sqlx.NewDb(db, databaseDriverName), encryptionKey)
	return mock, db, secretsDb
}
