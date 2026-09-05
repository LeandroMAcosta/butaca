package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/LeandroMAcosta/butaca/internal/store"
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true)
	labelStyle  = lipgloss.NewStyle().Bold(true)
	dimStyle    = lipgloss.NewStyle().Faint(true)
	warnStyle   = lipgloss.NewStyle().Bold(true)
	okStyle     = lipgloss.NewStyle()
	rowSelected = lipgloss.NewStyle().Bold(true)
	tabActive   = lipgloss.NewStyle().Bold(true).Underline(true)
	tabIdle     = lipgloss.NewStyle().Faint(true)
	headerStyle = lipgloss.NewStyle().Faint(true).Bold(true)
)

func headerRow(s string) string { return headerStyle.Render(s) + "\n" }

func cursorFor(sel bool) string {
	if sel {
		return "> "
	}
	return "  "
}

func render(sel bool, s string) string {
	if sel {
		return rowSelected.Render(s)
	}
	return s
}

// progressBar draws a transfer's completion, so the queue reads at a glance.
func progressBar(fraction float64, width int) string {
	if width < 4 {
		width = 4
	}
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}
	filled := int(fraction * float64(width))
	return dimStyle.Render("[" + strings.Repeat("=", filled) + strings.Repeat(" ", width-filled) + "]")
}

// visible returns the slice of rows that fits the terminal, scrolled to keep
// the cursor on screen.
func (b *browser) visible(rows []*store.Item) []*store.Item {
	h := b.pageSize()
	if len(rows) <= h {
		return rows
	}
	start := b.scrollOffset(len(rows))
	end := start + h
	if end > len(rows) {
		end = len(rows)
	}
	return rows[start:end]
}

func (b *browser) pageSize() int {
	h := b.height - 9 // header, tabs, footer and status lines
	if h < 5 {
		h = 5
	}
	return h
}

func (b *browser) scrollOffset(total int) int {
	h := b.pageSize()
	if total <= h {
		return 0
	}
	cur := b.cursor[b.tab]
	start := cur - h/2
	if start < 0 {
		start = 0
	}
	if start > total-h {
		start = total - h
	}
	return start
}

func truncate(s string, n int) string {
	if n <= 1 {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
