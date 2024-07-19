---
authors: Zeeshan Haque (@zphoenixrises)
state: review
---

# RFD 0001 - Job Worker Service

## What

This RFD proposes a prototype job worker service that provides a secure API to run arbitrary Linux processes with fine-grained resource control and real-time output streaming. The service aims to demonstrate a scalable and secure approach to process execution in a multi-tenant environment.

## Why

To showcase a secure and efficient approach to running and managing arbitrary processes in a controlled environment.

## Design Overview

### Components

1. **Library**: A reusable job management library
2. **API**: A gRPC API server
3. **Client**: A command-line interface (CLI) client

### Key Features

- Job management (start, stop, query status, get output)
- Real-time output streaming with support for multiple concurrent clients
- Resource control using cgroups (CPU, Memory, Disk I/O)
- Secure communication using mTLS
- Simple role-based authorization scheme
- gRPC API for client-server communication

#### Key Considerations 
- This design is not a distributed system and is only optimized to run on a single linux OS
- If needed, the components of the library can be split into a distributed microservices framework inthe future
- Rudimentary monitoring of total resource usage will be available through the the CLI if needed.

## Detailed Design

### 1. Library

#### a. In-memory Job Logger
- Implement a thread-safe circular buffer to store logs with a configurable maximum size
- Use a read-write mutex to ensure thread-safety for concurrent read/write operations
- Implement a pub/sub pattern for real-time log streaming:
  - Use channels for each subscriber
  - Implement a fan-out pattern to broadcast new log entries to all subscribers
- Provide methods for adding logs, retrieving all logs, and subscribing to log streams
- Implement log rotation to handle long-running jobs and prevent memory exhaustion

```go
// Logger represents an in-memory circular buffer for storing job logs.
type Logger struct {
	mu          sync.RWMutex
	logs        []string
	maxSize     int
	subscribers map[chan string]struct{}
}

// InitLogger creates a new Logger with the specified maximum size.
func InitLogger(maxSize int) *Logger

// Log adds a new log entry to the logger.
func (l *Logger) Log(log string)

// Retrieves all logs currently stored in the logger.
func (l *Logger) GetLogs() []string

// Returns a channel that receives new log entries.
func (l *Logger) Subscribe() <-chan string

// Removes a subscriber from the logger.
func (l *Logger) Unsubscribe(ch <-chan string)

// Closes all subscriber channels and clears the subscriber list.
func (l *Logger) Close()
```

#### b. Job Executor
- Use `os/exec` package to create and manage child processes
- Implement non-blocking I/O for stdout/stderr capture using `io.Pipe()`
- Use a separate goroutine for each of stdout and stderr to prevent blocking
- Implement a context-based cancellation mechanism for graceful job termination
- Monitor resource usage using cgroups
- Provide callbacks for resource usage updates
- Use a Go channel to periodically read current resource usage (memory, I/O, and CPU usage) from the associated cgroup

```go
// Executor is responsible for running jobs and managing their lifecycle.
type Executor struct {
	cmd     *exec.Cmd
	logger  *logger.Logger
    // Context to handle propagate state and cancellations
	ctx     context.Context
	cancel  context.CancelFunc
	started time.Time
	stopped time.Time
	mu      sync.Mutex
}

// NewExecutor creates a new Executor for the given command and arguments.
func NewExecutor(command string, args []string, logger *Logger) *Executor

// Start begins the execution of the job.
func (e *Executor) Start() error

// Stop terminates the running job.
func (e *Executor) Stop() error

// Wait blocks until the job is completed and returns its exit status.
func (e *Executor) Wait() error
```

#### c. Job Manager
- Manage job lifecycle (pending, running, completed)
- Maintain job queue and schedule based on resource availability
- Generate unique job IDs using UUIDs
- Utilize Logger, Executor, ResourceMonitor, and ResourceManager to provide a unified interface for job management

```go
// JobManager handles the scheduling and management of jobs.

type JobInfo struct {
	ID        string
	UserID    string
	Command   string
	Args      []string
	StartTime time.Time
	Executor  *executor.Executor
}

type JobManager struct {
	mu   sync.RWMutex
	jobs map[string]*JobInfo
}

// NewJobManager creates a new JobManager.
func NewJobManager() *JobManager

// CreateJob creates a new job with the given parameters and returns the uuid 
func (m *JobManager) CreateJob(userID, command string, args []string, exec *executor.Executor) (string, error)

// GetJob retrieves a job by its ID.
func (m *JobManager) GetJob(id string) (*JobInfo, bool)

// Retrieve all jobs for a given user
func (m *JobManager) GetJobsByUser(userID string) []*JobInfo

// StopJob stops a running job.
func (m *JobManager) StopJob(id string) error
```

#### d. Resource Manager
- Create a hierarchical cgroup structure:
  ```
  /jobworker
  ├── user1
  │   ├── job1
  │   └── job2
  └── user2
      └── job3
  ```
