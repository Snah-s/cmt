package ui

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// fakeEditor writes $CMT_EDIT (printf %b escapes) into the file, keeps it with "keep", or fails with "fail".
func fakeEditor(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ncase \"$CMT_EDIT\" in\nkeep) exit 0 ;;\nfail) exit 3 ;;\n*) printf '%b' \"$CMT_EDIT\" > \"$1\" ;;\nesac\n"
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFindEditor(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	if got := FindEditor(""); got != "" {
		t.Fatalf("no editor installed: %q", got)
	}
	vi, vim := fakeEditor(t, bin, "vi"), fakeEditor(t, bin, "vim")
	custom := fakeEditor(t, filepath.Join(t.TempDir(), "with spaces"), "my editor")
	for preference, want := range map[string]string{"": vim, "vi": vi, "missing": vim, custom: custom} {
		if got := FindEditor(preference); got != want {
			t.Fatalf("FindEditor(%q) = %q, want %q", preference, got, want)
		}
	}
}

func TestEditBody(t *testing.T) {
	editor := fakeEditor(t, filepath.Join(t.TempDir(), "with spaces"), "editor")
	for _, tc := range []struct{ edit, body, want string }{
		{"first paragraph\\n\\n  indented\\n\\nSigned-off-by: Ana\\n\\n", "old", "first paragraph\n\n  indented\n\nSigned-off-by: Ana"},
		{"keep", "old body\n", "old body\n"},
		{"", "old body", ""},
		{"  \\n\\n", "old body", ""},
	} {
		t.Setenv("CMT_EDIT", tc.edit)
		if got, err := editBody(editor, tc.body, nil, io.Discard); err != nil || got != tc.want {
			t.Fatalf("edit %q: %q, %v", tc.edit, got, err)
		}
	}
	for _, edit := range []string{"fail", "bad\\001"} {
		t.Setenv("CMT_EDIT", edit)
		if got, err := editBody(editor, "old body", nil, io.Discard); err == nil || got != "old body" {
			t.Fatalf("edit %q replaced the body: %q, %v", edit, got, err)
		}
	}
}

func TestBodyPromptOpensEditor(t *testing.T) {
	editor := fakeEditor(t, t.TempDir(), "nano")
	m := newCommitForm(testCatalog(t), Draft{Mode: "standard", Type: "docs", Body: "old body"}, Options{Editor: editor})
	prompt := newBodyModel(editor)
	if view := ansi.Strip(prompt.View().Content); !strings.Contains(view, "Ctrl+O: write it in nano") {
		t.Fatalf("editor hint: %q", view)
	}
	press(prompt, 'x')
	prompt.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	if !prompt.edit || prompt.done {
		t.Fatal("Ctrl+O did not choose the editor")
	}
	var output bytes.Buffer
	t.Setenv("CMT_EDIT", "fail")
	if done, err := m.applyBody(prompt, nil, &output); done || err != nil || m.draft.Body != "old body" || !strings.Contains(output.String(), "not changed") {
		t.Fatalf("failed edit: done=%v err=%v body=%q output=%q", done, err, m.draft.Body, output.String())
	}
	t.Setenv("CMT_EDIT", "")
	if done, err := m.applyBody(prompt, nil, &output); !done || err != nil || m.draft.Body != "" {
		t.Fatalf("emptied body: done=%v err=%v body=%q", done, err, m.draft.Body)
	}
	plain := newBodyModel("")
	plain.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	plain.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if plain.edit || !plain.aborted {
		t.Fatalf("without an editor Ctrl+O must do nothing and Esc must cancel: %+v", plain)
	}
}
