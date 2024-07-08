package db

import (
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/pablintino/automation-executor/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestSecretsDbGetRegistrySecretByRegistryOk(t *testing.T) {
	expectedModel := models.RegistrySecretModel{
		Registry: "registry.domain.local",
		SecretModel: models.SecretModel{
			Id:     uuid.New(),
			Value:  "unencrypted-value",
			Key:    "unencrypted-key",
			Name:   "test-secret-name",
			TypeId: models.SecretTypeIdRegistryTupleId,
		},
	}
	db := buildMockForGetRegistrySecretByRegistry(t, &expectedModel)
	defer db.Close()
	secretsDb := NewSqlSecretsDb(sqlx.NewDb(db, databaseDriverName), "test-key")
	model, err := secretsDb.GetRegistrySecretByRegistry(expectedModel.Registry)
	assert.NoError(t, err)
	if model != nil {
		assert.Equal(t, expectedModel, *model)
	}
}

func TestSecretsDbGetRegistrySecretByRegistryNoResultOk(t *testing.T) {
	db := buildMockForGetRegistrySecretByRegistry(t, nil)
	defer db.Close()
	secretsDb := NewSqlSecretsDb(sqlx.NewDb(db, databaseDriverName), "test-key")
	model, err := secretsDb.GetRegistrySecretByRegistry("non-existing")
	assert.NoError(t, err)
	assert.Nil(t, model)
}

func TestSecretsDbSaveTokenSecretOk(t *testing.T) {
	mock, db, secretsDb := buildMockedSecretsDb(t, sqlmock.QueryMatcherEqual, "test-key")
	defer db.Close()
	expectedModel := models.SecretModel{
		Id:     uuid.New(),
		Key:    "unencrypted-key",
		Name:   "test-secret-name",
		TypeId: models.SecretTypeIdKeyTokenId,
	}

	query := "INSERT INTO secret (name,type_id,secret_key) " +
		"VALUES ($1,$2,pgp_sym_encrypt($3, 'test-key')) RETURNING id"
	mock.ExpectBegin()
	mock.ExpectQuery(query).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(expectedModel.Id)).
		WithArgs(expectedModel.Name, expectedModel.TypeId, expectedModel.Key)
	mock.ExpectCommit()
	model, err := secretsDb.Save(&expectedModel)
	assert.NoError(t, err)
	assert.NotNil(t, model)
	if model != nil {
		assert.Equal(t, expectedModel, *model)
	}
}

func TestSecretsDbSaveTupleSecretOk(t *testing.T) {
	mock, db, secretsDb := buildMockedSecretsDb(t, sqlmock.QueryMatcherEqual, "test-key")
	defer db.Close()
	expectedModel := models.SecretModel{
		Id:     uuid.New(),
		Key:    "unencrypted-key",
		Value:  "unencrypted-value",
		Name:   "test-secret-name",
		TypeId: models.SecretTypeIdKeyTokenId,
	}

	query := "INSERT INTO secret (name,type_id,secret_key,secret_value) " +
		"VALUES ($1,$2,pgp_sym_encrypt($3, 'test-key'),pgp_sym_encrypt($4, 'test-key')) RETURNING id"
	mock.ExpectBegin()
	mock.ExpectQuery(query).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(expectedModel.Id)).
		WithArgs(expectedModel.Name, expectedModel.TypeId, expectedModel.Key, expectedModel.Value)
	mock.ExpectCommit()
	model, err := secretsDb.Save(&expectedModel)
	assert.NoError(t, err)
	assert.NotNil(t, model)
	if model != nil {
		assert.Equal(t, expectedModel, *model)
	}
}

func TestSecretsDbSaveRegistrySecretOk(t *testing.T) {
	mock, db, secretsDb := buildMockedSecretsDb(t, sqlmock.QueryMatcherEqual, "test-key")
	defer db.Close()
	expectedModel := models.RegistrySecretModel{
		Registry: "registry.domain.local",
		SecretModel: models.SecretModel{
			Id:     uuid.New(),
			Key:    "unencrypted-key",
			Value:  "unencrypted-value",
			Name:   "test-secret-name",
			TypeId: models.SecretTypeIdRegistryTupleId,
		},
	}
	secretId := uuid.New()
	query := "INSERT INTO secret (name,type_id,secret_key,secret_value) " +
		"VALUES ($1,$2,pgp_sym_encrypt($3, 'test-key'),pgp_sym_encrypt($4, 'test-key')) RETURNING id"
	mock.ExpectBegin()
	mock.ExpectQuery(query).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(secretId)).
		WithArgs(expectedModel.Name, expectedModel.TypeId, expectedModel.Key, expectedModel.Value)
	mock.ExpectQuery("INSERT INTO secret_registry (type_id, secret_id, registry) VALUES ($1, $2, $3) RETURNING id").
		WithArgs(models.SecretTypeIdRegistryTupleId, secretId, expectedModel.Registry).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(expectedModel.Id))
	mock.ExpectCommit()
	model, err := secretsDb.SaveRegistrySecret(&expectedModel)
	assert.NoError(t, err)
	assert.NotNil(t, model)
	if model != nil {
		assert.Equal(t, expectedModel, *model)
	}
}

