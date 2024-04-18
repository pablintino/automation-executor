package task

import (
	"github.com/pablintino/automation-executor/internal/models"
)

type TaskService interface {
	CreateTask(task *models.TaskModel) error
}

type TaskServiceImpl struct {
}

func (t *TaskServiceImpl) CreateTask(task *models.TaskModel) error {

	return nil
}
