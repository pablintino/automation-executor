package db

import (
	"database/sql"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/pablintino/automation-executor/internal/config"
	"github.com/pablintino/automation-executor/internal/models"
	"log"
)

const (
	databaseDriverName = "postgres"
)

type SqlDatabase struct {
	db            *sqlx.DB
	config        *config.DatabaseConfig
	environmentDb *sqlEnvironmentDb
	secretsDb     *sqlSecretsDb
}

func NewSQLDatabase(config *config.DatabaseConfig) (*SqlDatabase, error) {
	db, err := connect(config)
	if err != nil {
		log.Fatalln(err)
		return nil, err
	}
	dbX := sqlx.NewDb(db, databaseDriverName)
	return &SqlDatabase{
		db:            dbX,
		environmentDb: newSqlEnvironmentDb(dbX),
		secretsDb:     NewSqlSecretsDb(dbX, config.EncryptionKey),
		config:        config,
	}, nil
}

type SecretsDb interface {
	Save(secret *models.SecretModel) (*models.SecretModel, error)
	Exists(name string) (bool, error)
	SaveRegistrySecret(secret *models.RegistrySecretModel) (*models.RegistrySecretModel, error)
	GetRegistrySecretByRegistry(registry string) (*models.RegistrySecretModel, error)
	GetSecretByName(name string) (*models.SecretModel, error)
}

func (s *SqlDatabase) Secrets() SecretsDb {
	return s.secretsDb
}
func connect(config *config.DatabaseConfig) (*sql.DB, error) {
	db, err := sql.Open(databaseDriverName, config.DataSource)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}
