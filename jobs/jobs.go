// Package jobs facilitates a method of parallelized workers like errgroup, but allows for circular pipelines
package jobs

import (
	"context"
	"errors"
	"sync"

	"github.com/lanrat/tasker/jobs/memory"
)

// Job interface for data that can be used for a job
type Job interface {
	memJob
}

type memJob interface {
	Type() string
	Key() string
	IsInput() bool
	Errors() []error
}

// Jobs is an orchestrator of all threads and parallelism
//
//nolint:containedctx // context allows for sub-jobs
type Jobs struct {
	jobChan  chan Job
	saveChan chan Job
	saveDone chan bool
	errChan  chan error
	wg       sync.WaitGroup
	saveWg   sync.WaitGroup
	jobCtx   context.Context
	ctx      context.Context
	cancel   context.CancelFunc
	memory   *memory.Mem
	Stats    JobStats
}

// Start creates a jobs instance
func Start(ctx context.Context, classes ...memory.ClassType) *Jobs {
	var j Jobs
	j.jobChan = make(chan Job, 1)
	j.saveChan = make(chan Job, 10)
	j.errChan = make(chan error)
	j.ctx = ctx
	j.jobCtx, j.cancel = context.WithCancel(j.ctx)
	j.memory = memory.Memory(classes...)
	// j.Stats = stats.New(j.memory) // TODO use SetStats()
	return &j
}

func (j *Jobs) SetStats(stats JobStats) {
	j.Stats = stats
}

// Go starts a new thread that consumes jobs
func (j *Jobs) Go(worker func(context.Context, chan Job) error) {
	if j.Stats != nil {
		j.Stats.AddWorkers(1)
	}
	go func() {
		err := worker(j.jobCtx, j.jobChan)
		if err != nil {
			// log.Printf("Go error: %s", err)
			if !errors.Is(err, context.Canceled) {
				j.Stats.AddError(1)
				// if multiple threads error and try to write at the same time
				// this can panic due to the first error closing the channel
				// for now, errChan is not closed to prevent this panic
				j.errChan <- err
			}
		}
		if j.Stats != nil {
			j.Stats.AddWorkers(-1)
		}
	}()
}

// Add add a work item
//
//revive:disable:cognitive-complexity // easy to understand
func (j *Jobs) Add(jobs ...Job) {
	j.wg.Add(len(jobs))
	if j.Stats != nil {
		j.Stats.AddJobQueueTotal(len(jobs))
		j.Stats.AddJobQueueSize(len(jobs))
	}
	go func() {
		for _, w := range jobs {
			// only allow each type of object to be added to the queue once
			if !j.memory.LookupAndSave(memory.ClassType(w.Type()), memory.KeyType(w.Key())) {
				j.jobChan <- w
				if j.Stats != nil {
					j.Stats.AddWork(1)
					j.Stats.AddWorkType(w.Type())
				}
			} else {
				// the waitgroup was already increased, need to decrease it now since we are skipping this record
				if w.IsInput() && j.Stats != nil {
					// if the input was already worked on, then we mark it as done since the prior record would would not have incremented the save counter
					j.Stats.AddSavedInputs(1)
				}
				j.wg.Done()
				if j.Stats != nil {
					j.Stats.AddCacheHits(1)
				}
			}
			if j.Stats != nil {
				j.Stats.AddJobQueueSize(-1)
			}
		}
	}()
}

// MemorySize returns the number of objects in memory
func (j *Jobs) MemorySize() int {
	return j.memory.Size()
}

// Done should be called by the worker threads when they finish a job
func (j *Jobs) Done(w Job) {
	j.Save(w)
	j.wg.Done()
}

// Save a work item without working on it, useful for metadata
func (j *Jobs) Save(w Job) {
	if j.Stats != nil {
		j.Stats.AddResults(1)
		if w.IsInput() {
			j.Stats.AddSavedInputs(1)
		}
	}
	// if w.Errors() != nil {
	// 	j.Stats.AddError(1)
	// }
	if j.Stats != nil {
		j.Stats.AddError(len(w.Errors()))
	}
	j.saveWg.Add(1)
	j.saveChan <- w
}

// Wait blocks until all jobs are done or there is an error
func (j *Jobs) Wait() error {
	wgDone := make(chan bool)

	// goroutine to wait until WaitGroup is done
	go func() {
		j.wg.Wait()
		j.cancel()
		close(j.saveChan) // no more entries to be saved
		j.saveWg.Wait()   // wait for saving to finish
		close(wgDone)
	}()

	// Wait until either WaitGroup is done or an error is received through the channel
	select {
	case <-wgDone:
		// carry on (break)
	case err := <-j.errChan:
		// possible bug here, when we call cancel, all workers will return and error (context canceled)
		// and once errChan is closed this will cause a panic.
		// this is fixed by having Go() workers not return the context.Canceled error
		j.cancel()
		// close(j.errChan) // don't close errChan to allow multiple errors to be sent without causing a panic
		return err
	}

	// wait for save worker to finish and check for error
	select {
	case <-j.saveDone:
		// carry on (break)
	case err := <-j.errChan:
		// all workers should have already finished by now, so no need to cancel, but calling just in case
		// if somehow a worker is canceled here, it will likely return an error and cause a panic when we close errChan
		j.cancel()
		// all workers should be done by now so safe to cancel
		close(j.errChan)
		return err
	}
	return nil
}

func (j *Jobs) SaveGo(saver func(context.Context, chan Job, *sync.WaitGroup) error) {
	j.saveDone = make(chan bool)
	go func() {
		err := saver(j.ctx, j.saveChan, &j.saveWg)
		if err != nil {
			// log.Printf("Go error: %s", err)
			if !errors.Is(err, context.Canceled) {
				// if multiple threads error and try to write at the same time
				// this can panic due to the first error closing the channel
				// for now, errChan is not closed to prevent this panic
				j.errChan <- err
				return // saveDone is not set or closed to ensure that the error is picked up
			}
		}
		// notify Wait() that saving is done
		j.saveDone <- true
		close(j.saveDone)
	}()
}
