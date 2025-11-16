package linuxjobs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"syscall"
)

type cgroup interface {
	delete() error
}

// status represents the lifecycle state of a job.
type status int

const (
	unknown status = iota
	// running is when the linux process is running
	running
	// stopped is when the client has requested to stop a running process
	stopped
	// exited is when the process exited itself
	exited
	// failed is when the process has failed
	failed
)

func (s status) String() string {
	switch s {
	case running:
		return "Running"
	case stopped:
		return "Stopped"
	case exited:
		return "Exited"
	case failed:
		return "Failed"
	default:
		return "Unknown"
	}
}

// job represents a single Linux process managed by the system.
type job struct {
	mu sync.Mutex

	ID      string
	command string
	args    []string
	cmd     *exec.Cmd

	status   status
	exitErr  error // raw error returned by cmd.Wait()
	exitCode int   // numeric exit code derived from exitErr

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{} // closed when job finishes

	outBuf  *lockedBuffer
	readers map[*streamingReader]chan struct{} // active log streamers
	cgroup  cgroup
}

// newJob creates a new job instance with the given command and arguments.
func newJob(id, cmd string, args ...string) *job {
	return &job{
		ID:      id,
		command: cmd,
		args:    args,
		outBuf:  &lockedBuffer{b: new(bytes.Buffer)},
		readers: make(map[*streamingReader]chan struct{}),
		done:    make(chan struct{}),
	}
}

// Start begins execution of the job using its own cancellable context.
func (j *job) start(ctx context.Context, cgroupFD int) error {
	j.ctx, j.cancel = context.WithCancel(ctx)

	cmd := exec.CommandContext(j.ctx, j.command, j.args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CgroupFD:    cgroupFD,
		UseCgroupFD: true,
	}

	writer := &notifyingWriter{job: j}
	cmd.Stdout = writer
	cmd.Stderr = writer

	j.cmd = cmd

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting a linuxjob failed: %w", err)
	}

	// This lock is not necessary here since no other goroutine can access j.status yet. But holding it for clarity.
	j.mu.Lock()
	j.status = running
	j.mu.Unlock()

	go func() {
		err := cmd.Wait()

		j.mu.Lock()
		j.exitErr = err
		j.exitCode = exitCodeFromErr(err)
		switch {
		case err == nil:
			j.status = exited
		default:
			j.status = failed
		}
		close(j.done)

		j.mu.Unlock()

	}()

	return nil
}

// stop terminates a running job gracefully.
func (j *job) stop() error {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.status != running {
		return fmt.Errorf("job %s not running", j.ID)
	}

	j.cancel()

	err := j.cgroup.delete()
	if err != nil {
		return fmt.Errorf("delete cgroup: %w", err)
	}

	return nil
}

// statusSnapshot returns a  snapshot of job status.
func (j *job) statusSnapshot() (status, int, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.status, j.exitCode, j.exitErr
}

// Stream creates a new reader for consuming job output.
func (j *job) stream() io.ReadCloser {
	j.mu.Lock()
	done := j.status == exited ||
		j.status == failed ||
		j.status == stopped
	j.mu.Unlock()

	if done {
		return io.NopCloser(bytes.NewReader(j.outBuf.bytes()))
	}

	r := &streamingReader{
		job:     j,
		offset:  0,
		newData: make(chan struct{}, 1),
	}
	j.mu.Lock()
	j.readers[r] = r.newData
	j.mu.Unlock()
	return r
}

// notifyingWriter writes process output to the shared buffer
// and notifies all active readers about new data.
type notifyingWriter struct {
	job *job
}

func (w *notifyingWriter) Write(p []byte) (int, error) {
	n, err := w.job.outBuf.write(p)

	// Notify readers non-blockingly
	w.job.mu.Lock()
	for _, ch := range w.job.readers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	w.job.mu.Unlock()

	return n, err
}

// streamingReader allows each client to independently consume job output.
type streamingReader struct {
	job     *job
	offset  int
	newData chan struct{}
}

func (r *streamingReader) Read(p []byte) (int, error) {
	for {
		total := r.job.outBuf.len()

		if r.offset < total {
			n, err := r.job.outBuf.readAt(p, r.offset)
			r.offset += n
			return n, err
		}

		select {
		case <-r.job.done:
			total = r.job.outBuf.len()
			if r.offset >= total {
				return 0, io.EOF
			}
		case <-r.newData:
			continue
		}
	}
}

func (r *streamingReader) Close() error {
	r.job.mu.Lock()
	delete(r.job.readers, r)
	r.job.mu.Unlock()

	close(r.newData)

	return nil
}

// lockedBuffer is a threadsafe buffer used for storing process output.
type lockedBuffer struct {
	mu sync.RWMutex
	b  *bytes.Buffer
	n  int
}

func (l *lockedBuffer) write(p []byte) (int, error) {
	l.mu.Lock()
	n, err := l.b.Write(p)
	l.n += n
	l.mu.Unlock()
	return n, err
}

func (l *lockedBuffer) len() int {
	l.mu.RLock()
	n := l.n
	l.mu.RUnlock()
	return n
}

func (l *lockedBuffer) bytes() []byte {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return slices.Clone(l.b.Bytes())
}

func (l *lockedBuffer) readAt(p []byte, offset int) (int, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if offset >= l.n {
		return 0, io.EOF
	}

	buf := l.b.Bytes()

	n := copy(p, buf[offset:])

	return n, nil
}

// exitCodeFromErr extracts the process exit code from exec errors.
func exitCodeFromErr(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}
