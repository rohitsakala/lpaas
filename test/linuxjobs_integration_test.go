package test

import (
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/rohitsakala/lpaas/pkg/linuxjobs"
)

// Test Start and Status of a running job
func TestStartJobAndStatusRunning(t *testing.T) {
	t.Parallel()
	jm, err := linuxjobs.NewJobManager()
	require.NoError(t, err, "NewJobManager")

	jobID, err := jm.StartJob("sleep", "3")
	require.NoError(t, err, "StartJob")

	status, code, err := jm.Status(jobID)
	require.NoError(t, err, "Status")

	require.Equal(t, "Running", status, "status must be Running")
	require.Nil(t, code, "exitCode must be nil for running job")
}

// Test Stop Job
func TestStopJob(t *testing.T) {
	t.Parallel()
	jm, err := linuxjobs.NewJobManager()
	require.NoError(t, err, "NewJobManager")

	jobID, err := jm.StartJob("sleep", "2")
	require.NoError(t, err, "StartJob")

	err = jm.StopJob(jobID)
	require.NoError(t, err, "StopJob")

	require.Eventually(t, func() bool {
		status, code, err := jm.Status(jobID)
		if err != nil {
			return true
		}
		if status != "Stopped" {
			return false
		}
		return code != nil
	}, 2*time.Second, 50*time.Millisecond, "job should move to Stopped state")
}

// Test Job Status failed
func TestJobStatusExited(t *testing.T) {
	t.Parallel()

	jm, err := linuxjobs.NewJobManager()
	require.NoError(t, err, "NewJobManager")

	jobID, err := jm.StartJob("bash", "-c", "exit 7")
	require.NoError(t, err, "StartJob")

	require.Eventually(t, func() bool {
		status, code, err := jm.Status(jobID)
		return err != nil && status == "Failed" && code != nil
	}, 2*time.Second, 50*time.Millisecond)
}

// Test Job Stream
func TestStreamLiveOutput(t *testing.T) {
	t.Parallel()
	jm, err := linuxjobs.NewJobManager()
	require.NoError(t, err, "NewJobManager")

	jobID, err := jm.StartJob("bash", "-c", "echo hello; sleep 0.2; echo world")
	require.NoError(t, err, "StartJob")

	r, err := jm.StreamJob(jobID)
	require.NoError(t, err, "StreamJob")
	defer r.Close()

	data, err := io.ReadAll(r)
	require.NoError(t, err, "ReadAll stream")

	out := string(data)
	require.Contains(t, out, "hello", "stream output should include hello")
	require.Contains(t, out, "world", "stream output should include world")
}

// Test Stream after exit
func TestStreamAfterExit(t *testing.T) {
	t.Parallel()
	jm, err := linuxjobs.NewJobManager()
	require.NoError(t, err, "NewJobManager")

	jobID, err := jm.StartJob("bash", "-c", "echo one; echo two")
	require.NoError(t, err, "StartJob")

	require.Eventually(t, func() bool {
		status, _, err := jm.Status(jobID)
		return err == nil && status == "Exited"
	}, 2*time.Second, 50*time.Millisecond, "job should exit within timeout")

	r, err := jm.StreamJob(jobID)
	require.NoError(t, err, "StreamJob")
	defer r.Close()

	data, err := io.ReadAll(r)
	require.NoError(t, err, "ReadAll")
	out := string(data)

	require.Contains(t, out, "one", "stream output should include one")
	require.Contains(t, out, "two", "stream output should include two")
}