func TestSecretsDbGetSecretByNameOk(t *testing.T) {
	mock, db, secretsDb := buildMockedSecretsDb(t, sqlmock.QueryMatcherEqual, "test-key")
	defer db.Close()
	expected := models.SecretModel{
		Id:     uuid.New(),
		Key:    "unencrypted-key",
		Value:  "unencrypted-value",
		Name:   "test-secret-name",
		TypeId: models.SecretTypeIdKeyTokenId,
	}

	query := "SELECT id, type_id, name, COALESCE(pgp_sym_decrypt(secret_key, 'test-key'), '') as secret_key," +
		" COALESCE(pgp_sym_decrypt(secret_value, 'test-key'), '') as secret_value FROM secret WHERE name = $1"
	mock.ExpectQuery(query).
		WillReturnRows(
			sqlmock.NewRows([]string{"id", "type_id", "name", "secret_key", "secret_value"}).
				AddRow(
					expected.Id, expected.TypeId, expected.Name, expected.Key, expected.Value),
		).
		WithArgs(expected.Name)
	model, err := secretsDb.GetSecretByName(expected.Name)
	assert.NoError(t, err)
	assert.NotNil(t, model)
	if model != nil {
		assert.Equal(t, expected, *model)
	}
}

func TestSecretsDbGetSecretByNameEmptyOk(t *testing.T) {
	mock, db, secretsDb := buildMockedSecretsDb(t, sqlmock.QueryMatcherEqual, "test-key")
	defer db.Close()
	query := "SELECT id, type_id, name, COALESCE(pgp_sym_decrypt(secret_key, 'test-key'), '') as secret_key," +
		" COALESCE(pgp_sym_decrypt(secret_value, 'test-key'), '') as secret_value FROM secret WHERE name = $1"
	testName := "test-name"
	mock.ExpectQuery(query).
		WillReturnRows(
			sqlmock.NewRows([]string{"id", "type_id", "name", "secret_key", "secret_value"}),
		).
		WithArgs(testName)
	model, err := secretsDb.GetSecretByName(testName)
	assert.NoError(t, err)
	assert.Nil(t, model)
}

func buildMockedSecretsDb(t *testing.T, queryMatcher sqlmock.QueryMatcher, encryptionKey string) (sqlmock.Sqlmock, *sql.DB, *sqlSecretsDb) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(queryMatcher))
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	secretsDb := NewSqlSecretsDb(sqlx.NewDb(db, databaseDriverName), encryptionKey)
	return mock, db, secretsDb
}

func buildMockForGetRegistrySecretByRegistry(t *testing.T, model *models.RegistrySecretModel) *sql.DB {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	expectedGetRegistrySecretByRegistryQuery :=
		"SELECT sr.id, sr.type_id, sr.registry, s.name, COALESCE(pgp_sym_decrypt(secret_key, 'test-key'), '') " +
			"as secret_key,COALESCE(pgp_sym_decrypt(secret_value, 'test-key'), '') as secret_value " +
			"FROM secret_registry sr INNER JOIN secret s ON sr.secret_id = s.id WHERE registry = $1"
	expectedCols := []string{"id", "type_id", "registry", "name", "secret_key", "secret_value"}
	mockQuery := mock.ExpectQuery(expectedGetRegistrySecretByRegistryQuery)
	if model != nil {
		mockQuery.WithArgs(model.Registry).
			WillReturnRows(
				sqlmock.NewRows([]string{"id", "type_id", "registry", "name", "secret_key", "secret_value"}).
					AddRow(
						model.Id, model.TypeId, model.Registry, model.Name, model.Key, model.Value),
			)
	} else {
		mockQuery.WithArgs(AnyString{}).WillReturnRows(sqlmock.NewRows(expectedCols))
	}
	return db
}
