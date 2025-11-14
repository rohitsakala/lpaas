package linuxjobs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

var (
	cgroupRootPath  = "/sys/fs/cgroup"
	lpaasCgroupRoot = "/sys/fs/cgroup/lpaas"
)

const (
	defaultCPUPercent = 50                     // 50% of one CPU
	defaultMemBytes   = 1 * 1024 * 1024 * 1024 // 1 GB
	defaultIOBps      = 10 * 1024 * 1024       // 10 MB/s
	cpuMaxFile        = "cpu.max"
	memoryMaxFile     = "memory.max"
	ioMaxFile         = "io.max"
	cgroupKillFile    = "cgroup.kill"
)

// cgroupv2 represents a single job’s cgroup.
type cgroupv2 struct {
	Path string // full path: /sys/fs/cgroup/lpaas/<jobID>
}

// newCGroupV2 creates the directory for a job’s cgroup.
func newCGroupV2(jobID string) (*cgroupv2, error) {
	path := filepath.Join(lpaasCgroupRoot, jobID)

	if err := os.MkdirAll(path, 0o755); err != nil {
		return nil, fmt.Errorf("create job cgroup %q: %w", path, err)
	}

	return &cgroupv2{Path: path}, nil
}

// enableControllers activates cpu, memory, and io controllers for children under dir.
func enableControllers(dir string) error {
	controllers := []string{"cpu", "memory", "io"}
	subtree := filepath.Join(dir, "cgroup.subtree_control")

	for _, ctrl := range controllers {
		line := []byte("+" + ctrl + "\n")

		if err := os.WriteFile(subtree, line, 0o644); err != nil {
			return fmt.Errorf("enable controller %q at %q: %w", ctrl, subtree, err)
		}
	}

	return nil
}

// setLimits applies CPU, memory, and I/O throttling to this job.
func (cg *cgroupv2) setLimits() error {
	cpuPath := filepath.Join(cg.Path, cpuMaxFile)
	cpuLine := fmt.Sprintf("%d 100000", defaultCPUPercent*1000)

	if err := os.WriteFile(cpuPath, []byte(cpuLine), 0o644); err != nil {
		return fmt.Errorf("write cpu.max for %q: %w", cg.Path, err)
	}

	memPath := filepath.Join(cg.Path, memoryMaxFile)
	memLine := fmt.Sprintf("%d", defaultMemBytes)

	if err := os.WriteFile(memPath, []byte(memLine), 0o644); err != nil {
		return fmt.Errorf("write memory.max for %q: %w", cg.Path, err)
	}

	device, err := getRootBlockDevice()
	if err != nil {
		return fmt.Errorf("cannot determine root block device for io.max: %w", err)
	}

	ioPath := filepath.Join(cg.Path, ioMaxFile)
	ioLine := fmt.Sprintf("%s rbps=%d wbps=%d\n", device, defaultIOBps, defaultIOBps)

	if err := os.WriteFile(ioPath, []byte(ioLine), 0o644); err != nil {
		return fmt.Errorf("write io.max for %q: %w", cg.Path, err)
	}

	return nil
}

// getRootBlockDevice returns major:minor of block device backing "/".
func getRootBlockDevice() (string, error) {
	cmd := exec.Command("findmnt", "-no", "SOURCE", "/")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("findmnt failed: %w", err)
	}

	dev := strings.TrimSpace(string(out))
	if dev == "" {
		return "", fmt.Errorf("empty device from findmnt")
	}

	base := dev
	if strings.HasPrefix(dev, "/dev/") {
		base = strings.TrimRightFunc(dev, func(r rune) bool {
			return r >= '0' && r <= '9'
		})
	}

	st, err := os.Stat(base)
	if err != nil {
		return "", fmt.Errorf("stat failed for %q: %w", base, err)
	}

	stat, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("unexpected stat type for %q", base)
	}

	major := unix.Major(stat.Rdev)
	minor := unix.Minor(stat.Rdev)

	return fmt.Sprintf("%d:%d", major, minor), nil
}

// openFD opens the cgroup directory and returns its FD.
func (cg *cgroupv2) openFD() (int, error) {
	fd, err := unix.Open(cg.Path, unix.O_DIRECTORY|unix.O_RDONLY, 0)
	if err != nil {
		return -1, fmt.Errorf("open cgroup fd for %q: %w", cg.Path, err)
	}
	return fd, nil
}

// delete removes this cgroup by writing "1" to cgroup.kill and polling until
// the kernel deletes the directory. A missing cgroup.kill file is
// treated as normal because the kernel may remove the cgroup immediately.
func (cg *cgroupv2) delete() error {
	killPath := filepath.Join(cg.Path, cgroupKillFile)

	if err := os.WriteFile(killPath, []byte("1\n"), 0644); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("write cgroup.kill: %w", err)
	}

	timeout := time.After(1 * time.Second)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout deleting cgroup %q", cg.Path)
		case <-tick.C:
			err := os.RemoveAll(cg.Path)
			if err == nil || os.IsNotExist(err) {
				return nil
			}
		}
	}
}
