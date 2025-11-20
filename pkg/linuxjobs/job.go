package linuxjobs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

type cgroup interface {
	delete() error
	openFD() (int, error)
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

	ID         string
	command    string
	args       []string
	cmd        *exec.Cmd
	cleanupErr error

	status   status
	exitErr  error // raw error returned by cmd.Wait()
	exitCode int   // numeric exit code derived from exitErr

	cancel context.CancelFunc
	done   chan struct{} // closed when job finishes

	outBuf  *lockedBuffer
	readers map[*streamingReader]chan struct{} // active log streamers
	cgroup  cgroup
}

// newJob creates a new job instance with the given command and arguments.
func newJob(id, cmd string, args ...string) (*job, error) {
	cg, err := newCGroupV2(id, "")
	if err != nil {
		return nil, fmt.Errorf("create cgroup: %w", err)
	}

	if err := cg.setLimits(); err != nil {
		return nil, fmt.Errorf("set limits: %w", err)
	}

	return &job{
		ID:      id,
		command: cmd,
		args:    args,
		outBuf:  &lockedBuffer{b: new(bytes.Buffer)},
		readers: make(map[*streamingReader]chan struct{}),
		done:    make(chan struct{}),
		cgroup:  cg,
	}, nil
}

// Start begins execution of the job using its own cancellable context.
// It sets up cgroup association and output capturing.
// It spawns a goroutine to monitor job completion and update status accordingly.
func (j *job) start(ctx context.Context) error {
	jobContext, cancel := context.WithCancel(ctx)
	j.cancel = cancel

	fd, err := j.cgroup.openFD()
	if err != nil {
		return fmt.Errorf("open cgroup FD: %w", err)
	}
	defer unix.Close(fd)

	cmd := exec.CommandContext(jobContext, j.command, j.args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CgroupFD:    fd,
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
		// The only jobContext can err is when stop() function calls cancel()
		if jobContext.Err() != nil {
			j.status = stopped
		} else if err == nil {
			j.status = exited
		} else {
			j.status = failed
		}

		if err := j.cgroup.delete(); err != nil {
			j.cleanupErr = err
		}

		close(j.done)

		j.mu.Unlock()

	}()

	return nil
}

// stop terminates a running job gracefully by sending a cancellation signal.
func (j *job) stop() error {
	j.mu.Lock()

	if j.status != running {
		j.mu.Unlock()
		return fmt.Errorf("job %s not running", j.ID)
	}
	j.mu.Unlock()

	j.cancel()

	<-j.done

	return nil
}

// statusSnapshot returns a  snapshot of job status.
func (j *job) statusSnapshot() (status, int, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.status, j.exitCode, errors.Join(j.exitErr, j.cleanupErr)
}

// Stream creates a new reader for consuming job output from the beginning.
// If the job has already completed, it returns a reader over the complete output.
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

// Write writes data to the job's output buffer and notifies readers about any new data.
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

// Read reads data from the job's output buffer, blocking until new data is available or the job is done.
// Read must be closed when no longer needed.
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

// Close unregisters the reader from the job and releases associated resources.
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
