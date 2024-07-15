package db

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/pablintino/automation-executor/internal/models"
)

const (
	secretsPgpEncryptValueFormat = "pgp_sym_encrypt(%s, '%s')"
	secretsDbColNameTypeId       = "type_id"
	secretsDbColNameName         = "name"
	secretsDbColNameSecretKey    = "secret_key"
	secretsDbColNameSecretValue  = "secret_value"
)

type sqlSecretsDb struct {
	db            *sqlx.DB
	encryptionKey string
}

func NewSqlSecretsDb(db *sqlx.DB, encryptionKey string) *sqlSecretsDb {
	return &sqlSecretsDb{db: db, encryptionKey: encryptionKey}
}

func (s *sqlSecretsDb) Save(secret *models.SecretModel) (*models.SecretModel, error) {
	var err error
	tx, err := s.db.Beginx()
	if err != nil {
		return nil, err
	}
	defer func() {
		if tmpErr := tx.Rollback(); tmpErr != nil {
			err = errors.Join(err, tmpErr)
		}
	}()
	if _, err = s.saveBaseSecret(tx, secret); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return secret, nil
}

func (s *sqlSecretsDb) SaveRegistrySecret(secret *models.RegistrySecretModel) (*models.RegistrySecretModel, error) {
	var err error
	tx, err := s.db.Beginx()
	if err != nil {
		return nil, err
	}
	defer func() {
		if tmpErr := tx.Rollback(); tmpErr != nil {
			err = errors.Join(err, tmpErr)
		}
	}()

	// Enforce the type ID
	secret.TypeId = models.SecretTypeIdRegistryTupleId
	if _, err := s.saveBaseSecret(tx, &secret.SecretModel); err != nil {
		return nil, err
	}

	query := "INSERT INTO secret_registry (type_id, secret_id, registry) VALUES ($1, $2, $3) RETURNING id"
	rows, err := tx.Query(query, secret.TypeId, secret.SecretId, secret.Registry)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, errors.New("could not insert registry secret")
	}

	if err = rows.Scan(&secret.RegistrySecretId); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return secret, nil
}

func (s *sqlSecretsDb) GetRegistrySecretByRegistry(registry string) (*models.RegistrySecretModel, error) {
	query := strings.Join([]string{
		"SELECT",
		NewSqlSelectColumnsBuilder().
			ForTable("sr").
			ColumnRenamed("id", "registry_secret_id").
			Column("registry").
			ForTable("s").
			Column(secretsDbColNameTypeId).
			ColumnRenamed("id", "secret_id").
			Column(secretsDbColNameName).
			ColumnPgpEnc(secretsDbColNameSecretKey, secretsDbColNameSecretKey, s.encryptionKey).
			ColumnPgpEnc("secret_value", "secret_value", s.encryptionKey).
			Build(),
		"FROM secret_registry sr INNER JOIN secret s ON sr.secret_id = s.id WHERE registry = $1",
	}, " ")
	rows, err := s.db.Queryx(query, registry)
	if err != nil || !rows.Next() {
		return nil, err
	}
	defer rows.Close()
	secret := &models.RegistrySecretModel{}
	if err := rows.StructScan(secret); err != nil {
		return nil, err
	}
	return secret, nil
}

func (s *sqlSecretsDb) Exists(name string) (bool, error) {
	var exists []bool
	if err := s.db.Select(&exists, "SELECT EXISTS(SELECT 1 FROM secret WHERE name=$1)", name); err != nil || len(exists) == 0 {
		return false, err
	}
	return exists[0], nil
}

func (s *sqlSecretsDb) Delete(name string) error {
	// Secret sub-type rows will be deleted by the cascade in each table
	_, err := s.db.Exec(fmt.Sprintf("DELETE FROM secret WHERE %s=$1", secretsDbColNameName), name)
	return err
}

func (s *sqlSecretsDb) GetSecretByName(name string) (*models.SecretModel, error) {
	query := strings.Join([]string{
		"SELECT",
		NewSqlSelectColumnsBuilder().
			ColumnRenamed("id", "secret_id").
			Column(secretsDbColNameTypeId).
			Column(secretsDbColNameName).
			ColumnPgpEnc(secretsDbColNameSecretKey, secretsDbColNameSecretKey, s.encryptionKey).
			ColumnPgpEnc(secretsDbColNameSecretValue, secretsDbColNameSecretValue, s.encryptionKey).
			Build(),
		"FROM secret WHERE name = $1",
	}, " ")

	rows, err := s.db.Queryx(query, name)
	if err != nil || !rows.Next() {
		return nil, err
	}
	defer rows.Close()
	secret := &models.SecretModel{}
	if err := rows.StructScan(secret); err != nil {
		return nil, err
	}
	return secret, nil
}

func (s *sqlSecretsDb) saveBaseSecret(tx *sqlx.Tx, secret *models.SecretModel) (*models.SecretModel, error) {
	cols := []string{secretsDbColNameName, secretsDbColNameTypeId, secretsDbColNameSecretKey}
	values := []string{
		":" + secretsDbColNameName,
		":" + secretsDbColNameTypeId,
		s.getPgpQueryValueEncrypt(":" + secretsDbColNameSecretKey),
	}
	if secret.Value != "" {
		cols = append(cols, secretsDbColNameSecretValue)
		values = append(values, s.getPgpQueryValueEncrypt(":"+secretsDbColNameSecretValue))
	}
	query := "INSERT INTO secret (" + strings.Join(cols, ",") +
		") VALUES (" + strings.Join(values, ",") + ") RETURNING id"
	rows, err := tx.NamedQuery(query, secret)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, errors.New("could not insert secret")
	}

	if err := rows.Scan(&secret.SecretId); err != nil {
		return nil, err
	}
	return secret, err
}

func (s *sqlSecretsDb) getPgpQueryValueEncrypt(field string) string {
	return fmt.Sprintf(secretsPgpEncryptValueFormat, field, s.encryptionKey)
}
