package jobworker

import (
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

type JobStatus string

const (
	JobStatusPending   JobStatus = "PENDING"
	JobStatusRunning   JobStatus = "RUNNING"
	JobStatusCompleted JobStatus = "COMPLETED"
	JobStatusFailed    JobStatus = "FAILED"
	JobStatusStopped   JobStatus = "STOPPED"
	JobStatusKilled    JobStatus = "KILLED"
)

type Executor struct {
	Command      string
	Args         []string
	cmd          *exec.Cmd
	outputLogger *logger
	errorLogger  *logger
	rm           *ResourceManager
	ctx          context.Context
	cancel       context.CancelFunc
	started      time.Time
	stopped      time.Time
	mu           sync.Mutex
	status       JobStatus
	exitCode     int
	errorString  string
}

func NewExecutor(command string, args []string, limits ResourceLimits) (*Executor, string, error) {
	ctx, cancel := context.WithCancel(context.Background())
	rm, err := newResourceManager()
	if err != nil {
		cancel()
		return nil, "", fmt.Errorf("failed to create resource manager: %w", err)
	}

	if err := rm.setLimits(limits); err != nil {
		cancel()
		return nil, "", fmt.Errorf("failed to create resource manager: %w", err)
	}
	log.Println("Got Job: ")
	log.Println("command: ", command)
	log.Println("args: ", args)

	return &Executor{
		Command:      command,
		Args:         args,
		outputLogger: newLogger(),
		errorLogger:  newLogger(),
		rm:           rm,
		ctx:          ctx,
		cancel:       cancel,
		status:       JobStatusPending,
	}, rm.jobID, nil
}

func (e *Executor) Start() (err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	err = nil

	// Ensure cleanup if an error occurs
	defer func() {
		if err != nil {
			e.cancel()
			e.rm.Cleanup()
		}
	}()

	if e.status != JobStatusPending {
		err = fmt.Errorf("job is already started or completed")
		return
	}

	e.cmd = exec.CommandContext(e.ctx, e.Command, e.Args...)
	e.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	e.cmd.Stdout = e.outputLogger
	e.cmd.Stderr = e.errorLogger

	if err = e.cmd.Start(); err != nil {
		err = fmt.Errorf("failed to start command: %w", err)
		return
	}

	if err = e.rm.addProcess(e.cmd.Process.Pid); err != nil {
		err = fmt.Errorf("error adding process to cgroup: %v", err)
		return
	}

	e.started = time.Now()
	e.status = JobStatusRunning

	go func() {
		waitErr := e.cmd.Wait()

		e.mu.Lock()
		defer e.mu.Unlock()

		e.stopped = time.Now()
		if waitErr != nil {
			if exitErr, ok := waitErr.(*exec.ExitError); ok {
				e.exitCode = exitErr.ExitCode()
			}
			if e.ctx.Err() == context.Canceled {
				e.status = JobStatusStopped
				e.errorString = fmt.Sprintf("job cancelled: %s (exit code: %d)", e.errorString, e.exitCode)
			} else {
				e.status = JobStatusFailed
				e.errorString = fmt.Sprintf("job failed: %s (exit code: %d)", e.errorString, e.exitCode)
			}
		}
		// any other state is already handled
		if e.status == JobStatusRunning {
			e.status = JobStatusCompleted
			e.exitCode = 0
		}

		log.Println(e.rm.jobID, "Exited")
		e.outputLogger.Close()
		e.errorLogger.Close()
		log.Println(e.rm.jobID, "loggers closed")
		e.rm.Cleanup()
	}()

	return nil
}

func (e *Executor) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	defer e.cancel()

	if e.status != JobStatusRunning {
		return fmt.Errorf("job is not running")
	}

	e.status = JobStatusStopped
	e.errorString = "Stopped"
	log.Println("Stopping Job : ", e.rm.jobID)
	log.Println("Sending SIGTERM")
	// Send SIGTERM first
	if err := e.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM: %w", err)
	}

	// Wait for a short period to allow graceful shutdown
	select {
	case <-time.After(5 * time.Second):
		// If the process hasn't exited, force kill it
		log.Println("Sending SIGKILL")
		e.status = JobStatusKilled
		e.errorString = "Killed"
		if err := e.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process: %w", err)
		}
	case <-e.ctx.Done():
		// Process has exited
	}

	e.stopped = time.Now()
	return nil
}

func (e *Executor) GetStatus() JobStatus {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.status
}

func (e *Executor) GetOutputReader() io.Reader {
	return newLogReader(e.outputLogger)
}

func (e *Executor) GetErrorReader() io.Reader {
	return newLogReader(e.errorLogger)
}

func (e *Executor) GetStartTime() time.Time {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.started
}

func (e *Executor) GetStopTime() time.Time {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.stopped
}

func (e *Executor) GetExitCode() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.exitCode
}

func (e *Executor) GetErrorString() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.errorString
}

func (e *Executor) Wait() error {
	<-e.ctx.Done()
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.status == JobStatusFailed {
		return fmt.Errorf("job failed: %s (exit code: %d)", e.errorString, e.exitCode)
	}
	return nil
}
