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
	secretsPgpDecryptValueFormat = "COALESCE(pgp_sym_decrypt(%s, '%s'), '')"
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
	rows, err := tx.Query(query, secret.TypeId, secret.Id, secret.Registry)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, errors.New("could not insert registry secret")
	}

	if err = rows.Scan(&secret.Id); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return secret, nil
}

func (s *sqlSecretsDb) GetRegistrySecretByRegistry(registry string) (*models.RegistrySecretModel, error) {
	query := "SELECT sr.id, sr.type_id, sr.registry, s.name, " +
		s.getPgpQueryValueDecrypt("secret_key") + "," + s.getPgpQueryValueDecrypt("secret_value") +
		" FROM secret_registry sr INNER JOIN secret s ON sr.secret_id = s.id WHERE registry = $1"
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

func (s *sqlSecretsDb) GetSecretByName(name string) (*models.SecretModel, error) {
	query := "SELECT id, type_id, name, " + s.getPgpQueryValueDecrypt("secret_key") +
		", " + s.getPgpQueryValueDecrypt("secret_value") + " FROM secret WHERE name = $1"
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
	cols := []string{"name", "type_id", "secret_key"}
	values := []string{":name", ":type_id", s.getPgpQueryValueEncrypt(":secret_key")}
	if secret.Value != "" {
		cols = append(cols, "secret_value")
		values = append(values, s.getPgpQueryValueEncrypt(":secret_value"))
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

	if err := rows.Scan(&secret.Id); err != nil {
		return nil, err
	}
	return secret, err
}

func (s *sqlSecretsDb) getPgpQueryValueEncrypt(field string) string {
	return fmt.Sprintf(secretsPgpEncryptValueFormat, field, s.encryptionKey)
}

func (s *sqlSecretsDb) getPgpQueryValueDecrypt(field string) string {
	return fmt.Sprintf(secretsPgpDecryptValueFormat, field, s.encryptionKey) + " as " + field
}
