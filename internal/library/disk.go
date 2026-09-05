package library

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// DiskUsage describes the filesystem a media folder lives on.
type DiskUsage struct {
	Path  string
	Total int64
	Free  int64
	Used  int64
}

func (d DiskUsage) UsedPercent() float64 {
	if d.Total == 0 {
		return 0
	}
	return float64(d.Used) / float64(d.Total) * 100
}

// Usage reports the filesystem statistics for path, walking up to the nearest
// existing ancestor so it works before the folder has been created.
func Usage(path string) (DiskUsage, error) {
	probe := path
	for {
		if _, err := os.Stat(probe); err == nil {
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return DiskUsage{Path: path}, fmt.Errorf("no existing ancestor for %s", path)
		}
		probe = parent
	}

	var st syscall.Statfs_t
	if err := syscall.Statfs(probe, &st); err != nil {
		return DiskUsage{Path: path}, fmt.Errorf("statfs %s: %w", probe, err)
	}
	blockSize := int64(st.Bsize)
	total := int64(st.Blocks) * blockSize
	// Bavail, not Bfree: reserved blocks are not usable by an unprivileged
	// process, so counting them would overstate what is actually available.
	free := int64(st.Bavail) * blockSize
	return DiskUsage{Path: path, Total: total, Free: free, Used: total - free}, nil
}

// HumanSize renders a byte count in binary units.
func HumanSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
