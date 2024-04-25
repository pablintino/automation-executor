package db

import (
	"github.com/jmoiron/sqlx"
)

type sqlEnvironmentDb struct {
	db *sqlx.DB
}

func newSqlEnvironmentDb(db *sqlx.DB) *sqlEnvironmentDb {
	return &sqlEnvironmentDb{db: db}
}
