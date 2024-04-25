package models

import "github.com/google/uuid"

type EnvironmentModel struct {
	Id     uuid.UUID
	TaskId uuid.UUID
}

type ContainerEnvironmentModel struct {
	Id            uuid.UUID
	EnvironmentId uuid.UUID
	ContainerId   uuid.UUID
}
