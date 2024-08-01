package jobworker

import (
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestExecutorLifecycle(t *testing.T) {
	executor, jobID, err := NewExecutor("echo", []string{"Hello, World!"}, ResourceLimits{
		CPUWeight:   10000,
		MemoryLimit: 1024 * 1024,
		IOWeight:    10485760,
	})
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}
	if jobID == "" {
		t.Fatalf("Expected non-empty jobID")
	}

	// Test Start
	err = executor.Start()
	if err != nil {
		t.Fatalf("Failed to start executor: %v", err)
	}

	// Test Status
	status := executor.GetStatus()
	if status != JobStatusRunning {
		t.Errorf("Expected status Running, got %v", status)
	}

	// Test Stream
	output, err := readAllOutput(executor.GetOutputReader())
	if err != nil {
		t.Fatalf("Failed to read output: %v", err)
	}
	if !strings.Contains(output, "Hello, World!") {
		t.Errorf("Expected output to contain 'Hello, World!', got %s", output)
	}

	// Test Stop
	err = executor.Stop()
	if err != nil && executor.status != JobStatusCompleted {
		t.Fatalf("Failed to stop executor: %v", err)
	}

	status = executor.GetStatus()
	if status != JobStatusCompleted {
		t.Errorf("Expected status Stopped, got %v", status)
	}
}

func TestConcurrentOutputStreaming(t *testing.T) {
	executor, _, err := NewExecutor("yes", nil, ResourceLimits{
		CPUWeight:   10000,
		MemoryLimit: 1024 * 1024,
		IOWeight:    10485760,
	}) // Continuously output 'y'
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	err = executor.Start()
	if err != nil {
		t.Fatalf("Failed to start executor: %v", err)
	}

	var wg sync.WaitGroup
	readerCount := 5
	outputChan := make(chan string, readerCount)

	for i := 0; i < readerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reader := executor.GetOutputReader()
			buffer := make([]byte, 1024)
			n, err := reader.Read(buffer)
			if err != nil && err != io.EOF {
				t.Errorf("Error reading from output: %v", err)
				return
			}
			outputChan <- string(buffer[:n])
		}()
	}

	time.Sleep(100 * time.Millisecond) // Allow some time for output generation
	executor.Stop()

	wg.Wait()
	close(outputChan)

	outputs := make([]string, 0, readerCount)
	for output := range outputChan {
		outputs = append(outputs, output)
	}

	if len(outputs) != readerCount {
		t.Errorf("Expected %d outputs, got %d", readerCount, len(outputs))
	}

	for _, output := range outputs {
		if !strings.Contains(output, "y") {
			t.Errorf("Expected output to contain 'y', got %s", output)
		}
	}
}

func TestJobsWithDifferentOutputs(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		args     []string
		duration time.Duration
		expected string
	}{
		{"SparseOutput", "sh", []string{"-c", "echo 'start'; sleep 0.1; echo 'middle'; sleep 0.1; echo 'end'"}, 300 * time.Millisecond, "start\nmiddle\nend\n"},
		// {"DenseOutput", "yes", []string{"test"}, 1 * time.Millisecond, "test\n"},
		{"LongRunningJob", "sleep", []string{"1"}, 1100 * time.Millisecond, ""},
		{"ShortRunningJob", "echo", []string{"quick job"}, 100 * time.Millisecond, "quick job\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor, _, err := NewExecutor(tt.cmd, tt.args, ResourceLimits{
				CPUWeight:   10000,
				MemoryLimit: 1024 * 1024,
				IOWeight:    10485760,
			})
			if err != nil {
				t.Fatalf("Failed to create executor: %v", err)
			}

			err = executor.Start()
			if err != nil {
				t.Fatalf("Failed to start executor: %v", err)
			}

			time.Sleep(tt.duration)

			output, err := readAllOutput(executor.GetOutputReader())
			if err != nil {
				t.Fatalf("Failed to read output: %v", err)
			}

			if tt.expected != "" && !strings.Contains(output, tt.expected) {
				t.Errorf("Expected output to contain '%s', got '%s'", tt.expected, output)
			}

			executor.Stop()

			status := executor.GetStatus()
			if status != JobStatusStopped && status != JobStatusCompleted {
				t.Errorf("Expected final status Stopped or Completed, got %v", status)
			}
		})
	}
}

func TestResourceLimitEnforcement(t *testing.T) {
	tests := []struct {
		name       string
		command    string
		args       []string
		resource   string
		limitValue int
	}{
		{"CPU Stress", "stress-ng", []string{"--cpu", "2", "--timeout", "2s"}, "cpu", 10000},
		{"Memory Stress", "stress-ng", []string{"--vm", "1", "--vm-bytes", "2M", "--timeout", "2s"}, "memory", 1024 * 1024}, // 1 MB
		{"IO Stress", "stress-ng", []string{"--hdd", "1", "--hdd-bytes", "20M", "--timeout", "2s"}, "io", 10485760},         // 10 MB/s
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resourceLimits := ResourceLimits{
				CPUWeight:   10000,
				MemoryLimit: 1024 * 1024, // 1 MB
				IOWeight:    10485760,    // 10 MB/s
			}

			executor, jobID, err := NewExecutor(tt.command, tt.args, resourceLimits)
			if err != nil {
				t.Fatalf("Failed to create executor: %v", err)
			}
			if jobID == "" {
				t.Fatalf("Expected non-empty jobID")
			}

			// Test Start
			err = executor.Start()
			if err != nil {
				t.Fatalf("Failed to start executor: %v", err)
			}

			go func() {
				time.Sleep(3 * time.Second)
				executor.Stop()
			}()
			// Test Stops
			err = executor.Wait()
			if err != nil {
				t.Fatalf("Failed to stop executor: %v", err)
			}

			status := executor.GetStatus()
			if status != JobStatusCompleted {
				t.Errorf("Expected status Completed, got %v", status)
			}
		})
	}
}

func readAllOutput(reader io.Reader) (string, error) {
	output, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(output), nil
}
