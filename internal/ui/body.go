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

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

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

// bodyModel asks for a one-line body; Ctrl+O hands the body to the editor instead.
type bodyModel struct {
	field   *huh.Input
	line    string
	editor  string
	edit    bool
	done    bool
	aborted bool
	err     error
}

func newBodyModel(editor string) *bodyModel {
	m := &bodyModel{editor: editor}
	m.field = huh.NewInput().Title("Body (optional)").Value(&m.line)
	if editor != "" {
		m.field.Description("Ctrl+O: write it in " + filepath.Base(editor))
	}
	m.field.WithTheme(charmTheme())
	m.field.Focus()
	return m
}

func (m *bodyModel) Init() tea.Cmd { return nil }

func (m *bodyModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if press, ok := msg.(tea.KeyPressMsg); ok {
		switch press.String() {
		case "enter":
			if m.err = commit.ValidateBody(m.line); m.err == nil {
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
		}
	}
	_, cmd := m.field.Update(msg)
	return m, cmd
}

func (m *bodyModel) View() tea.View {
	if m.done || m.edit || m.aborted {
		return tea.NewView("")
	}
	content := m.field.View()
	if m.err != nil {
		content += "\n" + m.err.Error()
	}
	return tea.NewView(content)
}

// bodyField repeats the body prompt until a line is accepted or the editor saves a valid body.
func (m *commitForm) bodyField(ctx context.Context, input io.Reader, output io.Writer) error {
	for {
		prompt := newBodyModel(m.options.Editor)
		err := runProgram(ctx, prompt, input, output)
		if ctx.Err() != nil || errors.Is(err, tea.ErrInterrupted) || prompt.aborted {
			return ErrCanceled
		}
		if err != nil {
			return err
		}
		if done, err := m.applyBody(prompt, input, output); done || err != nil {
			return err
		}
	}
}

// applyBody records the prompt's answer; a failed edit keeps the body and reports false to ask again.
func (m *commitForm) applyBody(prompt *bodyModel, input io.Reader, output io.Writer) (bool, error) {
	if !prompt.edit {
		// An empty line keeps a received body and its trailers.
		if prompt.line != "" {
			m.draft.Body = prompt.line
		}
		_, err := fmt.Fprintln(output, "? Body (optional): "+prompt.line)
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
