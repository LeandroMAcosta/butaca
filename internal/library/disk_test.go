package library

import (
	"path/filepath"
	"testing"
)

func TestUsageReportsPlausibleNumbers(t *testing.T) {
	u, err := Usage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if u.Total <= 0 {
		t.Fatalf("total = %d, want a positive size", u.Total)
	}
	if u.Free < 0 || u.Free > u.Total {
		t.Errorf("free = %d is not within 0..%d", u.Free, u.Total)
	}
	if u.Used+u.Free != u.Total {
		t.Errorf("used+free = %d, want total %d", u.Used+u.Free, u.Total)
	}
	if p := u.UsedPercent(); p < 0 || p > 100 {
		t.Errorf("used percent = %f", p)
	}
}

// The TUI header asks for usage before the media folders necessarily exist.
func TestUsageWorksOnMissingPath(t *testing.T) {
	u, err := Usage(filepath.Join(t.TempDir(), "not", "yet", "there"))
	if err != nil {
		t.Fatalf("should walk up to an existing ancestor: %v", err)
	}
	if u.Total <= 0 {
		t.Error("expected the ancestor's filesystem stats")
	}
}

// gigabytes avoids a constant-overflow conversion in the table below.
func gigabytes(n float64) int64 { return int64(n * float64(int64(1)<<30)) }

func TestHumanSize(t *testing.T) {
	for _, tc := range []struct {
		in   int64
		want string
	}{
		{512, "512 B"},
		{2 << 20, "2.0 MB"},
		{gigabytes(78.27), "78.3 GB"},
	} {
		if got := HumanSize(tc.in); got != tc.want {
			t.Errorf("HumanSize(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
