package jobworker

import (
	"bufio"
	"fmt"
	"log"
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
var enableCgroups = true

type ResourceLimits struct {
	CPUWeight   float32
	MemoryLimit uint64
	IOBPS       uint64
}

type ResourceManager struct {
	mutex  sync.Mutex
	limits ResourceLimits
	path   string
	jobID  string
}

func init() {
	if !enableCgroups {
		return
	}
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
func newResourceManager() (*ResourceManager, error) {
	jobID := uuid.New().String()
	if !enableCgroups {
		return &ResourceManager{
			jobID: jobID,
			path:  "",
		}, nil
	}

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
	log.Printf("controllerpath: %s", controllersPath)
	return os.WriteFile(controllersPath, []byte("+cpu +io +memory"), 0644)
}

func (rm *ResourceManager) setLimits(limits ResourceLimits) error {
	if !enableCgroups {
		return nil
	}
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
	weight := int64(rm.limits.CPUWeight * 100000)
	if weight > 100000 {
		weight = 100000
	}
	if weight < 100 {
		weight = 100
	}
	limit := fmt.Sprintf("%d 100000", weight)
	log.Println("Setting cpu limit : ", limit)
	return os.WriteFile(filepath.Join(rm.path, "cpu.max"), []byte(limit), 0644)
}

func (rm *ResourceManager) setMemoryLimit() error {
	limit := fmt.Sprintf("%d", rm.limits.MemoryLimit)
	log.Println("Setting memory limit : ", limit)
	return os.WriteFile(filepath.Join(rm.path, "memory.max"), []byte(limit), 0644)
}

func (rm *ResourceManager) setIOLimit() error {
	limit := fmt.Sprintf("default rbps=%d wbps=%d riops=max wiops=max", rm.limits.IOBPS, rm.limits.IOBPS)
	log.Println("Setting io limit : ", limit)
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
		limit := fmt.Sprintf("%d:%d rbps=%d wbps=%d riops=max wiops=max", majorMinor[0], majorMinor[1], rm.limits.IOBPS, rm.limits.IOBPS)
		log.Println("Setting io limit : ", limit)
		if err := os.WriteFile(filepath.Join(rm.path, "io.max"), []byte(limit), 0644); err == nil {
			successful = true
		}
	}
	if !successful {
		return fmt.Errorf("unable to set IO limits")
	}
	return nil
}

func (rm *ResourceManager) addProcess(pid int) error {
	if !enableCgroups {
		return nil
	}
	return os.WriteFile(filepath.Join(rm.path, "cgroup.procs"), []byte(strconv.Itoa(pid)), 0644)
}

func (rm *ResourceManager) Cleanup() error {
	if !enableCgroups {
		return nil
	}
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
