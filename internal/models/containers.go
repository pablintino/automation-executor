package models

import "github.com/google/uuid"

const ContainerTypePodman = 1

type ContainerType int

type ContainerModel struct {
	Id        uuid.UUID     `db:"id"`
	TypeId    ContainerType `db:"type_id"`
	RuntimeId string        `db:"runtime_id"`
}

type ContainerExecModel struct {
	Id          uuid.UUID `db:"id"`
	ContainerId uuid.UUID `db:"container_id"`
	SessionId   string    `db:"session_id"`
}
