package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Snah-s/cmt/internal/commit"
)

// editors orders terminal editors from most approachable to least; vi is the near-universal last resort.
var editors = []string{"nano", "micro", "nvim", "vim", "hx", "vi"}

// FindEditor returns the configured editor if installed, else the first installed default; empty means none.
func FindEditor(preference string) string {
	candidates := editors
	if preference != "" {
		candidates = append([]string{preference}, editors...)
	}
	for _, name := range candidates {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

// promptModel reads one line in raw mode, so keys act at once: the counter follows every
// keystroke and, with an editor, Ctrl+O hands the answer to it. It draws a single
// "? Title [used/max]: answer" line, like the answers printed after each step.
type promptModel struct {
	input    textinput.Model
	title    string
	fallback string
	hint     string
	count    func(value string) (used, available, total int)
	validate func(value string) error
	editor   string
	edit     bool
	done     bool
	aborted  bool
	eof      bool
	err      error
}

const (
	sgrReset  = "\x1b[0m"
	sgrGreen  = "\x1b[32m"
	sgrBold   = "\x1b[1m"
	sgrDim    = "\x1b[2m"
	sgrYellow = "\x1b[33m"
	sgrRed    = "\x1b[31m"
)

func newPrompt(title, fallback string, validate func(string) error) *promptModel {
	m := &promptModel{title: title, fallback: fallback, validate: validate, input: textinput.New()}
	m.input.Prompt, m.input.Placeholder = "", fallback
	// Until the terminal reports its size; with no width textinput shows only one placeholder character.
	m.input.SetWidth(80)
	m.input.Focus()
	return m
}

// value is the typed line, or the received value when nothing was typed.
func (m *promptModel) value() string {
	if m.input.Value() == "" {
		return m.fallback
	}
	return m.input.Value()
}

func newBodyModel(editor string) *promptModel {
	m := newPrompt("Body (optional)", "", commit.ValidateBody)
	if editor != "" {
		m.editor, m.hint = editor, "Ctrl+O: write it in "+filepath.Base(editor)
	}
	return m
}

// newTitlePrompt counts the whole title as it will be written, so the limit holds before Enter.
func (m *commitForm) newTitlePrompt() *promptModel {
	draft := func(description string) commit.Draft {
		d := m.draft.Active()
		d.Description = description
		return d
	}
	prompt := newPrompt("Enter the commit title", m.draft.Description, func(value string) error {
		_, err := commit.Build(draft(value), m.catalog, m.options.MessageOptions)
		return err
	})
	prefix := commit.Preview(draft(""), m.catalog, m.options.MessageOptions).TitleLength
	prompt.count = func(value string) (int, int, int) {
		total := commit.Preview(draft(value), m.catalog, m.options.MessageOptions).TitleLength
		return total - prefix, commit.MaxTitleLength - prefix, total
	}
	return prompt
}

// titleCounter shows used/maximum characters of the whole title, counted as commitlint does.
func titleCounter(count int) string {
	text := fmt.Sprintf("%d/%d characters", count, commit.MaxTitleLength)
	switch {
	case count > commit.MaxTitleLength:
		text += " · too long"
	case count > commit.SoftTitleLength:
		text += fmt.Sprintf(" · over %d, consider shortening", commit.SoftTitleLength)
	}
	return text
}

func (m *promptModel) Init() tea.Cmd { return tea.RequestWindowSize }

func (m *promptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Scroll the answer inside the line instead of wrapping it.
		m.input.SetWidth(max(10, msg.Width-ansi.StringWidth(m.head())-1))
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		// Line-mode prompts accepted \n and Ctrl+D as EOF; raw mode keeps both for scripts and habit.
		case "enter", "ctrl+j":
			if m.err = m.validate(m.value()); m.err == nil {
				m.done = true
				return m, tea.Quit
			}
			return m, nil
		case "ctrl+o":
			if m.editor != "" {
				m.edit = true
				return m, tea.Quit
			}
			return m, nil
		case "esc", "ctrl+c":
			m.aborted = true
			return m, tea.Quit
		case "ctrl+d":
			if m.input.Value() == "" {
				m.eof = true
				return m, tea.Quit
			}
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// head is "? Title [used/max]: " with the counter yellow past the soft limit and red past the maximum.
func (m *promptModel) head() string {
	head := sgrGreen + "?" + sgrReset + " " + sgrBold + m.title
	if m.count != nil {
		used, available, total := m.count(m.value())
		color := ""
		switch {
		case total > commit.MaxTitleLength:
			color = sgrRed
		case total > commit.SoftTitleLength:
			color = sgrYellow
		}
		head += fmt.Sprintf(" %s[%d/%d]%s%s", color, used, available, sgrReset, sgrBold)
	}
	return head + ":" + sgrReset + " "
}

func (m *promptModel) View() tea.View {
	if m.done || m.edit || m.aborted || m.eof {
		return tea.NewView("")
	}
	content := m.head() + m.input.View()
	if m.hint != "" {
		content += "\n  " + sgrDim + m.hint + sgrReset
	}
	if m.err != nil {
		content += "\n" + sgrRed + m.err.Error() + sgrReset
	}
	return tea.NewView(content)
}

// runPrompt runs one prompt on the terminal; Esc, Ctrl+C or a signal cancel the whole message.
func runPrompt(ctx context.Context, prompt *promptModel, input io.Reader, output io.Writer) error {
	err := runProgram(ctx, prompt, input, output)
	if ctx.Err() != nil || errors.Is(err, tea.ErrInterrupted) || prompt.aborted {
		return ErrCanceled
	}
	if err == nil && prompt.eof {
		return io.EOF
	}
	return err
}

func (m *commitForm) titleField(ctx context.Context, input io.Reader, output io.Writer) error {
	prompt := m.newTitlePrompt()
	if err := runPrompt(ctx, prompt, input, output); err != nil {
		return err
	}
	m.draft.Description = prompt.value()
	_, err := fmt.Fprintln(output, "? Enter the commit title: "+m.draft.Description)
	return err
}

// bodyField repeats the body prompt until a line is accepted or the editor saves a valid body.
func (m *commitForm) bodyField(ctx context.Context, input io.Reader, output io.Writer) error {
	for {
		prompt := newBodyModel(m.options.Editor)
		if err := runPrompt(ctx, prompt, input, output); err != nil {
			return err
		}
		if done, err := m.applyBody(prompt, input, output); done || err != nil {
			return err
		}
	}
}

// applyBody records the prompt's answer; a failed edit keeps the body and reports false to ask again.
func (m *commitForm) applyBody(prompt *promptModel, input io.Reader, output io.Writer) (bool, error) {
	if !prompt.edit {
		// An empty line keeps a received body and its trailers.
		if prompt.input.Value() != "" {
			m.draft.Body = prompt.input.Value()
		}
		_, err := fmt.Fprintln(output, "? Body (optional): "+prompt.input.Value())
		return true, err
	}
	body, err := editBody(m.options.Editor, m.draft.Body, input, output)
	if err != nil {
		_, err = fmt.Fprintf(output, "%v; the body was not changed\n", err)
		return false, err
	}
	m.draft.Body = body
	_, err = fmt.Fprintln(output, "? Body (optional): written in "+filepath.Base(m.options.Editor))
	return true, err
}

// editBody opens body in a private temporary file; an emptied file removes the body.
func editBody(editor, body string, input io.Reader, output io.Writer) (string, error) {
	file, err := os.CreateTemp("", "cmt-body-*.txt")
	if err != nil {
		return body, err
	}
	defer os.Remove(file.Name())
	if _, err := io.WriteString(file, body); err != nil {
		file.Close()
		return body, err
	}
	if err := file.Close(); err != nil {
		return body, err
	}
	cmd := exec.Command(editor, file.Name())
	cmd.Stdin, cmd.Stdout, cmd.Stderr = input, output, output
	if err := cmd.Run(); err != nil {
		return body, fmt.Errorf("%s: %w", filepath.Base(editor), err)
	}
	data, err := os.ReadFile(file.Name())
	if err != nil {
		return body, err
	}
	// Editors append a final newline; trailing blank lines carry no content.
	text := strings.TrimRight(string(data), "\r\n")
	switch {
	case text == strings.TrimRight(body, "\r\n"):
		return body, nil
	case strings.TrimSpace(text) == "":
		return "", nil
	}
	if err := commit.ValidateBody(text); err != nil {
		return body, err
	}
	return text, nil
}
