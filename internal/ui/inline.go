package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode"

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
	emojis   *emojiSelector
	custom   *promptModel
	width    int
	height   int
	input    io.Reader
	output   io.Writer
	done     bool
}

func (m *inlineSelectorModel) Init() tea.Cmd {
	if m.step == "emoji" {
		return tea.RequestWindowSize
	}
	return m.form.Init()
}

func (m *inlineSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = size.Width, size.Height
	}
	if m.step == "customScope" {
		return m, m.updateCustomScope(msg)
	}
	if m.step == "emoji" {
		model, cmd := m.updateEmoji(msg)
		switch msg.(type) {
		case tea.KeyPressMsg, tea.PasteMsg:
			// Legacy terminals disagree on emoji widths; repaint until incremental redraw passes all width modes.
			return model, tea.Batch(cmd, tea.ClearScreen)
		}
		return model, cmd
	}
	if press, ok := msg.(tea.KeyPressMsg); ok && press.String() == "esc" && m.step == "scope" && m.selector.GetFiltering() {
		// Huh keeps an applied filter after Esc; a new selector brings the whole list back.
		m.selector, m.owner.draft.Scope = nil, ""
		m.configure("scope")
		return m, m.form.Init()
	}
	if press, ok := msg.(tea.KeyPressMsg); ok && press.String() == "esc" {
		if !m.selector.GetFiltering() {
			m.form.State = huh.StateAborted
			return m, tea.Quit
		}
	}
	if press, ok := msg.(tea.KeyPressMsg); ok && press.String() == "!" && m.step == "type" && !m.selector.GetFiltering() {
		// Rebuilding the step keeps the cursor on the hovered type and shows the mark on every option.
		if kind, ok := m.selector.Hovered(); ok {
			m.owner.draft.Type = kind
		}
		m.owner.draft.Breaking = !m.owner.draft.Breaking
		m.configure("type")
		return m, nil
	}
	if press, ok := msg.(tea.KeyPressMsg); ok && m.step == "scope" && !m.selector.GetFiltering() && press.Text != "" && press.Text != "/" && unicode.IsPrint([]rune(press.Text)[0]) {
		// Typing filters the scope list directly, like the emoji selector; the title stays until then.
		// "/" opens Huh's filter with its own key handling; Filtering(true) would leave Esc disabled.
		m.selector.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	}
	if press, ok := msg.(tea.KeyPressMsg); ok && press.String() == "enter" {
		if _, ok := m.selector.Hovered(); !ok {
			// A filter without matches must not fall back to the previous choice (Skip scope).
			return m, nil
		}
		_, _ = m.selector.Update(msg)
		if m.selector.Error() == nil {
			return m, m.choose()
		}
		return m, nil
	}
	_, cmd := m.form.Update(msg)
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
	switch m.step {
	case "emoji":
		content += m.emojis.view(m.width, m.height-len(m.answers))
	case "customScope":
		content += m.custom.View().Content
	default:
		content += m.selector.View()
		if err := m.selector.Error(); err != nil {
			content += "\n" + err.Error()
		}
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
		if m.owner.draft.Breaking {
			return "? Choose a commit type: " + m.owner.draft.Type + "! (breaking change)"
		}
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
	selector := &inlineSelectorModel{owner: m, input: input, output: output, form: &huh.Form{}}
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
		if value == customScope && m.owner.options.Scopes == nil {
			return nil
		}
		if value == "" || slices.ContainsFunc(m.owner.scopeChoices(), func(c scopeOption) bool { return c.name == value }) {
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
	if !m.owner.options.HideScope {
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
	if step == "emoji" {
		m.emojis = newEmojiSelector(m.owner)
		return
	}
	if m.selector == nil {
		theme := charmTheme()
		m.selector = huh.NewSelect[string]().Validate(m.validate)
		m.selector.WithTheme(theme)
		m.form = huh.NewForm(huh.NewGroup(m.selector)).WithInput(m.input).WithOutput(m.output).
			WithShowHelp(false).WithTheme(theme)
		m.form.SubmitCmd, m.form.CancelCmd = tea.Quit, tea.Quit
	}
	var title, description string
	var options []huh.Option[string]
	var value *string
	switch step {
	case "format":
		title, options, value = "Choose a format", huh.NewOptions("standard", "emoji", "hybrid"), &m.owner.draft.Mode
	case "type":
		title, value = "Choose a commit type", &m.owner.draft.Type
		description = "!: mark a breaking change"
		mark := ""
		if m.owner.draft.Breaking {
			description, mark = "!: breaking change marked", "!"
		}
		for _, kind := range commit.Types {
			options = append(options, huh.NewOption(fmt.Sprintf("%-9s %s", kind+mark, typeDescriptions[kind]), kind))
		}
	case "scope":
		// Skip comes first so Enter keeps the optional scope empty unless one was received.
		title, value = "Choose a scope (optional)", &m.owner.draft.Scope
		options = append(options, huh.NewOption("Skip scope", ""))
		for _, choice := range m.owner.scopeChoices() {
			label := choice.name
			if choice.note != "" {
				label = fmt.Sprintf("%-12s %s", choice.name, choice.note)
			}
			options = append(options, huh.NewOption(label, choice.name))
		}
		if m.owner.options.Scopes == nil {
			options = append(options, huh.NewOption("Custom scope…", customScope))
		}
	}
	if len(options) > 0 {
		options[0] = options[0].Selected(true)
	}
	var temporary string
	m.selector.Value(&temporary).Title(title).Description(description).Options(options...).Height(1 + min(5, len(options)))
	m.selector.Value(value).Validate(m.validate).Focus()
	if m.width > 0 {
		_, _ = m.form.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	}
}

func (m *inlineSelectorModel) choose() tea.Cmd {
	if m.step == "scope" && m.owner.draft.Scope == customScope {
		m.owner.draft.Scope, m.step = "", "customScope"
		m.custom = newPrompt("Scope", "", func(value string) error {
			if value == "" {
				return errors.New("enter a scope, or press Esc to go back to the list")
			}
			return commit.ValidateScope(value)
		})
		if m.width > 0 {
			m.custom.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		}
		return nil
	}
	if answer := m.answer(); answer != "" {
		m.answers = append(m.answers, answer)
	}
	hadSelector := m.selector != nil
	if hadSelector {
		m.selector.Blur()
	}
	if m.step == "format" || m.step == "type" {
		reconcile(&m.owner.draft, m.owner.catalog)
	}
	next := m.nextStep(m.step)
	if next == "" {
		m.done = true
		return tea.Quit
	}
	m.configure(next)
	if !hadSelector && m.selector != nil {
		return m.form.Init()
	}
	return nil
}

// updateCustomScope runs the "Custom scope…" field inside this program: Esc returns to the list,
// Ctrl+C cancels the message, and the field's own Quit is ignored.
func (m *inlineSelectorModel) updateCustomScope(msg tea.Msg) tea.Cmd {
	if press, ok := msg.(tea.KeyPressMsg); ok && press.String() == "ctrl+c" {
		m.form.State = huh.StateAborted
		return tea.Quit
	}
	m.custom.Update(msg)
	switch {
	case m.custom.done:
		m.owner.draft.Scope, m.step = m.custom.value(), "scope"
		m.custom = nil
		return m.choose()
	case m.custom.aborted || m.custom.eof:
		m.custom = nil
		m.configure("scope")
	}
	return nil
}

// typeDescriptions follow @commitlint/config-conventional; style means code formatting, not CSS.
var typeDescriptions = map[string]string{
	"feat":     "A new feature",
	"fix":      "A bug fix",
	"docs":     "Documentation only changes",
	"refactor": "Code change that neither fixes a bug nor adds a feature",
	"test":     "Adding or correcting tests",
	"chore":    "Other changes that don't touch source or tests",
	"perf":     "A code change that improves performance",
	"style":    "Formatting only, no change in meaning",
	"build":    "Build system or external dependencies",
	"ci":       "CI configuration and scripts",
	"revert":   "Reverts a previous commit",
}

// charmTheme caches both variants: Huh otherwise rebuilds the theme for each option on every render.
func charmTheme() huh.ThemeFunc {
	light, dark := huh.ThemeCharm(false), huh.ThemeCharm(true)
	return func(isDark bool) *huh.Styles {
		if isDark {
			return dark
		}
		return light
	}
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
	// The title and body prompts read keys in raw mode: a live counter and Ctrl+O need them.
	if err := m.titleField(ctx, input, output); err != nil {
		return err
	}
	if !m.options.HideBody {
		if err := m.bodyField(ctx, input, output); err != nil {
			return err
		}
	}
	return runAccessible(ctx, input, func(reader io.Reader) error {
		return m.confirmFields(&lineReader{reader: reader}, output)
	})
}
