package task

import (
	"sync"

	"github.com/pablintino/automation-executor/internal/models"
)

const (
	taskRunnerChanSize = 128
)

type taskRunnerWorkload struct {
	task *models.TaskModel
}

type taskRunner struct {
	taskChannel  chan *taskRunnerWorkload
	waitGroup    *sync.WaitGroup
	routineCount int
	tasks        map[string]*taskRunnerWorkload
}

func NewTaskRunner(count int) *taskRunner {
	return &taskRunner{
		waitGroup:    &sync.WaitGroup{},
		routineCount: count,
		taskChannel:  make(chan *taskRunnerWorkload, taskRunnerChanSize),
		tasks:        make(map[string]*taskRunnerWorkload),
	}
}

func (r *taskRunner) start() {
	for i := 0; i < r.routineCount; i++ {
		go func() {
			defer r.waitGroup.Done()
			for input := range r.taskChannel {
				r.routine(input)
			}
		}()
	}
}

func (r *taskRunner) stop() {
	close(r.taskChannel)
	r.waitGroup.Wait()
}

func (r *taskRunner) routine(workLoad *taskRunnerWorkload) {

}
