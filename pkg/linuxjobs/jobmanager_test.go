package linuxjobs

import (
	"testing"
)

func TestNewJobManager(t *testing.T) {
	jm, err := NewJobManager()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jm.jobs == nil {
		t.Fatalf("jobs map should be initialized")
	}
}

func TestJob_ExistingJob(t *testing.T) {
	jm := &JobManager{jobs: make(map[string]*job)}

	j := newJob("job-1", "echo")
	jm.jobs["job-1"] = j

	exists := jm.JobExists("job-1")
	if !exists {
		t.Fatalf("expected job to exist")
	}
}

func TestJob_NotFound(t *testing.T) {
	jm := &JobManager{jobs: make(map[string]*job)}
	ok := jm.JobExists("no-such-id")
	if ok {
		t.Fatalf("did not expect job to exist")
	}
}

func TestStatus_NotFound(t *testing.T) {
	jm := &JobManager{jobs: make(map[string]*job)}
	_, _, err := jm.Status("missing")
	if err == nil {
		t.Fatalf("expected error for missing job")
	}
}

func TestStopJob_NotFound(t *testing.T) {
	jm := &JobManager{jobs: make(map[string]*job)}
	err := jm.StopJob("missing")
	if err == nil {
		t.Fatalf("expected error for missing job")
	}
}

func TestStreamJob_NotFound(t *testing.T) {
	jm := &JobManager{jobs: make(map[string]*job)}

	_, err := jm.StreamJob("missing")
	if err == nil {
		t.Fatalf("expected error for missing job")
	}
}

func TestStatus_ReturnsValues(t *testing.T) {
	j := newJob("job-1", "echo")
	j.status = exited
	j.exitCode = 0
	j.exitErr = nil

	jm := &JobManager{jobs: map[string]*job{
		"job-1": j,
	}}

	st, exitCode, err := jm.Status("job-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st != "Exited" {
		t.Fatalf("expected exited status, got %v", st)
	}
	if exitCode == nil || *exitCode != 0 {
		t.Fatalf("expected exitCode=0")
	}
}

func TestStreamJob_ReturnsReader(t *testing.T) {
	j := newJob("job-1", "echo")
	j.status = running

	jm := &JobManager{jobs: map[string]*job{
		"job-1": j,
	}}

	r, err := jm.StreamJob("job-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if r == nil {
		t.Fatalf("expected reader, got nil")
	}
	_ = r.Close()
}
