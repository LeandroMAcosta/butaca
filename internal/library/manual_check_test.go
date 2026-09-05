package library

import (
	"os"
	"testing"
)

// Prints the same numbers the TUI header will show, for eyeballing against df.
func TestReportHomeMoviesUsage(t *testing.T) {
	if os.Getenv("BUTACA_IT") == "" {
		t.Skip("set BUTACA_IT=1 to print real disk usage")
	}
	home, _ := os.UserHomeDir()
	u, err := Usage(home + "/Movies")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("total=%s free=%s used=%.0f%%", HumanSize(u.Total), HumanSize(u.Free), u.UsedPercent())
}
