package db

import (
	"github.com/jmoiron/sqlx"
	"github.com/pablintino/automation-executor/internal/models"
)

type SqlTasksDb struct {
	db *sqlx.DB
}

func NewSqlTasksDb(db *sqlx.DB) *SqlTasksDb {
	return &SqlTasksDb{db: db}
}

func (s *SqlTasksDb) GetAll() ([]models.TaskModel, error) {
	query := "SELECT id, name, type_id FROM task"
	var tableRec []models.TaskModel
	err := s.db.Select(&tableRec, query)
	if err != nil {
		return nil, err
	}
	return tableRec, err
}

func (s *SqlTasksDb) GetAllAnsible() ([]models.AnsibleTaskModel, error) {
	const query = `
	SELECT
		at.id, at.task_id, at.playbook_vars,
		at.playbook_out_patterns, at.playbook, t.name
	FROM
		task_ansible at
	JOIN task t
	ON
		at.task_id = t.id
	`
	var tableRec []models.AnsibleTaskModel
	err := s.db.Select(&tableRec, query)
	if err != nil {
		return nil, err
	}
	return tableRec, err
}
