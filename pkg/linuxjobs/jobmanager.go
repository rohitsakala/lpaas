package linuxjobs

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/google/uuid"
)

// newJobID returns a unique job identifier.
func newJobID() string {
	return fmt.Sprintf("job-%s", uuid.NewString())
}

// JobManager manages the lifecycle of all jobs. It is safe for concurrent use.
type JobManager struct {
	jobs map[string]*job
	mu   sync.Mutex
}

// NewJobManager creates a JobManager with the map to hold jobs.
func NewJobManager() (*JobManager, error) {
	return &JobManager{
		jobs: make(map[string]*job),
	}, nil
}

// StartJob creates a job and starts running it.
func (jm *JobManager) StartJob(command string, args ...string) (string, error) {
	jobID := newJobID()

	job, err := newJob(jobID, command, args...)
	if err != nil {
		return "", fmt.Errorf("create job: %w", err)
	}

	if err := job.start(context.Background()); err != nil {
		return "", fmt.Errorf("failed to start job %s: %w", jobID, err)
	}

	jm.mu.Lock()
	jm.jobs[jobID] = job
	jm.mu.Unlock()

	return job.ID, nil
}

// StopJob calls the stop function of the job with the given ID.
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

	return nil
}

// Status returns the job's status, exit code (if any), and exit error (exit error will contain the cleanup error if any).
func (jm *JobManager) Status(jobID string) (string, *int32, error) {
	jm.mu.Lock()
	job, ok := jm.jobs[jobID]
	jm.mu.Unlock()

	if !ok {
		return "", nil, fmt.Errorf("job %s not found", jobID)
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

// StreamJob returns an io.ReadCloser that streams live and past output of a running job.
// The reader must be closed by the caller when no longer needed.
func (jm *JobManager) StreamJob(jobID string) (io.ReadCloser, error) {
	jm.mu.Lock()
	job, ok := jm.jobs[jobID]
	jm.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("job %s not found", jobID)
	}
	return job.stream(), nil
}
