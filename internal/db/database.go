package db

import (
	"database/sql"
	"log"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/pablintino/automation-executor/internal/config"
	"github.com/pablintino/automation-executor/internal/models"
)

type TasksDb interface {
	GetAll() ([]models.TaskModel, error)
	GetAllAnsible() ([]models.AnsibleTaskModel, error)
}

type Database interface {
	Tasks() TasksDb
}

type SqlDatabase struct {
	db      *sqlx.DB
	config  *config.DatabaseConfig
	tasksDb *SqlTasksDb
}

func NewSQLDatabase(config *config.DatabaseConfig) (*SqlDatabase, error) {
	db, err := connect(config)
	if err != nil {
		log.Fatalln(err)
		return nil, err
	}
	dbX := sqlx.NewDb(db, config.Driver)
	return &SqlDatabase{
		db:      dbX,
		tasksDb: NewSqlTasksDb(dbX),
		config:  config,
	}, nil
}

func (s *SqlDatabase) Tasks() TasksDb {
	return s.tasksDb
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
