package db

import (
	"database/sql"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/pablintino/automation-executor/internal/models"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/pablintino/automation-executor/internal/config"
)

}

type SqlDatabase struct {
	db            *sqlx.DB
	config        *config.DatabaseConfig
	environmentDb *sqlEnvironmentDb
}

func NewSQLDatabase(config *config.DatabaseConfig) (*SqlDatabase, error) {
	db, err := connect(config)
	if err != nil {
		log.Fatalln(err)
		return nil, err
	}
	dbX := sqlx.NewDb(db, config.Driver)
	return &SqlDatabase{
		db:            dbX,
		environmentDb: newSqlEnvironmentDb(dbX),
		containersDb:  newSqlContainerDb(dbX),
		config:        config,
	}, nil
}

func (s *SqlDatabase) Containers() ContainerDb {
	return s.containersDb
}

func connect(config *config.DatabaseConfig) (*sql.DB, error) {
	db, err := sql.Open(config.Driver, config.DataSource)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}
