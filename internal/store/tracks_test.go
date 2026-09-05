package store

import "testing"

func TestSummaryTruncatesLongLists(t *testing.T) {
	// Disclosure Day really does carry 44 subtitle languages.
	many := make([]string, 44)
	for i := range many {
		many[i] = "l" + itoa(i)
	}
	got := Summary(many, 3)
	if got != "l0 l1 l2 +41" {
		t.Errorf("Summary = %q", got)
	}
}

func TestSummaryShortListIsVerbatim(t *testing.T) {
	if got := Summary([]string{"es", "en"}, 3); got != "es en" {
		t.Errorf("Summary = %q, want %q", got, "es en")
	}
}

func TestSummaryEmpty(t *testing.T) {
	if got := Summary(nil, 3); got != "-" {
		t.Errorf("Summary = %q, want -", got)
	}
}
