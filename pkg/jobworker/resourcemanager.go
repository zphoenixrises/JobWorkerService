package jobworker

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
)

const (
	cgroupsPath   = "/sys/fs/cgroup"
	jobWorkerPath = "jobworker"
)

var cgroupInitialized = false

type ResourceLimits struct {
	CPUWeight   uint64
	MemoryLimit uint64
	IOWeight    uint64
}

type ResourceManager struct {
	mutex  sync.Mutex
	limits ResourceLimits
	path   string
	jobID  string
}

func init() {
	if err := os.MkdirAll(filepath.Join(cgroupsPath, jobWorkerPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create root cgroup: %v", err)
		return
	}
	// Enable controllers for the parent group (jobworker)
	if err := enableControllers(filepath.Join(cgroupsPath, jobWorkerPath)); err != nil {
		fmt.Fprintf(os.Stderr, "failed to enable controllers: %v", err)
		return
	}

	cgroupInitialized = true
}

// A new cgroup will be initialized by generating a UUID.
func NewResourceManager() (*ResourceManager, error) {
	jobID := uuid.New().String()
	if !cgroupInitialized {
		return nil, fmt.Errorf("cgroups root directory not initialized")
	}
	path := filepath.Join(cgroupsPath, jobWorkerPath, jobID)

	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cgroup: %v", err)
	}

	return &ResourceManager{
		jobID: jobID,
		path:  path,
	}, nil
}

func enableControllers(path string) error {
	controllersPath := filepath.Join(path, "cgroup.subtree_control")
	fmt.Printf("controllerpath: %s", controllersPath)
	// Check if the subtree_control file exists
	// if _, err := os.Stat(controllersPath); os.IsNotExist(err) {
	// 	return fmt.Errorf("cgroup.subtree_control not found at %s", controllersPath)
	// }
	return os.WriteFile(controllersPath, []byte("+cpu +io +memory"), 0644)
}

func (rm *ResourceManager) SetLimits(limits ResourceLimits) error {
	rm.limits = limits
	if err := rm.setCPULimit(); err != nil {
		return err
	}

	if err := rm.setMemoryLimit(); err != nil {
		return err
	}

	if err := rm.setIOLimit(); err != nil {
		return err
	}

	return nil
}

func (rm *ResourceManager) setCPULimit() error {
	limit := fmt.Sprintf("%d 100000", rm.limits.CPUWeight)
	fmt.Println("Setting cpu limit : ", limit)
	return os.WriteFile(filepath.Join(rm.path, "cpu.max"), []byte(limit), 0644)
}

func (rm *ResourceManager) setMemoryLimit() error {
	return os.WriteFile(filepath.Join(rm.path, "memory.max"), []byte(fmt.Sprintf("%d", rm.limits.MemoryLimit)), 0644)
}

func (rm *ResourceManager) setIOLimit() error {
	limit := fmt.Sprintf("default rbps=%d wbps=%d riops=max wiops=max", rm.limits.IOWeight, rm.limits.IOWeight)

	if err := os.WriteFile(filepath.Join(rm.path, "io.max"), []byte(limit), 0644); err != nil {
		return nil
	}

	// If default is not supported, get major:minor list and set dynamically for each device
	majorMinorList, err := getMajorMinorNumbers()
	if err != nil {
		return fmt.Errorf("unable to get io devices")
	}
	successful := false

	for _, majorMinor := range majorMinorList {
		limit := fmt.Sprintf("%d:%d rbps=%d wbps=%d riops=max wiops=max", majorMinor[0], majorMinor[1], rm.limits.IOWeight, rm.limits.IOWeight)
		if err := os.WriteFile(filepath.Join(rm.path, "io.max"), []byte(limit), 0644); err == nil {
			successful = true
		}
	}
	if !successful {
		return fmt.Errorf("unable to set IO limits")
	}
	return nil
}

func (rm *ResourceManager) AddProcess(pid int) error {
	return os.WriteFile(filepath.Join(rm.path, "cgroup.procs"), []byte(strconv.Itoa(pid)), 0644)
}

func (rm *ResourceManager) Cleanup() error {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	if err := os.RemoveAll(rm.path); err != nil {
		return fmt.Errorf("failed to remove cgroup: %v", err)
	}
	return nil
}

func getMajorMinorNumbers() ([][2]int, error) {
	file, err := os.Open("/proc/partitions")
	if err != nil {
		return nil, fmt.Errorf("failed to open /proc/partitions: %w", err)
	}
	defer file.Close()

	var majorMinorList [][2]int
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 4 {
			major, err1 := strconv.Atoi(fields[0])
			minor, err2 := strconv.Atoi(fields[1])
			if err1 == nil && err2 == nil {
				majorMinorList = append(majorMinorList, [2]int{major, minor})
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read /proc/partitions: %w", err)
	}
	return majorMinorList, nil
}
