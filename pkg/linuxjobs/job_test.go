package linuxjobs

import (
	"bytes"
	"errors"
	"io"
	"os/exec"
	"testing"
)

// newTestJob is a small helper to avoid repeating boilerplate.
func newTestJob() *job {
	j, _ := newJob("job-1", "echo", "hi")
	return j
}

type fakeCGroup struct {
	deleteCalled bool
	deleteErr    error
}

func (f *fakeCGroup) delete() error {
	f.deleteCalled = true
	return f.deleteErr
}

func (f *fakeCGroup) openFD() (int, error) {
	return 0, nil
}

func TestNewJob_InitialState(t *testing.T) {
	j := newTestJob()

	if j.ID != "job-1" {
		t.Fatalf("expected ID=job-1, got %q", j.ID)
	}
	if j.command != "echo" {
		t.Fatalf("expected command=echo, got %q", j.command)
	}
	if len(j.args) != 1 || j.args[0] != "hi" {
		t.Fatalf("unexpected args: %#v", j.args)
	}
	if j.outBuf == nil {
		t.Fatalf("outBuf must be initialized")
	}
	if j.readers == nil {
		t.Fatalf("readers map must be initialized")
	}
	if j.done == nil {
		t.Fatalf("done channel must be initialized")
	}
	if j.status != unknown {
		t.Fatalf("initial status must be unknown, got %v", j.status)
	}
}

func TestJobStop_HappyPath(t *testing.T) {
	j := &job{
		status: running,
		done:   make(chan struct{}),
	}

	// set a fake cancel that just closes done after being called
	j.cancel = func() {
		close(j.done)
	}

	err := j.stop()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if j.status != running {
		t.Fatalf("stop() should NOT modify status; got %v", j.status)
	}
}

func TestStatusSnapshot_ReturnsCopy(t *testing.T) {
	j := newTestJob()

	j.status = exited
	j.exitCode = 42
	j.exitErr = errors.New("boom")

	s, code, err := j.statusSnapshot()
	if s != exited {
		t.Fatalf("expected status exited, got %v", s)
	}
	if code != 42 {
		t.Fatalf("expected exitCode 42, got %d", code)
	}
	if err == nil || err.Error() != "boom" {
		t.Fatalf("unexpected exitErr: %v", err)
	}
}

func TestLockedBuffer_WriteAndBytes(t *testing.T) {
	lb := lockedBuffer{b: new(bytes.Buffer)}
	n, err := lb.write([]byte("hello"))
	if err != nil {
		t.Fatalf("write error: %v", err)
	}
	if n != 5 {
		t.Fatalf("expected 5 bytes written, got %d", n)
	}

	data1 := lb.bytes()
	if !bytes.Equal(data1, []byte("hello")) {
		t.Fatalf("unexpected buffer: %q", data1)
	}

	// Ensure bytes returns a copy, not the underlying slice.
	data1[0] = 'H'
	data2 := lb.bytes()
	if !bytes.Equal(data2, []byte("hello")) {
		t.Fatalf("buffer should not be affected by external modification, got %q", data2)
	}
}

func TestExitCodeFromErr_Nil(t *testing.T) {
	if code := exitCodeFromErr(nil); code != 0 {
		t.Fatalf("expected 0 for nil error, got %d", code)
	}
}

func TestExitCodeFromErr_ExitError(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 7")
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-nil error from failing command")
	}
	code := exitCodeFromErr(err)
	if code != 7 {
		t.Fatalf("expected exit code 7, got %d", code)
	}
}

func TestStreamingReader_ReadsAllDataAndEOF(t *testing.T) {
	j := newTestJob()
	j.outBuf = &lockedBuffer{
		b: bytes.NewBufferString("hello"),
		n: len("hello"),
	}
	j.done = make(chan struct{})
	close(j.done) // simulate finished job

	r := &streamingReader{
		job:     j,
		offset:  0,
		newData: make(chan struct{}, 1),
	}

	buf := make([]byte, 10)
	n, err := r.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error on first read: %v", err)
	}
	if string(buf[:n]) != "hello" {
		t.Fatalf("expected 'hello', got %q", buf[:n])
	}

	n, err = r.Read(buf)
	if err != io.EOF {
		t.Fatalf("expected EOF, got n=%d err=%v", n, err)
	}
}

func TestStreamingReader_PartialReads(t *testing.T) {
	j := newTestJob()
	j.outBuf = &lockedBuffer{
		b: bytes.NewBufferString("hellodef"),
		n: len("hellodef"),
	}
	j.done = make(chan struct{})
	close(j.done)

	r := &streamingReader{
		job:     j,
		offset:  0,
		newData: make(chan struct{}, 1),
	}

	buf := make([]byte, 4)

	n, err := r.Read(buf)
	if err != nil || string(buf[:n]) != "hell" {
		t.Fatalf("first read: n=%d err=%v data=%q", n, err, buf[:n])
	}

	n, err = r.Read(buf)
	if err != nil || string(buf[:n]) != "odef" {
		t.Fatalf("second read: n=%d err=%v data=%q", n, err, buf[:n])
	}

	n, err = r.Read(buf)
	if err != io.EOF {
		t.Fatalf("expected EOF after full consumption, got n=%d err=%v", n, err)
	}
}

func TestStreamingReader_CloseRemovesReader(t *testing.T) {
	j := newTestJob()
	j.outBuf = &lockedBuffer{
		b: bytes.NewBufferString("data"),
		n: len("data"),
	}
	j.done = make(chan struct{})

	r := j.stream().(*streamingReader)

	if len(j.readers) != 1 {
		t.Fatalf("expected 1 reader, got %d", len(j.readers))
	}

	if err := r.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	if len(j.readers) != 0 {
		t.Fatalf("expected readers map to be empty after Close, got %d", len(j.readers))
	}
}

func TestNotifyingWriter_WritesAndNotifies(t *testing.T) {
	j := newTestJob()
	j.outBuf = &lockedBuffer{
		b: new(bytes.Buffer),
		n: 0,
	}
	j.readers = make(map[*streamingReader]chan struct{})

	ch := make(chan struct{}, 1)
	reader := &streamingReader{
		job:     j,
		offset:  0,
		newData: ch,
	}

	j.readers[reader] = ch

	w := &notifyingWriter{job: j}
	n, err := w.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("write error: %v", err)
	}
	if n != 5 {
		t.Fatalf("expected to write 5 bytes, wrote %d", n)
	}

	// Verify data is in buffer
	data := j.outBuf.bytes()
	if string(data) != "hello" {
		t.Fatalf("unexpected buffer data: %q", data)
	}

	// Verify reader is notified
	select {
	case <-ch:
		// ok
	default:
		t.Fatalf("expected reader notification on newData channel")
	}
}

func TestStream_ReturnsStaticReaderForCompletedJob(t *testing.T) {
	j := newTestJob()
	j.outBuf = &lockedBuffer{
		b: bytes.NewBufferString("final"),
		n: len("final"),
	}
	j.status = exited

	rc := j.stream()
	defer rc.Close()

	buf := make([]byte, 10)
	n, err := rc.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("unexpected error from static reader: %v", err)
	}
	if string(buf[:n]) != "final" {
		t.Fatalf("expected 'final', got %q", buf[:n])
	}
}
