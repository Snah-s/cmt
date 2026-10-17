package ui

import (
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestEmojiTypeAhead(t *testing.T) {
	for _, test := range []struct{ query, code string }{
		{"sparkles", ":sparkles:"},
		{"SPR KLS", ":sparkles:"},
		{"sparlkes", ":sparkles:"},
		{"sparklex", ":sparkles:"},
		{":memo:", ":memo:"},
		{"🐛", ":bug:"},
		{"🧑‍💻", ":technologist:"},
		{"compiler warnings", ":rotating_light:"},
	} {
		t.Run(test.query, func(t *testing.T) {
			owner := newCommitForm(testCatalog(t), Draft{Mode: "emoji", Emoji: ":art:"}, Options{FormatExplicit: true})
			selector := owner.inlineSelector(strings.NewReader(""), io.Discard)
			selector.Update(tea.KeyPressMsg{Code: 's', Text: test.query})
			if got, _ := selector.emojis.hovered(); got != test.code {
				t.Fatalf("query %q selected %q, want %q", test.query, got, test.code)
			}
			if owner.draft.Emoji != ":art:" {
				t.Fatal("typing confirmed a choice without Enter")
			}
			selector.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if !selector.done || owner.draft.Emoji != test.code {
				t.Fatalf("selection not confirmed: %+v", owner.draft)
			}
		})
	}
}

func TestEmojiSearchEditingAndCancellation(t *testing.T) {
	owner := newCommitForm(testCatalog(t), Draft{Mode: "emoji", Emoji: ":art:"}, Options{FormatExplicit: true})
	selector := owner.inlineSelector(strings.NewReader(""), io.Discard)
	selector.Update(tea.PasteMsg{Content: "📝\n"})
	if selector.emojis.query != "📝" {
		t.Fatal("paste retained control characters")
	}
	selector.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if selector.emojis.query != "" || len(selector.emojis.matches) != len(owner.catalog.Entries) {
		t.Fatal("backspace did not remove the complete Unicode rune")
	}
	selector.Update(tea.PasteMsg{Content: "zzzzzzzz"})
	for _, key := range []rune{tea.KeyDown, tea.KeyUp, tea.KeyEnter} {
		selector.Update(tea.KeyPressMsg{Code: key})
	}
	if selector.done || owner.draft.Emoji != ":art:" || !strings.Contains(selector.View().Content, "No matching emojis") {
		t.Fatal("empty results selected a hidden default")
	}
	selector.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	if selector.emojis.query != "" {
		t.Fatal("Ctrl+U did not clear the query")
	}
	selector.Update(tea.PasteMsg{Content: "memo"})
	selector.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if got, _ := selector.emojis.hovered(); got != ":memo:" || selector.emojis.filtering || selector.form.State != huh.StateNormal {
		t.Fatal("Escape did not clear search while retaining the highlighted emoji")
	}
	selector.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if selector.form.State != huh.StateAborted {
		t.Fatal("second Escape did not cancel")
	}
}

func TestEmojiWheel(t *testing.T) {
	owner := newCommitForm(testCatalog(t), Draft{Mode: "emoji"}, Options{FormatExplicit: true})
	selector := owner.inlineSelector(strings.NewReader(""), io.Discard)
	s := selector.emojis
	rows := func() []string {
		var labels []string
		for _, line := range strings.Split(s.view(160, 40), "\n") {
			if strings.HasPrefix(line, "┃   ") || strings.HasPrefix(line, "┃ > ") {
				labels = append(labels, strings.TrimPrefix(strings.TrimPrefix(line, "┃   "), "┃ > "))
			}
		}
		return labels
	}
	for range len(s.options) + 2 {
		before := rows()
		selector.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		after := rows()
		if strings.Join(before[1:], "\n") != strings.Join(after[:4], "\n") {
			t.Fatalf("wheel jumped instead of advancing one row: %q -> %q", before, after)
		}
		if !strings.Contains(s.view(160, 40), "┃ > "+after[2]) {
			t.Fatal("selection moved away from the center")
		}
	}
	s.cursor = 0
	selector.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if got, _ := s.hovered(); got != owner.catalog.Entries[len(owner.catalog.Entries)-1].Code {
		t.Fatal("wheel did not wrap upward")
	}
}

func TestEmojiHybridSearchAndSmallWindow(t *testing.T) {
	owner := newCommitForm(testCatalog(t), Draft{Mode: "hybrid", Type: "docs"}, Options{FormatExplicit: true})
	selector := owner.inlineSelector(strings.NewReader(""), io.Discard)
	selector.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	selector.Update(tea.PasteMsg{Content: "bug"})
	for _, match := range selector.emojis.matches {
		if !owner.catalog.Compatible("docs", selector.emojis.options[match.index].entry.Code) {
			t.Fatal("search exposed an incompatible emoji")
		}
	}
	selector.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	selector.Update(tea.WindowSizeMsg{Width: 20, Height: 7})
	view := selector.View()
	if view.AltScreen {
		t.Fatal("selector entered the alternate screen")
	}
	for _, line := range strings.Split(view.Content, "\n") {
		if strings.HasPrefix(line, "┃") && (ansi.StringWidth(line) >= 20 || ansi.StringWidthWc(line) >= 20) {
			t.Fatalf("option wraps in a small terminal: %q", line)
		}
	}
}

func TestEmojiSearchKeepsWindowHeight(t *testing.T) {
	all := testCatalog(t)
	for _, size := range []int{0, 2, len(all.Entries)} {
		for _, height := range []int{0, 3, 5, 40} {
			c := all
			c.Entries = c.Entries[:size]
			owner := newCommitForm(c, Draft{Mode: "emoji"}, Options{FormatExplicit: true})
			s := owner.inlineSelector(strings.NewReader(""), io.Discard).emojis
			initial := strings.Count(s.view(160, height), "\n") + 1
			if height > 0 && initial > height {
				t.Fatalf("catalog size %d: %d rows exceed terminal height %d", size, initial, height)
			}
			query := "memo"
			if size > 0 {
				query = c.Entries[0].Code
			}
			for _, query := range []string{query, "zzzzzzzz", ""} {
				s.query = query
				s.filter()
				view := s.view(160, height)
				if got := strings.Count(view, "\n") + 1; got != initial {
					t.Fatalf("catalog size %d, terminal height %d, query %q: height changed %d -> %d", size, height, query, initial, got)
				}
				if len(s.matches) == 0 {
					if strings.Contains(view, "┃ > ") || !strings.Contains(view, "No matching emojis") {
						t.Fatalf("empty search rendered a choice: %q", view)
					}
				} else if strings.Count(view, "┃ > ") != 1 {
					t.Fatalf("search did not render exactly one selection: %q", view)
				}
			}
		}
	}
}

func TestOneEditApart(t *testing.T) {
	for _, test := range []struct {
		query, word string
		want        bool
	}{
		{"sparkles", "sparkles", true}, {"sparlkes", "sparkles", true},
		{"sparklex", "sparkles", true}, {"sparklesx", "sparkles", true},
		{"spakles", "sparkles", true}, {"spark", "sparkles", false},
		{"sparxlxs", "sparkles", false}, {"résumé", "resumé", true},
		{"résumé", "resume", false}, {"", "a", true}, {"", "ab", false},
	} {
		if got := oneEditApart(test.query, test.word); got != test.want {
			t.Errorf("%q / %q: %v, want %v", test.query, test.word, got, test.want)
		}
	}
}
