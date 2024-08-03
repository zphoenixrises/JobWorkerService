package jobworker

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"
)

const (
	smallBufferSize = 4 * 1024 // 4 KB buffer
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
	rm, err := NewResourceManager()
	if err != nil {
		cancel()
		return nil, "", fmt.Errorf("failed to create resource manager: %w", err)
	}

	if err := rm.SetLimits(limits); err != nil {
		cancel()
		return nil, "", fmt.Errorf("failed to create resource manager: %w", err)
	}

	return &Executor{
		Command:      command,
		Args:         args,
		outputLogger: NewLogger(),
		errorLogger:  NewLogger(),
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

	cgroupDir, err := os.Open(e.rm.path)
	if err != nil {
		err = fmt.Errorf("failed to open cgroup directory: %w", err)
		return
	}
	defer cgroupDir.Close()

	// Get the file descriptor for the cgroup directory
	// cgroupFd := cgroupDir.Fd()

	e.cmd = exec.CommandContext(e.ctx, e.Command, e.Args...)
	e.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		// UseCgroupFD: true,
		// CgroupFD:    int(cgroupFd),
		// Cloneflags:  syscall.CLONE_NEWNS | syscall.CLONE_NEWPID,
	}

	stdout, err := e.cmd.StdoutPipe()
	if err != nil {
		err = fmt.Errorf("failed to create stdout pipe: %w", err)
		return
	}

	stderr, err := e.cmd.StderrPipe()
	if err != nil {
		err = fmt.Errorf("failed to create stderr pipe: %w", err)
		return
	}

	if err = e.cmd.Start(); err != nil {
		err = fmt.Errorf("failed to start command: %w", err)
		return
	}

	pid := e.cmd.Process.Pid
	if err = os.WriteFile(filepath.Join(e.rm.path, "cgroup.procs"), []byte(strconv.Itoa(pid)), 0644); err != nil {
		err = fmt.Errorf("error adding process to cgroup: %v", err)
		return
	}

	e.started = time.Now()
	e.status = JobStatusRunning

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		e.handleOutput(stdout, e.outputLogger, "stdout")
	}()

	go func() {
		defer wg.Done()
		e.handleOutput(stderr, e.errorLogger, "stderr")
	}()

	go func() {
		wg.Wait()
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
			} else {
				e.status = JobStatusFailed
				e.errorString = waitErr.Error()
			}
		} else {
			e.status = JobStatusCompleted
			e.exitCode = 0
		}

		e.outputLogger.Close()
		e.errorLogger.Close()
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
	return NewLogReader(e.outputLogger)
}

func (e *Executor) GetErrorReader() io.Reader {
	return NewLogReader(e.errorLogger)
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

func (e *Executor) handleOutput(r io.Reader, logger *logger, sourceType string) {
	buffer := make([]byte, smallBufferSize)
	for {
		n, err := r.Read(buffer)
		if n > 0 {
			_, writeErr := logger.Write(buffer[:n])
			if writeErr != nil {
				e.mu.Lock()
				e.status = JobStatusFailed
				e.errorString = fmt.Sprintf("%s write error: %v", sourceType, writeErr)
				e.cancel()
				e.mu.Unlock()
				return
			}
		}
		if err != nil {
			if err != io.EOF {
				e.mu.Lock()
				e.status = JobStatusFailed
				e.errorString = fmt.Sprintf("%s read error: %v", sourceType, err)
				e.cancel()
				e.mu.Unlock()
			}
			return
		}
	}
}
