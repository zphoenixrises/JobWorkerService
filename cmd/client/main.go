package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"

	api "github.com/zphoenixrises/JobWorkerService/api/proto"
	"github.com/zphoenixrises/JobWorkerService/internal/client"
)

func main() {
	client, err := client.NewJobWorkerClient()
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "start":
		handleStart(client, args)
	case "stop":
		handleStop(client, args)
	case "status":
		handleStatus(client, args)
	case "stream":
		handleStream(client, args)
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: ./jobworker-client <command> [arguments]")
	fmt.Println("Commands:")
	fmt.Println("  start <cpulimit> <memorylimit> <iobpslimit> <command> [args...]    Start a new job")
	fmt.Println("  stop <job_id>                                                    Stop a running job")
	fmt.Println("  status <job_id>                                                  Get the status of a job")
	fmt.Println("  stream <job_id>                                                  Stream the output of a job")
}

func handleStart(client *client.JobWorkerClient, args []string) {
	if len(args) < 4 {
		fmt.Println("Usage: client start <command> [args...]")
		os.Exit(1)
	}

	cpu, _ := strconv.ParseFloat(args[0], 64)
	memory, _ := strconv.ParseUint(args[1], 10, 64)
	io, _ := strconv.ParseUint(args[2], 10, 64)

	command := args[3]
	commandArgs := args[4:]
	limits := &api.ResourceLimits{
		CpuWeight:        float32(cpu),
		MemoryLimitBytes: memory,
		IoMaxBps:         io,
	}

	resp, err := client.StartJob(context.Background(), command, commandArgs, limits)
	if err != nil {
		log.Fatalf("Failed to start job: %v", err)
	}

	fmt.Printf("Job started with ID: %s\nStatus: %s\n", resp.Id, resp.Status)
}

func handleStop(client *client.JobWorkerClient, args []string) {
	if len(args) != 1 {
		fmt.Println("Usage: client stop <job_id>")
		os.Exit(1)
	}

	jobID := args[0]

	resp, err := client.StopJob(context.Background(), jobID)
	if err != nil {
		log.Fatalf("Failed to stop job: %v", err)
	}

	fmt.Printf("Job stopped with ID: %s\nFinal Status: %s\n", resp.Id, resp.Status)
}

func handleStatus(client *client.JobWorkerClient, args []string) {
	if len(args) != 1 {
		fmt.Println("Usage: client status <job_id>")
		os.Exit(1)
	}

	jobID := args[0]

	resp, err := client.GetJobStatus(context.Background(), jobID)
	if err != nil {
		log.Fatalf("Failed to get job status: %v", err)
	}

	fmt.Printf("Job ID: %s\nStatus: %s\n", jobID, resp.Status)
}

func handleStream(client *client.JobWorkerClient, args []string) {
	if len(args) != 1 {
		fmt.Println("Usage: client stream <job_id>")
		os.Exit(1)
	}

	jobID := args[0]

	stream, err := client.GetJobOutput(context.Background(), jobID)
	if err != nil {
		log.Fatalf("Failed to get job output stream: %v", err)
	}

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("Error receiving job output: %v", err)
		}

		switch output := resp.Output.(type) {
		case *api.JobOutputResponse_Stdout:
			fmt.Print(string(output.Stdout))
		case *api.JobOutputResponse_Stderr:
			fmt.Fprint(os.Stderr, string(output.Stderr))
		}
	}
}
