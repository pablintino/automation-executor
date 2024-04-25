package utils

import (
	"github.com/pablintino/automation-executor/logging"
	"sync"
	"time"
)

type RetryRoutine struct {
	period    time.Duration
	timer     *time.Ticker
	closing   chan bool
	fn        func() (bool, error)
	fnDone    func(bool, error)
	fnErrComp func(error) bool
	counter   uint64
	retries   uint64
	mtx       sync.Mutex
	wg        sync.WaitGroup
	started   bool
}

func NewRetryRoutine(
	periodMs uint64,
	retries uint64,
	fn func() (bool, error),
	fnDone func(bool, error),
	fnErrComp func(error) bool,
) *RetryRoutine {
	return &RetryRoutine{
		closing:   make(chan bool),
		period:    time.Duration(periodMs) * time.Millisecond,
		fn:        fn,
		fnDone:    fnDone,
		fnErrComp: fnErrComp,
		counter:   0,
		retries:   retries,
	}
}
func (s *RetryRoutine) routine() {
loop:
	for {
		select {
		case _, ok := <-s.timer.C:
			if !ok {
				break loop
			}
			succeed, err := s.fn()
			s.counter++

			if succeed && (err == nil || (s.fnErrComp != nil && !s.fnErrComp(err))) {
				if s.fnDone != nil {
					s.fnDone(true, nil)
				}
				break loop
			}
			if s.counter >= s.retries {
				logging.Logger.Debug("Retry routine ending exhausting retries", "retries", s.retries)
				if s.fnDone != nil {
					s.fnDone(false, err)
				}
				break loop
			}

		case <-s.closing:
			break loop
		}
	}
	logging.Logger.Debug("Retry routine finished")
	s.wg.Done()
}

func (s *RetryRoutine) Start() {
	s.mtx.Lock()
	if !s.started {
		s.wg.Add(1)
		s.started = true
		s.timer = time.NewTicker(s.period)
		go s.routine()
	}
	s.mtx.Unlock()
}

func (s *RetryRoutine) Destroy() {
	s.mtx.Lock()
	s.timer.Stop()
	// DO not clean the started flag
	// the logic implemented by the RetryRoutine
	// is designed to run the go routine once
	s.closing <- true
	s.wg.Wait()
	s.mtx.Unlock()

}
