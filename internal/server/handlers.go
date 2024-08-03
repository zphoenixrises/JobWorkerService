package server

import (
	"context"
	"io"
	"log"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	api "github.com/zphoenixrises/JobWorkerService/api/proto"
	"github.com/zphoenixrises/JobWorkerService/pkg/jobworker"
)

func getJobStatus(status jobworker.JobStatus) api.JobStatus {
	return api.JobStatus(api.JobStatus_value[string(status)])
}

func (s *JobWorkerServer) StartJob(ctx context.Context, req *api.StartJobRequest) (*api.StartJobResponse, error) {
	username, ok := ctx.Value("username").(string)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}

	executor, jobID, err := jobworker.NewExecutor(req.Command, req.Args, jobworker.ResourceLimits{
		CPUWeight:   req.GetResourceLimits().CpuWeight,
		MemoryLimit: req.ResourceLimits.MemoryLimitBytes,
		IOBPS:       req.GetResourceLimits().IoMaxBps,
	})

	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create executor: %v", err)
	}

	if err := executor.Start(); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to start job: %v", err)
	}

	userJobs := s.getUserJobs(username)
	userJobs.mu.Lock()
	userJobs.jobs[jobID] = executor
	userJobs.mu.Unlock()

	executor.GetStatus()

	return &api.StartJobResponse{
		Id:     jobID,
		Status: api.JobStatus_RUNNING,
	}, nil
}

func (s *JobWorkerServer) StopJob(ctx context.Context, req *api.JobId) (*api.StopJobResponse, error) {
	username, ok := ctx.Value("username").(string)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}

	userJobs := s.getUserJobs(username)
	userJobs.mu.RLock()
	executor, ok := userJobs.jobs[req.Id]
	userJobs.mu.RUnlock()

	if !ok {
		return nil, status.Errorf(codes.NotFound, "job not found")
	}

	if err := executor.Stop(); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to stop job: %v", err)
	}

	return &api.StopJobResponse{
		Id:        req.Id,
		Status:    api.JobStatus_STOPPED,
		StartTime: timestamppb.New(executor.GetStartTime()),
		StopTime:  timestamppb.New(executor.GetStopTime()),
	}, nil
}

func (s *JobWorkerServer) GetJobStatus(ctx context.Context, req *api.JobId) (*api.JobStatusResponse, error) {
	username, ok := ctx.Value("username").(string)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}

	userJobs := s.getUserJobs(username)
	userJobs.mu.RLock()
	executor, ok := userJobs.jobs[req.Id]
	userJobs.mu.RUnlock()

	if !ok {
		return nil, status.Errorf(codes.NotFound, "job not found")
	}

	status := executor.GetStatus()

	return &api.JobStatusResponse{
		Status:    getJobStatus(status),
		Errorcode: int32(executor.GetExitCode()),
		Error:     executor.GetErrorString(),
	}, nil
}

func (s *JobWorkerServer) GetJobOutput(req *api.JobOutputRequest, stream api.JobWorker_GetJobOutputServer) error {
	username, ok := stream.Context().Value("username").(string)
	if !ok {
		return status.Errorf(codes.Unauthenticated, "user not authenticated")
	}

	userJobs := s.getUserJobs(username)
	userJobs.mu.RLock()
	executor, ok := userJobs.jobs[req.Id]
	userJobs.mu.RUnlock()

	if !ok {
		return status.Errorf(codes.NotFound, "job not found")
	}

	stream.Context().Done()
	outputReader := executor.GetOutputReader()
	errorReader := executor.GetErrorReader()
	ctx, cancel := context.WithCancel(stream.Context())
	go s.streamOutput(cancel, stream, outputReader, errorReader)

	<-ctx.Done()
	return nil
}

func (s *JobWorkerServer) streamOutput(cancel context.CancelFunc, stream api.JobWorker_GetJobOutputServer, outputReader, errorReader io.Reader) {
	buffer := make([]byte, 1024)
	var wg sync.WaitGroup
	clientCtx := stream.Context()

	log.Println("Started streaming output and errors")
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-clientCtx.Done():
				cancel()
				return
			default:
				n, err := outputReader.Read(buffer)
				if err == io.EOF {
					return
				}
				if err != nil {
					log.Printf("Error reading stdout: %v\n", err)
					break
				}
				if n > 0 {
					if err := stream.Send(&api.JobOutputResponse{
						Output: &api.JobOutputResponse_Stdout{
							Stdout: buffer[:n],
						},
					}); err != nil {
						log.Printf("Error sending stdout: %v\n", err)
						return
					}
				}
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-clientCtx.Done():
				cancel()
				return
			default:
				n, err := errorReader.Read(buffer)
				if err == io.EOF {
					return
				}
				if err != nil {
					log.Printf("Error reading stderr: %v\n", err)
					break
				}
				if n > 0 {
					if err := stream.Send(&api.JobOutputResponse{
						Output: &api.JobOutputResponse_Stderr{
							Stderr: buffer[:n],
						},
					}); err != nil {
						log.Printf("Error sending stderr: %v\n", err)
						return
					}
				}
			}
		}
	}()
	wg.Wait()
	log.Println("Stopped Streaming output and errors")
	cancel()
}
