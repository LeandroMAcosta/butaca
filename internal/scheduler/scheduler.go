// Package scheduler runs butaca's background work: importing finished
// downloads, searching for what is still missing, and filling subtitle gaps.
package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/LeandroMAcosta/butaca/internal/app"
)

type Intervals struct {
	Import    time.Duration
	Search    time.Duration
	Subtitles time.Duration
}

func DefaultIntervals() Intervals {
	return Intervals{
		// Downloads finish on their own schedule; a short poll keeps the gap
		// between "complete" and "in the library" small.
		Import: time.Minute,
		// Searching is expensive and fans out across every tracker.
		Search:    6 * time.Hour,
		Subtitles: 12 * time.Hour,
	}
}

type Scheduler struct {
	app  *app.App
	log  *slog.Logger
	ivl  Intervals
	once sync.Once
}

func New(a *app.App, log *slog.Logger, ivl Intervals) *Scheduler {
	return &Scheduler{app: a, log: log, ivl: ivl}
}

// Run blocks until ctx is cancelled. Each job runs on its own ticker; a slow
// search never delays an import.
func (s *Scheduler) Run(ctx context.Context) error {
	jobs := []struct {
		name    string
		every   time.Duration
		fn      func(context.Context)
		atStart bool
	}{
		{"import", s.ivl.Import, s.runImport, true},
		{"search", s.ivl.Search, s.runSearch, false},
		{"subtitles", s.ivl.Subtitles, s.runSubtitles, false},
	}

	var wg sync.WaitGroup
	for _, j := range jobs {
		if j.every <= 0 {
			s.log.Info("job disabled", "job", j.name)
			continue
		}
		wg.Add(1)
		go func(name string, every time.Duration, fn func(context.Context), atStart bool) {
			defer wg.Done()
			if atStart {
				fn(ctx)
			}
			t := time.NewTicker(every)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					fn(ctx)
				}
			}
		}(j.name, j.every, j.fn, j.atStart)
	}

	s.log.Info("butaca running",
		"import_every", s.ivl.Import, "search_every", s.ivl.Search, "subtitles_every", s.ivl.Subtitles)
	<-ctx.Done()
	wg.Wait()
	s.log.Info("stopped")
	return nil
}

func (s *Scheduler) runImport(ctx context.Context) {
	lines, err := s.app.ImportReady(ctx)
	if err != nil {
		s.log.Warn("import failed", "err", err)
		return
	}
	for _, l := range lines {
		s.log.Info("imported", "detail", l)
	}
}

func (s *Scheduler) runSearch(ctx context.Context) {
	found, err := s.app.SearchMissing(ctx)
	if err != nil {
		s.log.Warn("search failed", "err", err)
		return
	}
	for _, l := range found {
		s.log.Info("grabbed", "detail", l)
	}
}

func (s *Scheduler) runSubtitles(ctx context.Context) {
	got, err := s.app.FillSubtitleGaps(ctx)
	if err != nil {
		s.log.Warn("subtitle sweep failed", "err", err)
		return
	}
	for _, l := range got {
		s.log.Info("subtitles", "detail", l)
	}
}