- Create and manage cgroups for jobs
- Set and enforce resource limits
- Implement resource allocation strategies:
  - Use CPU shares for fair CPU allocation
  - Set memory limits with both soft and hard limits
  - Configure I/O weight for disk I/O prioritization
- Create a parent cgroup during initialization to monitor resource usage for all jobs
- Create a cgroup for each user under the parent cgroup
- Execute each job as a child of the user cgroup
```go
// ResourceManager monitors and controls resource usage for jobs.

// ResourceLimits defines the resource constraints for a job.
type ResourceLimits struct {
	CPUShares     uint64 // CPU shares (relative weight)
	MemoryLimit   int64  // Memory limit in bytes
	IOReadBPS     uint64 // I/O read rate limit in bytes per second
	IOWriteBPS    uint64 // I/O write rate limit in bytes per second
	IOReadIOPS    uint64 // I/O read rate limit in operations per second
	IOWriteIOPS   uint64 // I/O write rate limit in operations per second
}

// ResourceUsage represents the current resource consumption of a job.
type ResourceUsage struct {
	CPUUsage     uint64 // CPU usage in nanoseconds
	MemoryUsage  uint64 // Memory usage in bytes
	IOReadBytes  uint64 // Total I/O bytes read
	IOWriteBytes uint64 // Total I/O bytes written
	IOReadOps    uint64 // Total I/O read operations
	IOWriteOps   uint64 // Total I/O write operations
}

// jobResource represents the resources associated with a single job.
type jobResource struct {
	mutex sync.Mutex // Mutex for synchronizing access to this job's resources
	path  string     // Path to this job's cgroup
}

// ResourceManager manages system resources for multiple jobs.
type ResourceManager struct {
	mu   sync.Mutex              // Mutex for synchronizing access to the jobs map
	jobs map[string]*jobResource // Map of job IDs to their associated resources
}

// NewResourceManager creates a new ResourceManager.
func NewResourceManager() *ResourceManager

// SetLimits sets resource limits for a job.
func (rm *ResourceManager) SetLimits(jobID string, limits ResourceLimits) error

// Adds a process to a job's cgroups.
// It writes the process ID to the tasks file in each of the job's cgroups.
func (rm *ResourceManager) AddProcess(jobID string, pid int) error

// Retrieves the current resource usage for a job.
func (rm *ResourceManager) GetUsage(jobID string) ResourceUsage

// Removes a job's cgroups and cleans up associated resources.
func (rm *ResourceManager) RemoveJob(jobID string) error
```

### 2. API

#### gRPC API Specification

```protobuf
syntax = "proto3";

package jobworker;

import "google/protobuf/timestamp.proto";

// JobWorker service provides methods to manage and monitor jobs.
service JobWorker {
  // Starts a new job with the given command and resource limits.
  rpc StartJob(StartJobRequest) returns (StartJobResponse) {}
  
  // Stops a running job identified by its ID.
  rpc StopJob(JobId) returns (StopJobResponse) {}
  
  // Retrieves the current status of a job.
  rpc GetJobStatus(JobId) returns (stream JobStatusResponse) {}
  
  // Streams the output of a running job.
  rpc GetJobOutput(JobId) returns (stream JobOutputResponse) {}

  // Returns the list of all the jobs for the user
  rpc ListJobs() returns (ListJobsResponse) {}
}

// StartJobRequest contains the information needed to start a new job.
message StartJobRequest {
  // The command to be executed.
  string command = 1;
  
  // The arguments for the command.
  repeated string args = 2;
  
  // The resource limits for the job.
  ResourceLimits resource_limits = 3;
}

message JobId {
  // The UUID of the job.
  string id = 1;
}

// StartJobResponse contains the result of a job start request.
message StartJobResponse {
  // The UUID of the started job.
  string id = 1;
  
  // The initial status of the job.
  JobStatus status = 2;
}

// StopJobResponse contains the result of a job stop request.
message StopJobResponse {
  // The UUID of the stopped job.
  string id = 1;
  
  // The final status of the job.
  JobStatus status = 2;
  
  // The time when the job was started.
  google.protobuf.Timestamp start_time = 3;
  
  // The time when the job was stopped.
  google.protobuf.Timestamp stop_time = 4;
}

// JobStatusResponse contains the current status and resource usage of a job.
message JobStatusResponse {
  // The current status of the job.
  JobStatus status = 1;
  
  // The current resource usage of the job.
  ResourceUsage resource_usage = 2;
}

// JobOutputResponse contains a chunk of output data from a job.
message JobOutputResponse {
  // A chunk of output data from the job.
  bytes data = 1;
}

// ResourceLimits defines the resource constraints for a job.
message ResourceLimits {
  // The number of CPU shares allocated to the job.
  uint64 cpu_shares = 1;
  
  // The memory limit in bytes.
  uint64 memory_limit_bytes = 2;
  
  // The I/O weight for prioritizing disk I/O.
  uint64 io_weight = 3;
}

// ResourceUsage represents the current resource consumption of a job.
message ResourceUsage {
  // The current CPU usage.
  uint64 cpu_usage = 1;
  
  // The current memory usage in bytes.
  uint64 memory_usage = 2;
  
  // The current I/O usage.
  uint64 io_usage = 3;
}

message ListJobsRequest {
  // List of user ids to retrieve running jobs for
  // Will return all if empty
  repeated string user_ids = 1;
}

// List of jobs by user
message ListJobsResponse {
   message JobList {
      repeated string job_ids = 1; 
   }
   map<string, JobList> jobs = 1;
}

// JobStatus represents the possible states of a job.
enum JobStatus {
  // The job is waiting to be executed.
  PENDING = 0;
  RUNNING = 1;
  // The job has completed successfully.
  COMPLETED = 2;
  // The job has failed.
  FAILED = 3;  
  // The job was stopped by user request.
  STOPPED = 4;
}
```

