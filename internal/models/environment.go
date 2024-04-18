package models

type EnvironmentModel struct {
	Id          string
	TaskId      string
	WorkspaceId string
}

type ContainerEnvironmentModel struct {
	EnvironmentModel
	ContainerId string
	SessionId   string
}
