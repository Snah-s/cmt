package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/Snah-s/cmt/internal/commit"
)

type inlineSelectorModel struct {
	form     *huh.Form
	owner    *commitForm
	selector *huh.Select[string]
	step     string
	answers  []string
	done     bool
}

func (m *inlineSelectorModel) Init() tea.Cmd { return m.form.Init() }

func (m *inlineSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if press, ok := msg.(tea.KeyPressMsg); ok && press.String() == "esc" {
		if !m.selector.GetFiltering() {
			m.form.State = huh.StateAborted
			return m, tea.Quit
		}
	}
	if press, ok := msg.(tea.KeyPressMsg); ok && press.String() == "enter" {
		_, _ = m.selector.Update(msg)
		if m.selector.Error() == nil {
			return m, m.choose()
		}
		return m, nil
	}
	_, cmd := m.form.Update(msg)
	if _, ok := msg.(tea.KeyPressMsg); ok && m.step == "emoji" {
		// shortcut: repaint the inline list until incremental rendering handles differing emoji widths.
		return m, tea.Batch(cmd, tea.ClearScreen)
	}
	return m, cmd
}

func (m *inlineSelectorModel) View() tea.View {
	if m.done || m.form.State != huh.StateNormal {
		return tea.NewView("")
	}
	content := strings.Join(m.answers, "\n")
	if content != "" {
		content += "\n"
	}
	content += m.selector.View()
	if err := m.selector.Error(); err != nil {
		content += "\n" + err.Error()
	}
	view := tea.NewView(content)
	view.ReportFocus = true
	return view
}

func (m *inlineSelectorModel) answer() string {
	switch m.step {
	case "format":
		return "? Choose a format: " + m.owner.draft.Mode
	case "type":
		return "? Choose a commit type: " + m.owner.draft.Type
	case "emoji":
		if entry, ok := m.owner.catalog.Find(m.owner.draft.Emoji); ok {
			return "? Choose an emoji: " + entry.Emoji + " - " + entry.Description
		}
	case "scope":
		if m.owner.draft.Scope == "" {
			return "? Choose a scope (optional): Skip scope"
		}
		return "? Choose a scope (optional): " + m.owner.draft.Scope
	}
	return ""
}

func (m *commitForm) inlineSelector(input io.Reader, output io.Writer) *inlineSelectorModel {
	selector := &inlineSelectorModel{owner: m}
	// Huh otherwise rebuilds the theme for each option on every render.
	light, dark := huh.ThemeCharm(false), huh.ThemeCharm(true)
	theme := huh.ThemeFunc(func(isDark bool) *huh.Styles {
		if isDark {
			return dark
		}
		return light
	})
	selector.selector = huh.NewSelect[string]()
	selector.selector.WithTheme(theme)
	selector.selector.Validate(selector.validate)
	selector.form = huh.NewForm(huh.NewGroup(selector.selector)).WithInput(input).WithOutput(output).
		WithShowHelp(false).WithTheme(theme)
	selector.form.SubmitCmd, selector.form.CancelCmd = tea.Quit, tea.Quit
	selector.configure(selector.nextStep(""))
	return selector
}

func (m *inlineSelectorModel) validate(value string) error {
	switch m.step {
	case "format":
		if slices.Contains([]string{"standard", "emoji", "hybrid"}, value) {
			return nil
		}
	case "type":
		return m.owner.validateType(value)
	case "emoji":
		if _, ok := m.owner.catalog.Find(value); ok &&
			(m.owner.draft.Mode != "hybrid" || m.owner.catalog.Compatible(m.owner.draft.Type, value)) {
			return nil
		}
		return errors.New("choose a compatible emoji")
	case "scope":
		if slices.Contains(m.owner.scopeChoices(), value) || value == "" {
			return commit.ValidateScope(value)
		}
	}
	return errors.New("choose one of the listed options")
}

func (m *inlineSelectorModel) nextStep(after string) string {
	switch after {
	case "":
		if !m.owner.fixedMode {
			return "format"
		}
		return m.modeStep()
	case "format":
		return m.modeStep()
	case "type":
		if m.owner.draft.Mode == "hybrid" {
			return "emoji"
		}
		return m.scopeStep()
	case "emoji":
		return m.scopeStep()
	case "scope":
		return ""
	default:
		return ""
	}
}

func (m *inlineSelectorModel) modeStep() string {
	if m.owner.draft.Mode != "emoji" {
		return "type"
	}
	return "emoji"
}

func (m *inlineSelectorModel) scopeStep() string {
	if !m.owner.options.HideScope && m.owner.options.Scopes != nil {
		return "scope"
	}
	return ""
}

func (m *inlineSelectorModel) configure(step string) {
	if step == "" {
		m.done = true
		return
	}
	m.step = step
	var title string
	var options []huh.Option[string]
	var value *string
	switch step {
	case "format":
		title, options, value = "Choose a format", huh.NewOptions("standard", "emoji", "hybrid"), &m.owner.draft.Mode
	case "type":
		title, options, value = "Choose a commit type", huh.NewOptions(commit.Types...), &m.owner.draft.Type
	case "emoji":
		title, options, value = "Choose an emoji", m.owner.emojiOptions(), &m.owner.draft.Emoji
	case "scope":
		title, value = "Choose a scope (optional)", &m.owner.draft.Scope
		options = append(options, huh.NewOption("Skip scope", ""))
		for _, scope := range m.owner.scopeChoices() {
			options = append(options, huh.NewOption(scope, scope))
		}
	}
	if len(options) > 0 {
		options[0] = options[0].Selected(true)
	}
	var temporary string
	m.selector.Value(&temporary).Title(title).Options(options...).Height(1 + min(5, len(options)))
	m.selector.Value(value).Validate(m.validate).Focus()
}

func (m *inlineSelectorModel) choose() tea.Cmd {
	if answer := m.answer(); answer != "" {
		m.answers = append(m.answers, answer)
	}
	m.selector.Blur()
	if m.step == "format" || m.step == "type" {
		reconcile(&m.owner.draft, m.owner.catalog)
	}
	next := m.nextStep(m.step)
	if next == "" {
		m.done = true
		return tea.Quit
	}
	m.configure(next)
	return nil
}

func (m *commitForm) inlineFields(ctx context.Context, input io.Reader, output io.Writer) error {
	selector := m.inlineSelector(input, output)
	err := runProgram(ctx, selector, input, output)
	if ctx.Err() != nil || errors.Is(err, tea.ErrInterrupted) || selector.form.State == huh.StateAborted {
		return ErrCanceled
	}
	if err != nil {
		return err
	}
	if len(selector.answers) > 0 {
		if _, err := fmt.Fprintln(output, strings.Join(selector.answers, "\n")); err != nil {
			return err
		}
	}
	return runAccessible(ctx, input, func(reader io.Reader) error {
		return m.textFields(&lineReader{reader: reader}, output)
	})
}