#### Authentication and Authorization
- Use mutual TLS (mTLS) for authentication
- Use X.509 certificate extensions to store role information
- Implement a role-based access control (RBAC) system with the following roles:
  - Admin: Full access to all operations
  - User: Can start/stop/query own jobs
  - Reader: Can only query job status and output

#### TLS Configuration
- TLS version: 1.3
- Cipher suites: 
  - TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384
  - TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
  - TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256
  - TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256
- Key exchange: ECDHE with P-256 curve
- Certificate signature: ECDSA with P-256 curve and SHA-256

### 3. CLI Client

Implement a command-line interface with the following capabilities:
- `start`: Start a new job with resource limits
- `stop`: Stop a running job
- `status`: Get the current status and resource usage of a job
- `stream`: Stream the output of a running job

Example usage:
```bash
$ jobworker start "echo 'Life is Great'" --cpu-weight 1024 --memory-limit 100M --io-weight 100
Job started. ID: 550e8400-e29b-41d4-a716-446655440000

$ jobworker status 550e8400-e29b-41d4-a716-446655440000
Status: RUNNING
CPU Usage: 1024 uS
Memory Usage: 512MB
I/O Read: 4MB
I/O Write: 0B

$ jobworker stream 550e8400-e29b-41d4-a716-446655440000
Life is Great

$ jobworker stop 550e8400-e29b-41d4-a716-446655440000
Status: STOPPED
Job ID: 550e8400-e29b-41d4-a716-446655440000
```

## Job Creation Flow

0. Client and Server TLS certificates are generated
1. Client sends job request with TLS
2. Server authenticates client and checks authorization
3. Request is validated and sent to job manager queue
4. Job Manager creates job and assigns to Executor
5. CGroup Manager sets resource limits
6. Executor runs job within cgroup constraints
7. Output and resource usage are streamed to logger
8. Client requests output log stream
9. Logger streams logs to client(s)

Here's a visual representation of the job creation flow:
![alt text](job_worker.svg)
## Testing Approach

1. Authentication/Authorization layer
   - Test mTLS handshake with valid and invalid certificates
   - Test authorization checks for different roles

2. Networking and job management
   - Test job lifecycle (start, status, stream, stop)
   - Test concurrent output streaming
   - Test resource limit enforcement

## Edge Cases and Considerations

1. **Process Termination**: Implement forceful termination for unresponsive processes. Kill processes that don't respond to standard termination signals.

2. **Resource Exhaustion**: 
   - Monitor total resource usage and implement OOM killer
   - Keep track of total resource usage and only start processes from the queue if there are enough resources
   - Configure resources at the root level cgroup
   - Implement a hard limit by enabling the OOM killer, with a configurable percentage of the input Resource Limit

3. **Network Interruptions**: 
   - Ensure graceful handling of disconnections and reconnections
   - Stream logs from the beginning each time a streaming connection is established, as logs are stored in the Logger module

4. **Large Output Streams**: 
   - Consider persistent storage for large outputs
   - For this PoC, assume infinite memory, but note the need for a storage solution in production

5. **Concurrent Access**: 
   - Use in-memory logger to support multiple clients
   - Stream all output from the job to each client through goroutines when initiating the streaming gRPC to retrieve output

6. **Job Timeouts**: Implement configurable job timeouts

7. **Invalid Commands**: Consider input sanitization and process sandboxing (future enhancement)

8. **Certificate Expiration**: Implement certificate renewal policy (future enhancement)

## Future Enhancements

- Distributed architecture for high availability
- Container-based job isolation
- Enhanced security measures (e.g., input sanitization, process sandboxing)
- Certificate renewal automation
- Persistent storage solution for large output streams
- Improved error handling and recovery mechanisms
