package linuxjobs

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

var (
	// cgroupInitOnce ensures the cgroup hierarchy is initialized only once.
	cgroupInitOnce sync.Once
)

// initCgroupHierarchy sets up the LPaaS cgroup root and enables controllers.
func initCgroupHierarchy() error {
	// Create root /sys/fs/cgroup/lpaas
	if err := os.MkdirAll(lpaasCgroupRoot, 0755); err != nil {
		return fmt.Errorf("create cgroup root: %w", err)
	}

	// Enable controllers once
	if err := enableControllers(cgroupRootPath); err != nil {
		return err
	}
	if err := enableControllers(lpaasCgroupRoot); err != nil {
		return err
	}

	return nil
}

// ensureCgroupHierarchy initializes the cgroup hierarchy once only.
func ensureCgroupHierarchy() error {
	var cgroupInitErr error
	cgroupInitOnce.Do(func() {
		cgroupInitErr = initCgroupHierarchy()
	})
	return cgroupInitErr
}

// newJobID returns a unique job identifier.
func newJobID() string {
	return fmt.Sprintf("job-%s", uuid.NewString())
}

// JobManager manages the lifecycle of all jobs. It is safe for concurrent use.
type JobManager struct {
	jobs map[string]*job
	mu   sync.Mutex
}

// NewJobManager creates a JobManager and initializes the cgroup hierarchy.
func NewJobManager() (*JobManager, error) {
	if err := ensureCgroupHierarchy(); err != nil {
		return nil, fmt.Errorf("failed to initialize cgroup: %w", err)
	}

	return &JobManager{
		jobs: make(map[string]*job),
	}, nil
}

// StartJob creates a job, configures its cgroup, and starts the process.
func (jm *JobManager) StartJob(command string, args ...string) (string, error) {
	jobID := newJobID()

	cg, err := newCGroupV2(jobID)
	if err != nil {
		return "", fmt.Errorf("create cgroup: %w", err)
	}
	if err := cg.setLimits(); err != nil {
		return "", fmt.Errorf("set limits: %w", err)
	}

	fd, err := cg.openFD()
	if err != nil {
		return "", fmt.Errorf("open cgroup FD: %w", err)
	}

	job := newJob(jobID, command, args...)
	job.cgroup = cg

	// Start job synchronously (internally spawns cmd.Wait goroutine)
	if err := job.start(context.Background(), fd); err != nil {
		unix.Close(fd)
		return "", fmt.Errorf("failed to start job %s: %w", jobID, err)
	}

	unix.Close(fd)

	jm.mu.Lock()
	jm.jobs[jobID] = job
	jm.mu.Unlock()

	return job.ID, nil
}

// StopJob stops the job with the given ID.
func (jm *JobManager) StopJob(jobID string) error {
	jm.mu.Lock()
	job, ok := jm.jobs[jobID]
	jm.mu.Unlock()
	if !ok {
		return fmt.Errorf("job %s not found", jobID)
	}

	if err := job.stop(); err != nil {
		return fmt.Errorf("stop job: %w", err)
	}

	<-job.done

	job.mu.Lock()
	job.status = stopped
	job.mu.Unlock()

	return nil
}

// Status returns the job's status, exit code (if any), and exit error.
func (jm *JobManager) Status(jobID string) (string, *int32, error) {
	jm.mu.Lock()
	job, ok := jm.jobs[jobID]
	jm.mu.Unlock()

	if !ok {
		return "Unknown", nil, fmt.Errorf("job %s not found", jobID)
	}

	statusVal, code, jobErr := job.statusSnapshot()

	var exitCode *int32
	if statusVal == exited || statusVal == failed || statusVal == stopped {
		v := int32(code)
		exitCode = &v
	}

	return statusVal.String(), exitCode, jobErr
}

// JobExists returns true if a job with the given ID exists.
func (jm *JobManager) JobExists(jobID string) bool {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	_, ok := jm.jobs[jobID]
	return ok
}

// StreamJob returns an io.Reader that streams live and past output of a running job.
func (jm *JobManager) StreamJob(jobID string) (io.ReadCloser, error) {
	jm.mu.Lock()
	job, ok := jm.jobs[jobID]
	jm.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("job %s not found", jobID)
	}
	return job.stream(), nil
}
