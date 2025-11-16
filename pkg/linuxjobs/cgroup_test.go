package linuxjobs

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestNewCGroupV2_CreatesDirectory(t *testing.T) {

	cg, err := newCGroupV2("job1", t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(cg.Path); err != nil {
		t.Fatalf("expected directory created: %v", err)
	}
}

func TestEnableControllers_HappyPath(t *testing.T) {
	tmp := t.TempDir()

	subtree := filepath.Join(tmp, "cgroup.subtree_control")

	// Pre-create the target file
	if err := os.WriteFile(subtree, nil, 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	if err := enableControllers(tmp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expect only final overwrite: "+io\n"
	data, _ := os.ReadFile(subtree)
	want := "+io\n"
	if string(data) != want {
		t.Fatalf("unexpected subtree_control:\n got=%q\nwant=%q", data, want)
	}
}

func TestSetLimits_HappyPath(t *testing.T) {
	cg, err := newCGroupV2("job1", t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, f := range []string{cpuMaxFile, memoryMaxFile, ioMaxFile} {
		if err := os.WriteFile(filepath.Join(cg.Path, f), nil, 0644); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
	}

	if err := cg.setLimits(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if b, _ := os.ReadFile(filepath.Join(cg.Path, cpuMaxFile)); len(b) == 0 {
		t.Fatalf("cpu.max not written")
	}
	if b, _ := os.ReadFile(filepath.Join(cg.Path, memoryMaxFile)); len(b) == 0 {
		t.Fatalf("memory.max not written")
	}
	if b, _ := os.ReadFile(filepath.Join(cg.Path, ioMaxFile)); len(b) == 0 {
		t.Fatalf("io.max not written")
	}
}

func TestSetLimits_WritesFilesEvenIfMissing(t *testing.T) {
	cg, err := newCGroupV2("job1", t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only create memory + io, CPU missing intentionally
	if err := os.WriteFile(filepath.Join(cg.Path, memoryMaxFile), nil, 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cg.Path, ioMaxFile), nil, 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Should succeed because WriteFile creates missing files
	if err := cg.setLimits(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// CPU file must now exist
	if _, err := os.Stat(filepath.Join(cg.Path, cpuMaxFile)); err != nil {
		t.Fatalf("cpu.max should have been created")
	}
}

func TestOpenFD_HappyPath(t *testing.T) {
	tmp := t.TempDir()
	cg := &cgroupv2{Path: tmp}

	fd, err := cg.openFD()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fd < 0 {
		t.Fatalf("expected valid FD, got %d", fd)
	}
	unix.Close(fd)
}

func TestOpenFD_Error(t *testing.T) {
	cg := &cgroupv2{Path: "/nonexistent"}
	if _, err := cg.openFD(); err == nil {
		t.Fatalf("expected error but got none")
	}
}

func TestDelete_HappyPath(t *testing.T) {
	tmp := t.TempDir()
	cg := &cgroupv2{Path: tmp}

	if err := os.WriteFile(filepath.Join(tmp, cgroupKillFile), nil, 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	if err := cg.delete(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatalf("expected directory removed")
	}
}

func TestDelete_IgnoresMissingCgroupKillFile(t *testing.T) {
	tmp := t.TempDir()
	cg := &cgroupv2{Path: tmp}

	// Should succeed even if cgroup.kill file is missing
	if err := cg.delete(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
