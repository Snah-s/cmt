package ui

import (
	"io"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestScopeFromPath(t *testing.T) {
	for path, want := range map[string]string{
		"internal/ui/body.go": "ui", "cmd/cmt/run.go": "cmt", "packages/Web-App/src/x.ts": "web-app",
		"scripts/package.sh": "scripts", "internal/doc.go": "internal", "README.md": "",
		".github/workflows/go.yml": "", "my docs/a.md": "",
	} {
		if got := scopeFromPath(path); got != want {
			t.Fatalf("scopeFromPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func scopeNames(m *commitForm) []string {
	var names []string
	for _, choice := range m.scopeChoices() {
		names = append(names, choice.name)
	}
	return names
}

func TestScopeChoicesOrder(t *testing.T) {
	m := newCommitForm(testCatalog(t), Draft{Scope: "api"}, Options{
		StagedPaths:  []string{"cmd/cmt/run.go", "internal/ui/a.go", "internal/ui/b.go", "README.md"},
		CustomScopes: []string{"deps", "ui"},
		TrackedPaths: []string{"internal/ui/a.go", "docs/x.md", "internal/catalog/c.go", "api/y.go"},
	})
	want := []string{"api", "ui", "cmt", "deps", "catalog", "docs"}
	if got := scopeNames(m); !slices.Equal(got, want) {
		t.Fatalf("order: %v, want %v", got, want)
	}
	if notes := m.scopeChoices(); notes[1].note != "staged · 2 files" || notes[2].note != "staged · 1 file" || notes[3].note != "saved" {
		t.Fatalf("notes: %+v", notes)
	}
	strict := newCommitForm(testCatalog(t), Draft{}, Options{Scopes: []string{"api", "cli"}, StagedPaths: []string{"internal/ui/a.go"}, CustomScopes: []string{"deps"}})
	if got := scopeNames(strict); !slices.Equal(got, []string{"api", "cli"}) {
		t.Fatalf("a project list must be strict: %v", got)
	}
}

func TestCustomScope(t *testing.T) {
	m := newCommitForm(testCatalog(t), Draft{Mode: "standard", Type: "feat"}, Options{FormatExplicit: true, StagedPaths: []string{"internal/ui/a.go"}})
	selector := m.inlineSelector(strings.NewReader(""), io.Discard)
	press(selector, tea.KeyEnter) // type
	view := ansi.Strip(selector.View().Content)
	if selector.step != "scope" || !strings.Contains(view, "> Skip scope") || !strings.Contains(view, "ui           staged · 1 file") {
		t.Fatalf("scope list: step=%q %q", selector.step, view)
	}
	press(selector, tea.KeyDown, tea.KeyDown, tea.KeyEnter) // Custom scope…
	press(selector, []rune("Auth")...)
	press(selector, tea.KeyEnter)
	if selector.step != "customScope" || !strings.Contains(ansi.Strip(selector.View().Content), "scope must be lowercase") {
		t.Fatalf("an invalid custom scope was accepted: step=%q", selector.step)
	}
	press(selector, tea.KeyEscape)
	if selector.step != "scope" || m.draft.Scope != "" || selector.form.State != huh.StateNormal {
		t.Fatalf("Esc must return to the list: step=%q scope=%q", selector.step, m.draft.Scope)
	}
	press(selector, tea.KeyDown, tea.KeyDown, tea.KeyEnter)
	press(selector, []rune("auth")...)
	press(selector, tea.KeyEnter)
	if !selector.done || m.draft.Scope != "auth" || selector.answers[len(selector.answers)-1] != "? Choose a scope (optional): auth" {
		t.Fatalf("custom scope: done=%v scope=%q answers=%q", selector.done, m.draft.Scope, selector.answers)
	}
}

func TestTypingFiltersScopes(t *testing.T) {
	m := newCommitForm(testCatalog(t), Draft{Mode: "standard", Type: "feat"}, Options{FormatExplicit: true, TrackedPaths: []string{"internal/ui/a.go", "internal/catalog/b.go", "internal/config/c.go"}})
	selector := m.inlineSelector(strings.NewReader(""), io.Discard)
	press(selector, tea.KeyEnter)
	if view := ansi.Strip(selector.View().Content); !strings.Contains(view, "Choose a scope (optional)") {
		t.Fatalf("the title must show until typing starts: %q", view)
	}
	press(selector, []rune("zzz")...)
	press(selector, tea.KeyEnter)
	if selector.done || selector.step != "scope" {
		t.Fatal("Enter on a filter without matches must not skip the scope")
	}
	press(selector, tea.KeyEscape)
	press(selector, []rune("conf")...)
	press(selector, tea.KeyEnter)
	if !selector.done || m.draft.Scope != "config" {
		t.Fatalf("typed filter: done=%v scope=%q", selector.done, m.draft.Scope)
	}
}
