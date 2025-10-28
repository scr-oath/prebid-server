package task

import (
	"sync"
	"time"

	"github.com/golang/glog"
)

type Runner interface {
	Run() error
}

// RunnerFunc is a function adapter that allows ordinary functions to be used as
// Runners. If f is a function with the appropriate signature, RunnerFunc(f) is a
// Runner that calls f.
//
// Example usage:
//
//	task := NewTickerTask(5*time.Second, RunnerFunc(func() error {
//	    fmt.Println("Running periodic task")
//	    return nil
//	}))
//	task.Start()
type RunnerFunc func() error

// Run implements the Runner interface by calling the function itself.
func (r RunnerFunc) Run() error {
	return r()
}

// Compile-time assertion that RunnerFunc implements Runner
var _ Runner = RunnerFunc(nil)

// TickerTask executes a Runner implementation immediately and then periodically
// at a specified interval. It supports graceful shutdown via the Stop method.
type TickerTask struct {
	interval time.Duration
	runner   Runner
	done     chan struct{}
	wg       sync.WaitGroup
}

func NewTickerTask(interval time.Duration, runner Runner) *TickerTask {
	return &TickerTask{
		interval: interval,
		runner:   runner,
		done:     make(chan struct{}),
	}
}

// Start runs the task immediately and then schedules the task to run periodically
// if a positive fetching interval has been specified. Errors from the runner are
// logged but do not stop execution.
func (t *TickerTask) Start() {
	if err := t.runner.Run(); err != nil {
		glog.Errorf("[TickerTask] Initial task execution failed: %v", err)
	}

	if t.interval > 0 {
		t.wg.Add(1)
		go t.runRecurring()
	}
}

// Stop stops the periodic task and waits for it to complete. The task runner
// maintains state. Stop is idempotent and safe to call multiple times.
func (t *TickerTask) Stop() {
	select {
	case <-t.done:
		return // already stopped
	default:
		close(t.done)
	}
	t.wg.Wait() // Wait for goroutine to finish
	glog.Info("[TickerTask] Stopped")
}

// runRecurring creates a ticker that ticks at the specified interval. On each tick,
// the task is executed until the done channel is closed.
func (t *TickerTask) runRecurring() {
	defer t.wg.Done()
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop() // Ensure ticker is stopped to prevent resource leak

	for {
		select {
		case <-ticker.C:
			if err := t.runner.Run(); err != nil {
				glog.Errorf("[TickerTask] Periodic task execution failed: %v", err)
			}
		case <-t.done:
			glog.Info("[TickerTask] Stopping periodic task")
			return
		}
	}
}
