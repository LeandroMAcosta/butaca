package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LeandroMAcosta/butaca/internal/config"
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true)
	helpStyle   = lipgloss.NewStyle().Faint(true)
	cursorStyle = lipgloss.NewStyle().Bold(true)
	chosenStyle = lipgloss.NewStyle().Bold(true)
	footerStyle = lipgloss.NewStyle().Faint(true).MarginTop(1)
	errStyle    = lipgloss.NewStyle().Bold(true)
)

type model struct {
	cfg     *config.Config
	steps   []step
	idx     int
	answers map[string]string
	input   textinput.Model
	done    bool
	aborted bool
	err     error
}

// NewSetup builds the first-run wizard over cfg.
func NewSetup(cfg *config.Config) tea.Model {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.CharLimit = 512

	m := model{cfg: cfg, steps: newSteps(cfg), answers: map[string]string{}, input: ti}
	m.idx = m.nextVisible(-1)
	m.loadStep()
	return m
}

// RunSetup runs the wizard and saves the config unless the user aborts.
func RunSetup(cfg *config.Config) error {
	p := tea.NewProgram(NewSetup(cfg))
	res, err := p.Run()
	if err != nil {
		return err
	}
	m, ok := res.(model)
	if !ok || m.aborted {
		return fmt.Errorf("setup cancelled; nothing was saved")
	}
	if m.err != nil {
		return m.err
	}
	return nil
}

func (m model) Init() tea.Cmd { return textinput.Blink }

// nextVisible returns the next step index whose visibleIf passes, or len(steps).
func (m model) nextVisible(from int) int {
	for i := from + 1; i < len(m.steps); i++ {
		if m.steps[i].visibleIf == nil || m.steps[i].visibleIf(m.answers) {
			return i
		}
	}
	return len(m.steps)
}

func (m model) prevVisible(from int) int {
	for i := from - 1; i >= 0; i-- {
		if m.steps[i].visibleIf == nil || m.steps[i].visibleIf(m.answers) {
			return i
		}
	}
	return -1
}

func (m *model) loadStep() {
	if m.idx >= len(m.steps) {
		return
	}
	s := m.steps[m.idx]
	if s.kind == kindInput {
		m.input.SetValue(s.value)
		m.input.Placeholder = s.placeholder
		m.input.CursorEnd()
		m.input.Focus()
	} else {
		m.input.Blur()
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		m.aborted = true
		return m, tea.Quit
	}

	if m.idx >= len(m.steps) {
		return m, tea.Quit
	}
	cur := &m.steps[m.idx]

	switch key.Type {
	case tea.KeyEnter:
		if cur.kind == kindInput {
			cur.value = strings.TrimSpace(m.input.Value())
			m.answers[cur.key] = cur.value
		} else {
			m.answers[cur.key] = cur.options[cur.cursor].value
		}
		m.idx = m.nextVisible(m.idx)
		if m.idx >= len(m.steps) {
			apply(m.cfg, m.answers)
			m.err = m.cfg.Save()
			m.done = true
			return m, tea.Quit
		}
		m.loadStep()
		return m, textinput.Blink

	case tea.KeyUp:
		if cur.kind == kindChoice && cur.cursor > 0 {
			cur.cursor--
		}
		return m, nil

	case tea.KeyDown:
		if cur.kind == kindChoice && cur.cursor < len(cur.options)-1 {
			cur.cursor++
		}
		return m, nil

	case tea.KeyShiftTab:
		if prev := m.prevVisible(m.idx); prev >= 0 {
			m.idx = prev
			m.loadStep()
		}
		return m, nil
	}

	if cur.kind == kindInput {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) View() string {
	if m.aborted {
		return "Setup cancelled. Nothing was written.\n"
	}
	if m.done {
		return m.summary()
	}
	if m.idx >= len(m.steps) {
		return ""
	}

	s := m.steps[m.idx]
	var b strings.Builder
	fmt.Fprintf(&b, "butaca setup  ·  %d of %d\n\n", m.visibleIndex()+1, m.visibleTotal())
	fmt.Fprintln(&b, titleStyle.Render(s.title))
	if s.help != "" {
		fmt.Fprintln(&b, helpStyle.Render(s.help))
	}
	b.WriteString("\n")

	if s.kind == kindInput {
		fmt.Fprintln(&b, m.input.View())
		b.WriteString(footerStyle.Render("enter continue · shift+tab back · esc cancel"))
		return b.String() + "\n"
	}

	for i, o := range s.options {
		marker := "  "
		label := o.label
		if i == s.cursor {
			marker = cursorStyle.Render("> ")
			label = chosenStyle.Render(o.label)
		}
		fmt.Fprintf(&b, "%s%s\n", marker, label)
		if o.help != "" {
			fmt.Fprintf(&b, "    %s\n", helpStyle.Render(o.help))
		}
	}
	b.WriteString(footerStyle.Render("↑↓ move · enter select · shift+tab back · esc cancel"))
	return b.String() + "\n"
}

func (m model) summary() string {
	var b strings.Builder
	if m.err != nil {
		return errStyle.Render("could not save: "+m.err.Error()) + "\n"
	}
	fmt.Fprintf(&b, "Saved %s\n\n", m.cfg.Path())
	fmt.Fprintf(&b, "  movies      %s\n", m.cfg.Paths.Movies)
	fmt.Fprintf(&b, "  series      %s\n", m.cfg.Paths.TV)
	fmt.Fprintf(&b, "  downloads   %s\n", m.cfg.Paths.Downloads)
	fmt.Fprintf(&b, "  audio       %s\n", m.cfg.Rules.LanguageMode)
	fmt.Fprintf(&b, "  quality     %s\n", strings.Join(m.cfg.Rules.Resolutions, ", "))
	if len(m.cfg.Subtitles.Languages) > 0 {
		fmt.Fprintf(&b, "  subtitles   %s\n", strings.Join(m.cfg.Subtitles.Languages, ", "))
	} else {
		fmt.Fprintf(&b, "  subtitles   off\n")
	}
	fmt.Fprintf(&b, "\nNext: butaca status\n")
	return b.String()
}

func (m model) visibleTotal() int {
	n := 0
	for _, s := range m.steps {
		if s.visibleIf == nil || s.visibleIf(m.answers) {
			n++
		}
	}
	return n
}

func (m model) visibleIndex() int {
	n := 0
	for i, s := range m.steps {
		if i >= m.idx {
			break
		}
		if s.visibleIf == nil || s.visibleIf(m.answers) {
			n++
		}
	}
	return n
}
