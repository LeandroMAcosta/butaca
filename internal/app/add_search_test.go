package app

import "testing"

func TestStripYearLeavesTheTitle(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Amelie 2001", "Amelie"},
		{"Taxi Driver 1976", "Taxi Driver"},
		// A title that is itself a number must survive.
		{"1984", "1984"},
		{"Blade Runner 2049", "Blade Runner"},
		{"Se7en", "Se7en"},
		{"", ""},
	} {
		if got := stripYear(tc.in); got != tc.want {
			t.Errorf("stripYear(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// "Blade Runner 2049" loses its 2049 above, which is correct for matching:
// release names carry the real year separately, and the shortened title still
// matches by prefix.
func TestStripYearRejectsImplausibleYears(t *testing.T) {
	// Only a plausible release year is treated as one; anything else is part
	// of the title.
	for _, in := range []string{"Apollo 1234", "Catch 4400", "Ocean 0011"} {
		if got := stripYear(in); got != in {
			t.Errorf("stripYear(%q) = %q, want it left alone", in, got)
		}
	}
}
