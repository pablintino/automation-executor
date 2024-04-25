package db

import (
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/pablintino/automation-executor/internal/models"
)

type sqlContainerDb struct {
	db *sqlx.DB
}

func newSqlContainerDb(db *sqlx.DB) *sqlContainerDb {
	return &sqlContainerDb{db: db}
}

type CreateExecTxDeferred struct {
	tx              *sqlx.Tx
	ContainerExecId uuid.UUID
	ContainerId     uuid.UUID
}

type CreateContainerTxDeferred struct {
	tx          *sqlx.Tx
	ContainerId uuid.UUID
	typeId      models.ContainerType
}

func (c *CreateExecTxDeferred) Save(execId string) (*models.ContainerExecModel, error) {
	defer c.tx.Rollback()
	const query = `
	UPDATE
		container_exec
	SET
		session_id=$1
	WHERE
		id=$2
	`
	_, err := c.tx.Exec(query, execId, c.ContainerExecId)
	if err != nil {
		return nil, err
	}
	if err := c.tx.Commit(); err != nil {
		return nil, err
	}
	return &models.ContainerExecModel{Id: c.ContainerExecId, ContainerId: c.ContainerId, SessionId: execId}, nil
}

func (c *CreateContainerTxDeferred) Save(runtimeId string) (*models.ContainerModel, error) {
	defer c.tx.Rollback()
	const query = `
	UPDATE
		container
	SET
		runtime_id=$1
	WHERE
		id=$2
	`
	_, err := c.tx.Exec(query, runtimeId, c.ContainerId)
	if err != nil {
		return nil, err
	}
	if err := c.tx.Commit(); err != nil {
		return nil, err
	}
	return &models.ContainerModel{Id: c.ContainerId, RuntimeId: runtimeId, TypeId: c.typeId}, nil
}

func (c *CreateExecTxDeferred) Cancel() error {
	return c.tx.Rollback()
}

func (c *CreateContainerTxDeferred) Cancel() error {
	return c.tx.Rollback()
}

func (s *sqlContainerDb) CreateContainerExecTxDeferred(containerId uuid.UUID) (*CreateExecTxDeferred, error) {
	tx := s.db.MustBegin()

	var err error
	var id uuid.UUID
	const query = `
	INSERT INTO
		container_exec (container_id)
	VALUES ($1)
	RETURNING id
	`
	insertResult, err := tx.Query(query, containerId.String())
	if err != nil {
		goto ROLLBACK
	}
	if !insertResult.Next() {
		goto ROLLBACK
	}

	if err = insertResult.Scan(&id); err != nil {
		goto ROLLBACK
	}
	if err = insertResult.Close(); err != nil {
		goto ROLLBACK
	}
	return &CreateExecTxDeferred{tx: tx, ContainerExecId: id, ContainerId: containerId}, nil

ROLLBACK:
	if rErr := tx.Rollback(); rErr != nil {
		return nil, rErr
	}
	return nil, err
}

func (s *sqlContainerDb) CreateContainerTxDeferred(containerType models.ContainerType) (*CreateContainerTxDeferred, error) {
	tx := s.db.MustBegin()

	var err error
	var id uuid.UUID
	const query = `
	INSERT INTO
		container (type_id)
	VALUES ($1)
	RETURNING id
	`
	insertResult, err := tx.Query(query, containerType)
	if err != nil {
		goto ROLLBACK
	}
	if !insertResult.Next() {
		goto ROLLBACK
	}

	if err = insertResult.Scan(&id); err != nil {
		goto ROLLBACK
	}
	if err = insertResult.Close(); err != nil {
		goto ROLLBACK
	}
	return &CreateContainerTxDeferred{tx: tx, ContainerId: id, typeId: containerType}, nil

ROLLBACK:
	if rErr := tx.Rollback(); rErr != nil {
		return nil, rErr
	}
	return nil, err
}
